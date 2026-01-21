package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
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

// OAuth structures
type AuthCode struct {
	Code          string
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	Scope         string
	ExpiresAt     time.Time
}

type AccessToken struct {
	Token     string
	ClientID  string
	Scope     string
	ExpiresAt time.Time
}

// MCP Server
type MCPServer struct {
	apiURL       string
	apiUsername  string
	apiPassword  string
	mcpTokens    []string // Valid tokens (comma-separated in env, used as client_secret)
	clientID     string   // OAuth client ID
	baseURL      string   // Server's base URL for OAuth
	authCodes    map[string]*AuthCode
	accessTokens map[string]*AccessToken
	mu           sync.RWMutex
}

func NewMCPServer() *MCPServer {
	apiURL := os.Getenv("FAMILYBOT_API_URL")
	if apiURL == "" {
		apiURL = "https://family.tazhate.com"
	}

	baseURL := os.Getenv("MCP_BASE_URL")
	if baseURL == "" {
		baseURL = "https://mcp.family.tazhate.com"
	}

	clientID := os.Getenv("MCP_CLIENT_ID")
	if clientID == "" {
		clientID = "familybot"
	}

	// Parse comma-separated tokens
	var tokens []string
	tokenStr := os.Getenv("FAMILYBOT_MCP_TOKEN")
	if tokenStr != "" {
		for _, t := range strings.Split(tokenStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tokens = append(tokens, t)
			}
		}
	}

	return &MCPServer{
		apiURL:       apiURL,
		apiUsername:  os.Getenv("FAMILYBOT_API_USERNAME"),
		apiPassword:  os.Getenv("FAMILYBOT_API_PASSWORD"),
		mcpTokens:    tokens,
		clientID:     clientID,
		baseURL:      baseURL,
		authCodes:    make(map[string]*AuthCode),
		accessTokens: make(map[string]*AccessToken),
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
	// OAuth 2.1 endpoints
	http.HandleFunc("/.well-known/oauth-protected-resource", s.handleProtectedResourceMetadata)
	http.HandleFunc("/.well-known/oauth-authorization-server", s.handleAuthServerMetadata)
	http.HandleFunc("/authorize", s.handleAuthorize)
	http.HandleFunc("/token", s.handleToken)

	// MCP endpoints
	http.HandleFunc("/mcp", s.handleHTTP)
	http.HandleFunc("/mcp/sse", s.handleSSE)

	// Health check
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Printf("MCP HTTP server starting on %s", addr)
	log.Printf("OAuth endpoints enabled, base URL: %s", s.baseURL)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

// OAuth 2.1 Protected Resource Metadata (RFC 9728)
func (s *MCPServer) handleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	metadata := map[string]interface{}{
		"resource":              s.baseURL,
		"authorization_servers": []string{s.baseURL},
		"scopes_supported":      []string{"mcp"},
	}

	json.NewEncoder(w).Encode(metadata)
}

// OAuth 2.1 Authorization Server Metadata (RFC 8414)
func (s *MCPServer) handleAuthServerMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	metadata := map[string]interface{}{
		"issuer":                                s.baseURL,
		"authorization_endpoint":                s.baseURL + "/authorize",
		"token_endpoint":                        s.baseURL + "/token",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
		"scopes_supported":                      []string{"mcp"},
	}

	json.NewEncoder(w).Encode(metadata)
}

// OAuth 2.1 Authorization Endpoint
func (s *MCPServer) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	log.Printf("OAuth /authorize request: %s %s", r.Method, r.URL.String())

	// CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Parse parameters
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	responseType := r.URL.Query().Get("response_type")
	scope := r.URL.Query().Get("scope")
	state := r.URL.Query().Get("state")
	codeChallenge := r.URL.Query().Get("code_challenge")
	codeChallengeMethod := r.URL.Query().Get("code_challenge_method")

	log.Printf("OAuth authorize params: client_id=%s, redirect_uri=%s, response_type=%s, scope=%s, code_challenge_method=%s",
		clientID, redirectURI, responseType, scope, codeChallengeMethod)

	// Validate required parameters
	if responseType != "code" {
		s.oauthError(w, r, redirectURI, "unsupported_response_type", "Only 'code' response type is supported", state)
		return
	}

	if codeChallengeMethod != "" && codeChallengeMethod != "S256" {
		s.oauthError(w, r, redirectURI, "invalid_request", "Only S256 code challenge method is supported", state)
		return
	}

	if redirectURI == "" {
		http.Error(w, "redirect_uri is required", http.StatusBadRequest)
		return
	}

	// Generate authorization code
	code := s.generateCode()

	// Store authorization code
	s.mu.Lock()
	s.authCodes[code] = &AuthCode{
		Code:          code,
		ClientID:      clientID,
		RedirectURI:   redirectURI,
		CodeChallenge: codeChallenge,
		Scope:         scope,
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	}
	s.mu.Unlock()

	log.Printf("OAuth: Generated auth code for client %s", clientID)

	// Redirect back with code
	redirectURL, err := url.Parse(redirectURI)
	if err != nil {
		http.Error(w, "Invalid redirect_uri", http.StatusBadRequest)
		return
	}

	q := redirectURL.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	redirectURL.RawQuery = q.Encode()

	log.Printf("OAuth: Redirecting to %s", redirectURL.String())
	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// OAuth 2.1 Token Endpoint
