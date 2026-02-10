package protocol

import (
	"encoding/json"
	"fmt"
	"sync"
)

// HandlerFunc processes a JSON-RPC request and returns a result or error.
type HandlerFunc func(params json.RawMessage) (any, *Error)

// Handler routes JSON-RPC methods to registered handler functions.
type Handler struct {
	mu       sync.RWMutex
	handlers map[string]HandlerFunc
}

// NewHandler creates an empty method handler.
func NewHandler() *Handler {
	return &Handler{
		handlers: make(map[string]HandlerFunc),
	}
}

// Register adds a handler for a method. Overwrites any existing handler.
func (h *Handler) Register(method string, fn HandlerFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[method] = fn
}

// Handle processes a single JSON-RPC request and returns a response.
func (h *Handler) Handle(req Request) Response {
	if req.JSONRPC != "2.0" {
		return NewErrorResponse(req.ID, CodeInvalidRequest, "invalid jsonrpc version", nil)
	}

	h.mu.RLock()
	fn, ok := h.handlers[req.Method]
	h.mu.RUnlock()

	if !ok {
		return NewErrorResponse(req.ID, CodeMethodNotFound,
			fmt.Sprintf("method not found: %s", req.Method), nil)
	}

	result, rpcErr := fn(req.Params)
	if rpcErr != nil {
		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   rpcErr,
		}
	}

	return NewResponse(req.ID, result)
}

// HandleRaw parses raw JSON bytes as a request, processes it, and returns a response.
func (h *Handler) HandleRaw(data []byte) Response {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return NewErrorResponse(nil, CodeParseError, "parse error: "+err.Error(), nil)
	}
	return h.Handle(req)
}

// Methods returns all registered method names.
func (h *Handler) Methods() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	methods := make([]string, 0, len(h.handlers))
	for m := range h.handlers {
		methods = append(methods, m)
	}
	return methods
}

// ParseParams is a helper to unmarshal JSON-RPC params into a typed struct.
func ParseParams[T any](params json.RawMessage) (T, *Error) {
	var p T
	if len(params) == 0 || string(params) == "null" {
		return p, nil
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return p, &Error{
			Code:    CodeInvalidParams,
			Message: fmt.Sprintf("invalid params: %v", err),
		}
	}
	return p, nil
}
