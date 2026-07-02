package main

import (
	"encoding/json"
	"fmt"
	"log"
)

// JSON-RPC 2.0 types.

type JSONRPCRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

// IsNotification returns true if this is a JSON-RPC notification (no id field).
func (r *JSONRPCRequest) IsNotification() bool {
	return r.ID == nil
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP-specific types.

type InitializeParams struct {
	ProtocolVersion string      `json:"protocolVersion"`
	Capabilities    interface{} `json:"capabilities"`
	ClientInfo      interface{} `json:"clientInfo"`
}

type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

type Capabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

type ToolsCapability struct{}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type ToolCallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type ContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
}

const (
	mcpProtocolVersion = "2025-03-26"
	serverName         = "paperless-mcp"
	serverVersion      = "0.1.0"
)

// MCPServer dispatches MCP/JSON-RPC methods.
type MCPServer struct {
	client *Client
	tools  *ToolRegistry
}

// NewMCPServer creates an MCP server backed by the given Paperless client.
func NewMCPServer(client *Client) *MCPServer {
	s := &MCPServer{client: client}
	s.tools = NewToolRegistry(client)
	return s
}

// HandleRequest processes a single JSON-RPC request and returns a response.
// Returns nil for notifications (no response expected).
func (s *MCPServer) HandleRequest(req *JSONRPCRequest) *JSONRPCResponse {
	log.Printf("mcp: method=%s", req.Method)

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		// Notification — no response.
		return nil
	case "ping":
		return s.makeResult(req, struct{}{})
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	default:
		return s.makeError(req, -32601, "Method not found", req.Method)
	}
}

// HandleMessage parses a raw JSON message that may be a single request or a batch,
// and returns the serialised JSON response(s).
func (s *MCPServer) HandleMessage(data []byte) ([]byte, int) {
	// Trim whitespace to determine if it's an array (batch) or object (single).
	trimmed := trimLeftSpace(data)
	if len(trimmed) == 0 {
		return mustJSON(&JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage("null"),
			Error:   &RPCError{Code: -32700, Message: "Parse error"},
		}), 200
	}

	if trimmed[0] == '[' {
		return s.handleBatch(data)
	}
	return s.handleSingle(data)
}

func (s *MCPServer) handleSingle(data []byte) ([]byte, int) {
	var req JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return mustJSON(&JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage("null"),
			Error:   &RPCError{Code: -32700, Message: "Parse error"},
		}), 200
	}

	resp := s.HandleRequest(&req)
	if resp == nil {
		// Notification — 202 Accepted, no body.
		return nil, 202
	}
	return mustJSON(resp), 200
}

func (s *MCPServer) handleBatch(data []byte) ([]byte, int) {
	var reqs []JSONRPCRequest
	if err := json.Unmarshal(data, &reqs); err != nil {
		return mustJSON(&JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage("null"),
			Error:   &RPCError{Code: -32700, Message: "Parse error"},
		}), 200
	}

	if len(reqs) == 0 {
		return mustJSON(&JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage("null"),
			Error:   &RPCError{Code: -32600, Message: "Invalid Request: empty batch"},
		}), 200
	}

	var responses []JSONRPCResponse
	for i := range reqs {
		resp := s.HandleRequest(&reqs[i])
		if resp != nil {
			responses = append(responses, *resp)
		}
	}

	if len(responses) == 0 {
		// All were notifications.
		return nil, 202
	}

	return mustJSON(responses), 200
}

func (s *MCPServer) handleInitialize(req *JSONRPCRequest) *JSONRPCResponse {
	return s.makeResult(req, InitializeResult{
		ProtocolVersion: mcpProtocolVersion,
		Capabilities: Capabilities{
			Tools: &ToolsCapability{},
		},
		ServerInfo: ServerInfo{
			Name:    serverName,
			Version: serverVersion,
		},
	})
}

func (s *MCPServer) handleToolsList(req *JSONRPCRequest) *JSONRPCResponse {
	return s.makeResult(req, s.tools.List())
}

func (s *MCPServer) handleToolsCall(req *JSONRPCRequest) *JSONRPCResponse {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.makeError(req, -32602, "Invalid params", err.Error())
	}

	result, err := s.tools.Call(params.Name, params.Arguments)
	if err != nil {
		return s.makeError(req, -32603, "Tool execution failed", err.Error())
	}

	return s.makeResult(req, result)
}

func (s *MCPServer) makeResult(req *JSONRPCRequest, result interface{}) *JSONRPCResponse {
	id := json.RawMessage("null")
	if req.ID != nil {
		id = *req.ID
	}
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

func (s *MCPServer) makeError(req *JSONRPCRequest, code int, message string, data interface{}) *JSONRPCResponse {
	id := json.RawMessage("null")
	if req.ID != nil {
		id = *req.ID
	}
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message, Data: data},
	}
}

func mustJSON(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustJSON: %v", err))
	}
	return data
}

func trimLeftSpace(data []byte) []byte {
	for i, b := range data {
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			return data[i:]
		}
	}
	return nil
}