func (s *MCPServer) handleToken(w http.ResponseWriter, r *http.Request) {
	log.Printf("OAuth /token request: %s", r.Method)

	// CORS
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

	// Parse form data
	if err := r.ParseForm(); err != nil {
		s.tokenError(w, "invalid_request", "Failed to parse form")
		return
	}

	grantType := r.FormValue("grant_type")
	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	codeVerifier := r.FormValue("code_verifier")

	// Also check Basic auth for client credentials
	if clientID == "" || clientSecret == "" {
		if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Basic ") {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authHeader, "Basic "))
			if err == nil {
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) == 2 {
					clientID = parts[0]
					clientSecret = parts[1]
				}
			}
		}
	}

	log.Printf("OAuth token request: grant_type=%s, client_id=%s, code=%s", grantType, clientID, code[:min(10, len(code))]+"...")

	if grantType != "authorization_code" {
		s.tokenError(w, "unsupported_grant_type", "Only authorization_code grant is supported")
		return
	}

	// Validate client_secret against list of valid tokens
	validSecret := false
	for _, token := range s.mcpTokens {
		if clientSecret == token {
			validSecret = true
			break
		}
	}
	if !validSecret {
		log.Printf("OAuth: Invalid client_secret")
		s.tokenError(w, "invalid_client", "Invalid client credentials")
		return
	}

	// Look up authorization code
	s.mu.Lock()
	authCode, exists := s.authCodes[code]
	if exists {
		delete(s.authCodes, code) // One-time use
	}
	s.mu.Unlock()

	if !exists {
		log.Printf("OAuth: Code not found")
		s.tokenError(w, "invalid_grant", "Invalid or expired authorization code")
		return
	}

	if time.Now().After(authCode.ExpiresAt) {
		log.Printf("OAuth: Code expired")
		s.tokenError(w, "invalid_grant", "Authorization code expired")
		return
	}

	// Validate redirect_uri
	if redirectURI != authCode.RedirectURI {
		log.Printf("OAuth: Redirect URI mismatch: %s != %s", redirectURI, authCode.RedirectURI)
		s.tokenError(w, "invalid_grant", "Redirect URI mismatch")
		return
	}

	// Validate PKCE code_verifier
	if authCode.CodeChallenge != "" {
		if codeVerifier == "" {
			log.Printf("OAuth: Missing code_verifier")
			s.tokenError(w, "invalid_grant", "code_verifier required")
			return
		}

		// S256: BASE64URL(SHA256(code_verifier)) == code_challenge
		h := sha256.Sum256([]byte(codeVerifier))
		computedChallenge := base64.RawURLEncoding.EncodeToString(h[:])

		if computedChallenge != authCode.CodeChallenge {
			log.Printf("OAuth: PKCE verification failed")
			log.Printf("  code_verifier: %s", codeVerifier[:min(20, len(codeVerifier))]+"...")
			log.Printf("  computed challenge: %s", computedChallenge)
			log.Printf("  expected challenge: %s", authCode.CodeChallenge)
			s.tokenError(w, "invalid_grant", "PKCE verification failed")
			return
		}
	}

	// Generate access token
	accessToken := s.generateToken()

	// Store token
	s.mu.Lock()
	s.accessTokens[accessToken] = &AccessToken{
		Token:     accessToken,
		ClientID:  clientID,
		Scope:     authCode.Scope,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	s.mu.Unlock()

	log.Printf("OAuth: Issued access token for client %s", clientID)

	// Return token response
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	response := map[string]interface{}{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   86400,
		"scope":        authCode.Scope,
	}

	json.NewEncoder(w).Encode(response)
}

func (s *MCPServer) oauthError(w http.ResponseWriter, r *http.Request, redirectURI, errorCode, description, state string) {
	if redirectURI == "" {
		http.Error(w, description, http.StatusBadRequest)
		return
	}

	redirectURL, err := url.Parse(redirectURI)
	if err != nil {
		http.Error(w, description, http.StatusBadRequest)
		return
	}

	q := redirectURL.Query()
	q.Set("error", errorCode)
	q.Set("error_description", description)
	if state != "" {
		q.Set("state", state)
	}
	redirectURL.RawQuery = q.Encode()

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

func (s *MCPServer) tokenError(w http.ResponseWriter, errorCode, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)

	response := map[string]string{
		"error":             errorCode,
		"error_description": description,
	}

	json.NewEncoder(w).Encode(response)
}

func (s *MCPServer) generateCode() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *MCPServer) generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
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
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource"`, s.baseURL))
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
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource"`, s.baseURL))
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

// checkAuth verifies Bearer token (OAuth access token or legacy token)
func (s *MCPServer) checkAuth(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		// If no auth configured, allow access
		if len(s.mcpTokens) == 0 {
			return true
		}
		return false
	}

	// Check Bearer token
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")

		// Check if it's a valid OAuth access token
		s.mu.RLock()
		accessToken, exists := s.accessTokens[token]
		s.mu.RUnlock()

		if exists && time.Now().Before(accessToken.ExpiresAt) {
			return true
		}

		// Fall back to legacy simple token check
		for _, validToken := range s.mcpTokens {
			if token == validToken {
				return true
			}
		}
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
