package todoist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	BaseURL = "https://api.todoist.com/rest/v2"
)

// Client is a Todoist API client
type Client struct {
	token            string
	httpClient       *http.Client
	projectID        string // Optional: specific project to sync with
	sectionID        string // Optional: owner's section to sync with
	partnerSectionID string // Optional: partner's section to sync with
}

// NewClient creates a new Todoist client
func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsConfigured returns true if the client has a token
func (c *Client) IsConfigured() bool {
	return c.token != ""
}

// SetProjectID sets the project to sync with
func (c *Client) SetProjectID(id string) {
	c.projectID = id
}

// GetProjectID returns the configured project ID
func (c *Client) GetProjectID() string {
	return c.projectID
}

// SetSectionID sets the owner's section to sync with
func (c *Client) SetSectionID(id string) {
	c.sectionID = id
}

// GetSectionID returns the configured owner's section ID
func (c *Client) GetSectionID() string {
	return c.sectionID
}

// SetPartnerSectionID sets the partner's section to sync with
func (c *Client) SetPartnerSectionID(id string) {
	c.partnerSectionID = id
}

// GetPartnerSectionID returns the configured partner's section ID
func (c *Client) GetPartnerSectionID() string {
	return c.partnerSectionID
}

// doRequest performs an HTTP request with auth
func (c *Client) doRequest(method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest(method, BaseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// GetTasks returns all active tasks, optionally filtered by project
func (c *Client) GetTasks(projectID string) ([]Task, error) {
	path := "/tasks"
	if projectID != "" {
		path += "?project_id=" + projectID
	} else if c.projectID != "" {
		path += "?project_id=" + c.projectID
	}

	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var tasks []Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("unmarshal tasks: %w", err)
	}

	return tasks, nil
}

// GetTasksBySection returns tasks filtered by section ID
func (c *Client) GetTasksBySection(sectionID string) ([]Task, error) {
	// Todoist API doesn't support direct section filter, so we get all project tasks and filter
	// Use configured project ID by passing empty string (will fallback to c.projectID)
	tasks, err := c.GetTasks(c.projectID)
	if err != nil {
		return nil, err
	}

	if sectionID == "" {
		return tasks, nil
	}

	var filtered []Task
	for _, t := range tasks {
		if t.SectionID == sectionID {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

// GetTask returns a single task by ID
func (c *Client) GetTask(id string) (*Task, error) {
	data, err := c.doRequest("GET", "/tasks/"+id, nil)
	if err != nil {
		return nil, err
	}

	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("unmarshal task: %w", err)
	}

	return &task, nil
}

// CreateTask creates a new task
func (c *Client) CreateTask(req *CreateTaskRequest) (*Task, error) {
	// Use default project if not specified
	if req.ProjectID == "" && c.projectID != "" {
		req.ProjectID = c.projectID
	}
	// Use default section if not specified
	if req.SectionID == "" && c.sectionID != "" {
		req.SectionID = c.sectionID
	}

	data, err := c.doRequest("POST", "/tasks", req)
	if err != nil {
		return nil, err
	}

	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("unmarshal task: %w", err)
	}

	return &task, nil
}

// UpdateTask updates an existing task
func (c *Client) UpdateTask(id string, req *UpdateTaskRequest) error {
	_, err := c.doRequest("POST", "/tasks/"+id, req)
	return err
}

// CloseTask marks a task as complete
func (c *Client) CloseTask(id string) error {
	_, err := c.doRequest("POST", "/tasks/"+id+"/close", nil)
	return err
}

// ReopenTask reopens a completed task
func (c *Client) ReopenTask(id string) error {
	_, err := c.doRequest("POST", "/tasks/"+id+"/reopen", nil)
	return err
}

// DeleteTask deletes a task
func (c *Client) DeleteTask(id string) error {
	_, err := c.doRequest("DELETE", "/tasks/"+id, nil)
	return err
}

// GetProjects returns all projects
func (c *Client) GetProjects() ([]Project, error) {
	data, err := c.doRequest("GET", "/projects", nil)
	if err != nil {
		return nil, err
	}

	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, fmt.Errorf("unmarshal projects: %w", err)
	}

	return projects, nil
}

// GetProject returns a single project by ID
func (c *Client) GetProject(id string) (*Project, error) {
	data, err := c.doRequest("GET", "/projects/"+id, nil)
	if err != nil {
		return nil, err
	}

	var project Project
	if err := json.Unmarshal(data, &project); err != nil {
		return nil, fmt.Errorf("unmarshal project: %w", err)
	}

	return &project, nil
}

// GetSections returns all sections for a project
func (c *Client) GetSections(projectID string) ([]Section, error) {
	path := "/sections"
	if projectID != "" {
		path += "?project_id=" + projectID
	} else if c.projectID != "" {
		path += "?project_id=" + c.projectID
	}

	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var sections []Section
	if err := json.Unmarshal(data, &sections); err != nil {
		return nil, fmt.Errorf("unmarshal sections: %w", err)
	}

	return sections, nil
}
