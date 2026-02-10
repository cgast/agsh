package protocol

import (
	"encoding/json"
	"testing"
)

func TestHandlerMethodNotFound(t *testing.T) {
	h := NewHandler()
	req := Request{JSONRPC: "2.0", ID: 1, Method: "nonexistent"}

	resp := h.Handle(req)
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != CodeMethodNotFound {
		t.Errorf("Code = %d, want %d", resp.Error.Code, CodeMethodNotFound)
	}
}

func TestHandlerInvalidVersion(t *testing.T) {
	h := NewHandler()
	req := Request{JSONRPC: "1.0", ID: 1, Method: "test"}

	resp := h.Handle(req)
	if resp.Error == nil {
		t.Fatal("expected error for invalid version")
	}
	if resp.Error.Code != CodeInvalidRequest {
		t.Errorf("Code = %d, want %d", resp.Error.Code, CodeInvalidRequest)
	}
}

func TestHandlerSuccess(t *testing.T) {
	h := NewHandler()
	h.Register("echo", func(params json.RawMessage) (any, *Error) {
		return map[string]string{"echo": string(params)}, nil
	})

	req := Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "echo",
		Params:  json.RawMessage(`"hello"`),
	}

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	result, ok := resp.Result.(map[string]string)
	if !ok {
		t.Fatalf("unexpected result type: %T", resp.Result)
	}
	if result["echo"] != `"hello"` {
		t.Errorf("echo = %q", result["echo"])
	}
}

func TestHandlerError(t *testing.T) {
	h := NewHandler()
	h.Register("fail", func(params json.RawMessage) (any, *Error) {
		return nil, &Error{Code: CodeCommandFailed, Message: "boom"}
	})

	req := Request{JSONRPC: "2.0", ID: 2, Method: "fail"}
	resp := h.Handle(req)

	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != CodeCommandFailed {
		t.Errorf("Code = %d", resp.Error.Code)
	}
	if resp.ID != 2 {
		t.Errorf("ID = %v", resp.ID)
	}
}

func TestHandleRaw(t *testing.T) {
	h := NewHandler()
	h.Register("ping", func(params json.RawMessage) (any, *Error) {
		return "pong", nil
	})

	raw := []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	resp := h.HandleRaw(raw)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.Result != "pong" {
		t.Errorf("Result = %v", resp.Result)
	}
}

func TestHandleRawParseError(t *testing.T) {
	h := NewHandler()
	resp := h.HandleRaw([]byte(`{invalid json`))

	if resp.Error == nil {
		t.Fatal("expected parse error")
	}
	if resp.Error.Code != CodeParseError {
		t.Errorf("Code = %d", resp.Error.Code)
	}
}

func TestParseParams(t *testing.T) {
	raw := json.RawMessage(`{"command":"fs:list","args":{"path":"."}}`)
	params, err := ParseParams[ExecuteParams](raw)
	if err != nil {
		t.Fatalf("ParseParams: %v", err)
	}
	if params.Command != "fs:list" {
		t.Errorf("Command = %q", params.Command)
	}
}

func TestParseParamsNil(t *testing.T) {
	params, err := ParseParams[ExecuteParams](nil)
	if err != nil {
		t.Fatalf("ParseParams(nil): %v", err)
	}
	if params.Command != "" {
		t.Errorf("expected empty command, got %q", params.Command)
	}
}

func TestParseParamsInvalid(t *testing.T) {
	_, err := ParseParams[ExecuteParams](json.RawMessage(`"not an object"`))
	if err == nil {
		t.Fatal("expected error for invalid params")
	}
	if err.Code != CodeInvalidParams {
		t.Errorf("Code = %d", err.Code)
	}
}

func TestHandlerMethods(t *testing.T) {
	h := NewHandler()
	h.Register("a", func(params json.RawMessage) (any, *Error) { return nil, nil })
	h.Register("b", func(params json.RawMessage) (any, *Error) { return nil, nil })

	methods := h.Methods()
	if len(methods) != 2 {
		t.Errorf("Methods() len = %d, want 2", len(methods))
	}
}
