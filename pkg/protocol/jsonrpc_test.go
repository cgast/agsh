package protocol

import (
	"encoding/json"
	"testing"
)

func TestRequestMarshal(t *testing.T) {
	req := Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  MethodExecute,
		Params:  json.RawMessage(`{"command":"fs:list"}`),
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Method != MethodExecute {
		t.Errorf("Method = %q, want %q", decoded.Method, MethodExecute)
	}
}

func TestResponseSuccess(t *testing.T) {
	resp := NewResponse(1, map[string]any{"data": "hello"})

	if resp.JSONRPC != "2.0" {
		t.Error("JSONRPC should be 2.0")
	}
	if resp.Error != nil {
		t.Error("Error should be nil for success response")
	}
	if resp.ID != 1 {
		t.Errorf("ID = %v, want 1", resp.ID)
	}
}

func TestResponseError(t *testing.T) {
	resp := NewErrorResponse(2, CodeMethodNotFound, "method not found", nil)

	if resp.Error == nil {
		t.Fatal("Error should not be nil")
	}
	if resp.Error.Code != CodeMethodNotFound {
		t.Errorf("Code = %d, want %d", resp.Error.Code, CodeMethodNotFound)
	}
	if resp.Error.Error() != "method not found" {
		t.Errorf("Message = %q", resp.Error.Message)
	}
}

func TestResponseMarshalRoundTrip(t *testing.T) {
	resp := NewResponse("abc", map[string]string{"status": "ok"})

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.ID != "abc" {
		t.Errorf("ID = %v, want %q", decoded.ID, "abc")
	}
}

func TestExecuteParamsMarshal(t *testing.T) {
	params := ExecuteParams{
		Command: "github:repo:info",
		Args:    map[string]any{"repo": "cgast/agsh"},
		Intent:  "Get repo info",
		Verify: []AssertionDef{
			{Type: "not_empty"},
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded ExecuteParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Command != "github:repo:info" {
		t.Errorf("Command = %q", decoded.Command)
	}
	if len(decoded.Verify) != 1 {
		t.Errorf("Verify len = %d", len(decoded.Verify))
	}
}

func TestMethodConstants(t *testing.T) {
	methods := []string{
		MethodExecute, MethodPipeline,
		MethodCommandsList, MethodCommandsDescribe,
		MethodContextGet, MethodContextSet,
		MethodCheckpointSave, MethodCheckpointRestore,
		MethodHistory,
		MethodProjectLoad, MethodProjectPlan,
		MethodProjectApprove, MethodProjectReject,
		MethodProjectRun, MethodProjectInit, MethodProjectValidate,
	}

	seen := make(map[string]bool)
	for _, m := range methods {
		if m == "" {
			t.Error("empty method constant")
		}
		if seen[m] {
			t.Errorf("duplicate method: %s", m)
		}
		seen[m] = true
	}

	if len(methods) != 16 {
		t.Errorf("expected 16 methods, got %d", len(methods))
	}
}

func TestErrorResponseWithData(t *testing.T) {
	resp := NewErrorResponse(1, CodeCommandFailed, "exec failed", map[string]string{
		"command": "fs:read",
		"detail":  "file not found",
	})

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Error.Code != CodeCommandFailed {
		t.Errorf("Code = %d", decoded.Error.Code)
	}
}
