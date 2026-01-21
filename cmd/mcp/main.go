package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// JSON-RPC structures
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP structures
type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type ToolCallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MCP Server
type MCPServer struct {
	apiURL      string
	apiUsername string
	apiPassword string
	mcpToken    string
}

func NewMCPServer() *MCPServer {
	apiURL := os.Getenv("FAMILYBOT_API_URL")
	if apiURL == "" {
		apiURL = "https://family.tazhate.com"
	}
	return &MCPServer{
		apiURL:      apiURL,
		apiUsername: os.Getenv("FAMILYBOT_API_USERNAME"),
		apiPassword: os.Getenv("FAMILYBOT_API_PASSWORD"),
		mcpToken:    os.Getenv("FAMILYBOT_MCP_TOKEN"),
	}
}

// RunStdio runs the server in stdio mode (for local Claude Code)
func (s *MCPServer) RunStdio() {
	reader := bufio.NewReader(os.Stdin)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return
			}
			fmt.Fprintf(os.Stderr, "Error reading: %v\n", err)
			continue
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
			continue
		}

		response := s.handleRequest(req)
		responseBytes, _ := json.Marshal(response)
		fmt.Println(string(responseBytes))
	}
}

// RunHTTP runs the server in HTTP mode (for mobile Claude)
func (s *MCPServer) RunHTTP(addr string) {
	http.HandleFunc("/mcp", s.handleHTTP)
	http.HandleFunc("/mcp/sse", s.handleSSE)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Printf("MCP HTTP server starting on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

// handleHTTP handles regular HTTP POST requests
func (s *MCPServer) handleHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS headers for browser clients
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check authentication
	if !s.checkAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	response := s.handleRequest(req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSSE handles Server-Sent Events for streaming (MCP Streamable HTTP)
func (s *MCPServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Check authentication
	if !s.checkAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// For GET requests, just keep connection alive (for initial SSE setup)
	if r.Method == "GET" {
		// Send endpoint event to tell client where to POST
		fmt.Fprintf(w, "event: endpoint\ndata: /mcp\n\n")
		flusher.Flush()

		// Keep connection alive
		<-r.Context().Done()
		return
	}

	// For POST requests, process the JSON-RPC request
	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: Error reading body\n\n")
		flusher.Flush()
		return
	}
	defer r.Body.Close()

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		fmt.Fprintf(w, "event: error\ndata: Invalid JSON\n\n")
		flusher.Flush()
		return
	}

	response := s.handleRequest(req)
	responseBytes, _ := json.Marshal(response)

	fmt.Fprintf(w, "event: message\ndata: %s\n\n", string(responseBytes))
	flusher.Flush()
}

// checkAuth verifies Bearer token or Basic auth
func (s *MCPServer) checkAuth(r *http.Request) bool {
	if s.mcpToken == "" {
		return true // No auth required if token not configured
	}

	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false
	}

	// Check Bearer token
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		return token == s.mcpToken
	}

	return false
}

func (s *MCPServer) handleRequest(req JSONRPCRequest) JSONRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "initialized":
		return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: nil}
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	default:
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32601, Message: "Method not found"},
		}
	}
}

