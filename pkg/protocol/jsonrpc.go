package protocol

import "encoding/json"

// JSON-RPC 2.0 message types for agent mode communication.

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"` // string or int; nil for notifications
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

// Error is a JSON-RPC 2.0 error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}

// Standard JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Application-specific error codes.
const (
	CodeCommandNotFound = -32000
	CodeCommandFailed   = -32001
	CodeVerifyFailed    = -32002
	CodeSpecInvalid     = -32003
	CodeNoPendingPlan   = -32004
)

// Method constants for all supported JSON-RPC methods.
const (
	// Core command execution.
	MethodExecute  = "execute"
	MethodPipeline = "pipeline"

	// Command discovery.
	MethodCommandsList    = "commands.list"
	MethodCommandsDescribe = "commands.describe"

	// Context store operations.
	MethodContextGet = "context.get"
	MethodContextSet = "context.set"

	// Checkpoint operations.
	MethodCheckpointSave    = "checkpoint.save"
	MethodCheckpointRestore = "checkpoint.restore"

	// Execution history.
	MethodHistory = "history"

	// Project lifecycle (spec-driven).
	MethodProjectLoad     = "project.load"
	MethodProjectPlan     = "project.plan"
	MethodProjectApprove  = "project.approve"
	MethodProjectReject   = "project.reject"
	MethodProjectRun      = "project.run"
	MethodProjectInit     = "project.init"
	MethodProjectValidate = "project.validate"
)

// NewResponse creates a successful response.
func NewResponse(id any, result any) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

// NewErrorResponse creates an error response.
func NewErrorResponse(id any, code int, message string, data any) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// Parameter types for common methods.

// ExecuteParams holds parameters for the "execute" method.
type ExecuteParams struct {
	Command string         `json:"command"`
	Args    map[string]any `json:"args,omitempty"`
	Intent  string         `json:"intent,omitempty"`
	Verify  []AssertionDef `json:"verify,omitempty"`
}

// AssertionDef defines an assertion in a JSON-RPC request.
type AssertionDef struct {
	Type     string `json:"type"`
	Target   string `json:"target,omitempty"`
	Expected any    `json:"expected,omitempty"`
}

// PipelineParams holds parameters for the "pipeline" method.
type PipelineParams struct {
	Steps []PipelineStepDef `json:"steps"`
}

// PipelineStepDef defines a step within a pipeline request.
type PipelineStepDef struct {
	Command string         `json:"command"`
	Args    map[string]any `json:"args,omitempty"`
	Intent  string         `json:"intent,omitempty"`
	Verify  []AssertionDef `json:"verify,omitempty"`
	OnError string         `json:"on_error,omitempty"`
}

// ContextGetParams holds parameters for "context.get".
type ContextGetParams struct {
	Scope string `json:"scope"`
	Key   string `json:"key"`
}

// ContextSetParams holds parameters for "context.set".
type ContextSetParams struct {
	Scope string `json:"scope"`
	Key   string `json:"key"`
	Value any    `json:"value"`
}

// CheckpointParams holds parameters for checkpoint operations.
type CheckpointParams struct {
	Name string `json:"name"`
}

// ProjectLoadParams holds parameters for "project.load".
type ProjectLoadParams struct {
	Path   string            `json:"path"`
	Params map[string]string `json:"params,omitempty"`
}

// ProjectPlanParams holds parameters for "project.plan" (optional overrides).
type ProjectPlanParams struct {
	// Empty: uses the currently loaded spec.
}

// ProjectApproveParams holds parameters for "project.approve".
type ProjectApproveParams struct {
	PlanID string `json:"plan_id,omitempty"`
}

// ProjectRejectParams holds parameters for "project.reject".
type ProjectRejectParams struct {
	PlanID   string `json:"plan_id,omitempty"`
	Feedback string `json:"feedback,omitempty"`
}

// CommandsDescribeParams holds parameters for "commands.describe".
type CommandsDescribeParams struct {
	Name string `json:"name"`
}

// ExecuteResult holds the result of a command execution.
type ExecuteResult struct {
	Payload      any                `json:"payload"`
	Meta         map[string]any     `json:"meta,omitempty"`
	Verification *VerificationInfo  `json:"verification,omitempty"`
	Provenance   []ProvenanceStep   `json:"provenance,omitempty"`
}

// VerificationInfo holds verification results in a response.
type VerificationInfo struct {
	Passed  bool              `json:"passed"`
	Results []AssertionOutput `json:"results"`
}

// AssertionOutput holds a single assertion result in a response.
type AssertionOutput struct {
	Type    string `json:"type"`
	Passed  bool   `json:"passed"`
	Actual  any    `json:"actual,omitempty"`
	Message string `json:"message,omitempty"`
}

// ProvenanceStep records a provenance entry in a response.
type ProvenanceStep struct {
	Command  string `json:"command"`
	Duration string `json:"duration,omitempty"`
	Status   string `json:"status"`
}

// CommandInfo describes a command in the commands.list response.
type CommandInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Namespace   string `json:"namespace"`
}

// CommandDetail describes a command with its schema in commands.describe response.
type CommandDetail struct {
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	Namespace    string     `json:"namespace"`
	InputSchema  SchemaInfo `json:"input_schema"`
	OutputSchema SchemaInfo `json:"output_schema"`
	Credentials  []string   `json:"required_credentials,omitempty"`
}

// SchemaInfo is a simplified schema representation for JSON-RPC responses.
type SchemaInfo struct {
	Type       string                    `json:"type"`
	Properties map[string]SchemaFieldInfo `json:"properties,omitempty"`
	Required   []string                  `json:"required,omitempty"`
}

// SchemaFieldInfo describes a field in a schema for JSON-RPC responses.
type SchemaFieldInfo struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}
