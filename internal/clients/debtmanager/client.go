package debtmanager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is the HTTP client for debt-manager API
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new debt-manager API client
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsConfigured returns true if the client has URL and token configured
func (c *Client) IsConfigured() bool {
	return c.baseURL != "" && c.token != ""
}

// doRequest performs an HTTP request with authentication
func (c *Client) doRequest(method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

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

// GetDebts returns all debts
func (c *Client) GetDebts() ([]Debt, error) {
	body, err := c.doRequest("GET", "/debts", nil)
	if err != nil {
		return nil, err
	}

	var debts []Debt
	if err := json.Unmarshal(body, &debts); err != nil {
		return nil, fmt.Errorf("unmarshal debts: %w", err)
	}

	return debts, nil
}

// GetDebt returns a specific debt by ID
func (c *Client) GetDebt(id uint) (*Debt, error) {
	body, err := c.doRequest("GET", fmt.Sprintf("/debts/%d", id), nil)
	if err != nil {
		return nil, err
	}

	var debt Debt
	if err := json.Unmarshal(body, &debt); err != nil {
		return nil, fmt.Errorf("unmarshal debt: %w", err)
	}

	return &debt, nil
}

// GetIncomes returns all incomes
func (c *Client) GetIncomes() ([]Income, error) {
	body, err := c.doRequest("GET", "/incomes", nil)
	if err != nil {
		return nil, err
	}

	var incomes []Income
	if err := json.Unmarshal(body, &incomes); err != nil {
		return nil, fmt.Errorf("unmarshal incomes: %w", err)
	}

	return incomes, nil
}

// GetDashboard returns the dashboard summary
func (c *Client) GetDashboard() (*Dashboard, error) {
	body, err := c.doRequest("GET", "/dashboard", nil)
	if err != nil {
		return nil, err
	}

	var dashboard Dashboard
	if err := json.Unmarshal(body, &dashboard); err != nil {
		return nil, fmt.Errorf("unmarshal dashboard: %w", err)
	}

	return &dashboard, nil
}

// CreatePayment creates a new payment record
func (c *Client) CreatePayment(debtID uint, amount float64, date time.Time) error {
	payload := map[string]interface{}{
		"debt_id":      debtID,
		"amount":       amount,
		"payment_date": date.Format("2006-01-02"),
		"status":       "paid",
	}

	_, err := c.doRequest("POST", "/payments", payload)
	return err
}

// GetPaymentStatuses returns payment statuses for tracking
func (c *Client) GetPaymentStatuses() ([]PaymentStatus, error) {
	body, err := c.doRequest("GET", "/payment-statuses", nil)
	if err != nil {
		return nil, err
	}

	var statuses []PaymentStatus
	if err := json.Unmarshal(body, &statuses); err != nil {
		return nil, fmt.Errorf("unmarshal payment statuses: %w", err)
	}

	return statuses, nil
}

// UpdatePaymentStatus updates the status of a payment
func (c *Client) UpdatePaymentStatus(id uint, status string) error {
	payload := map[string]interface{}{
		"id":     id,
		"status": status,
	}

	_, err := c.doRequest("PUT", "/payment-status", payload)
	return err
}

// GetDebtsForDay returns debts that have payment on a specific day of month
func (c *Client) GetDebtsForDay(day int) ([]Debt, error) {
	debts, err := c.GetDebts()
	if err != nil {
		return nil, err
	}

	var result []Debt
	for _, d := range debts {
		if d.PaymentDay == day && d.CurrentAmount > 0 {
			result = append(result, d)
		}
	}

	return result, nil
}

// GetTotalMonthlyPayment calculates total monthly payment for all active debts
func (c *Client) GetTotalMonthlyPayment() (float64, error) {
	debts, err := c.GetDebts()
	if err != nil {
		return 0, err
	}

	var total float64
	for _, d := range debts {
		if d.CurrentAmount > 0 {
			total += d.MonthlyPayment
		}
	}

	return total, nil
}

// IsPayday checks if today is a payday based on incomes
func (c *Client) IsPayday(day int) (bool, []Income, error) {
	incomes, err := c.GetIncomes()
	if err != nil {
		return false, nil, err
	}

	var paydayIncomes []Income
	for _, inc := range incomes {
		if inc.PaymentDay == day && inc.IsRecurring {
			paydayIncomes = append(paydayIncomes, inc)
		}
	}

	return len(paydayIncomes) > 0, paydayIncomes, nil
}