func (s *MCPServer) handleInitialize(req JSONRPCRequest) JSONRPCResponse {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: map[string]interface{}{
			"tools": map[string]interface{}{},
		},
	}
	result.ServerInfo.Name = "familybot-mcp"
	result.ServerInfo.Version = "1.0.0"

	return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (s *MCPServer) handleToolsList(req JSONRPCRequest) JSONRPCResponse {
	tools := []Tool{
		{
			Name:        "familybot_list_tasks",
			Description: "Получить список активных задач. Возвращает все незавершённые задачи с их приоритетами и датами.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]Property{}},
		},
		{
			Name:        "familybot_list_tasks_today",
			Description: "Получить задачи на сегодня (срочные и с дедлайном сегодня).",
			InputSchema: InputSchema{Type: "object", Properties: map[string]Property{}},
		},
		{
			Name:        "familybot_create_task",
			Description: "Создать новую задачу. Приоритеты: urgent (срочно), week (на неделю), someday (когда-нибудь).",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"title":    {Type: "string", Description: "Название задачи"},
					"priority": {Type: "string", Description: "Приоритет: urgent, week, someday", Enum: []string{"urgent", "week", "someday"}},
					"due_date": {Type: "string", Description: "Дата дедлайна в формате YYYY-MM-DD (опционально)"},
				},
				Required: []string{"title"},
			},
		},
		{
			Name:        "familybot_complete_task",
			Description: "Отметить задачу как выполненную по её ID.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"task_id": {Type: "string", Description: "ID задачи (число)"},
				},
				Required: []string{"task_id"},
			},
		},
		{
			Name:        "familybot_delete_task",
			Description: "Удалить задачу по её ID.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"task_id": {Type: "string", Description: "ID задачи (число)"},
				},
				Required: []string{"task_id"},
			},
		},
		{
			Name:        "familybot_list_people",
			Description: "Получить список людей (семья, дети, контакты) с их днями рождения.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]Property{}},
		},
		{
			Name:        "familybot_list_birthdays",
			Description: "Получить ближайшие дни рождения (на 60 дней вперёд).",
			InputSchema: InputSchema{Type: "object", Properties: map[string]Property{}},
		},
		{
			Name:        "familybot_list_reminders",
			Description: "Получить список активных напоминаний.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]Property{}},
		},
		{
			Name:        "familybot_week_schedule",
			Description: "Получить недельное расписание (регулярные события по дням недели).",
			InputSchema: InputSchema{Type: "object", Properties: map[string]Property{}},
		},
		{
			Name:        "familybot_shared_tasks",
			Description: "Получить общие семейные задачи.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]Property{}},
		},
	}

	return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: ToolsListResult{Tools: tools}}
}

func (s *MCPServer) handleToolsCall(req JSONRPCRequest) JSONRPCResponse {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32602, Message: "Invalid params"},
		}
	}

	var result string
	var isError bool

	switch params.Name {
	case "familybot_list_tasks":
		result, isError = s.apiGet("/api/tasks")
	case "familybot_list_tasks_today":
		result, isError = s.apiGet("/api/tasks/today")
	case "familybot_create_task":
		result, isError = s.apiPost("/api/tasks", params.Arguments)
	case "familybot_complete_task":
		taskID := fmt.Sprintf("%v", params.Arguments["task_id"])
		result, isError = s.apiPost("/api/task/"+taskID+"/done", nil)
	case "familybot_delete_task":
		taskID := fmt.Sprintf("%v", params.Arguments["task_id"])
		result, isError = s.apiDelete("/api/task/" + taskID)
	case "familybot_list_people":
		result, isError = s.apiGet("/api/people")
	case "familybot_list_birthdays":
		result, isError = s.apiGet("/api/birthdays")
	case "familybot_list_reminders":
		result, isError = s.apiGet("/api/reminders")
	case "familybot_week_schedule":
		result, isError = s.apiGet("/api/week")
	case "familybot_shared_tasks":
		result, isError = s.apiGet("/api/tasks/shared")
	default:
		result = "Unknown tool: " + params.Name
		isError = true
	}

	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: result}},
			IsError: isError,
		},
	}
}

func (s *MCPServer) apiGet(path string) (string, bool) {
	return s.apiRequest("GET", path, nil)
}

func (s *MCPServer) apiPost(path string, body interface{}) (string, bool) {
	return s.apiRequest("POST", path, body)
}

func (s *MCPServer) apiDelete(path string) (string, bool) {
	return s.apiRequest("DELETE", path, nil)
}

func (s *MCPServer) apiRequest(method, path string, body interface{}) (string, bool) {
	url := s.apiURL + path

	var reqBody io.Reader
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return fmt.Sprintf("Error creating request: %v", err), true
	}

	req.SetBasicAuth(s.apiUsername, s.apiPassword)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("Error making request: %v", err), true
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("Error reading response: %v", err), true
	}

	// Parse and format the response
	var apiResp struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   string          `json:"error"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return string(respBody), resp.StatusCode >= 400
	}

	if !apiResp.Success {
		return fmt.Sprintf("API Error: %s", apiResp.Error), true
	}

	// Pretty print the data
	var prettyData bytes.Buffer
	if err := json.Indent(&prettyData, apiResp.Data, "", "  "); err != nil {
		return string(apiResp.Data), false
	}

	return prettyData.String(), false
}

func main() {
	server := NewMCPServer()

	// Check if running in HTTP mode
	httpAddr := os.Getenv("MCP_HTTP_ADDR")
	if httpAddr != "" {
		server.RunHTTP(httpAddr)
	} else {
		server.RunStdio()
	}
}
