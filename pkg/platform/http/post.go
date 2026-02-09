package http

import (
	gocontext "context"
	"fmt"
	"io"
	"net/http"
	"strings"

	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/platform"
)

// PostCommand implements http:post — performs an HTTP POST request with domain allowlisting.
type PostCommand struct {
	allowedDomains []string
	httpClient     *http.Client
}

// NewPostCommand creates a new http:post command with domain restrictions.
func NewPostCommand(allowedDomains []string) *PostCommand {
	return &PostCommand{
		allowedDomains: allowedDomains,
		httpClient:     &http.Client{},
	}
}

func (c *PostCommand) Name() string        { return "http:post" }
func (c *PostCommand) Description() string { return "Perform an HTTP POST request" }
func (c *PostCommand) Namespace() string   { return "http" }

func (c *PostCommand) InputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"url":          {Type: "string", Description: "URL to post to"},
			"body":         {Type: "string", Description: "Request body"},
			"content_type": {Type: "string", Description: "Content-Type header (default: application/json)"},
			"headers":      {Type: "object", Description: "Optional HTTP headers"},
		},
		Required: []string{"url"},
	}
}

func (c *PostCommand) OutputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"status_code": {Type: "integer", Description: "HTTP status code"},
			"body":        {Type: "string", Description: "Response body"},
			"headers":     {Type: "object", Description: "Response headers"},
		},
	}
}

func (c *PostCommand) RequiredCredentials() []string { return nil }

func (c *PostCommand) Execute(ctx gocontext.Context, input agshctx.Envelope, _ agshctx.ContextStore) (agshctx.Envelope, error) {
	rawURL, reqBody, contentType, headers, err := extractPostParams(input)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("http:post: %w", err)
	}

	if err := checkAllowedDomain(rawURL, c.allowedDomains); err != nil {
		return agshctx.Envelope{}, fmt.Errorf("http:post: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(reqBody))
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("http:post: create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("http:post: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("http:post: read body: %w", err)
	}

	respHeaders := make(map[string]string)
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}

	result := map[string]any{
		"status_code": resp.StatusCode,
		"body":        string(body),
		"headers":     respHeaders,
	}

	respContentType := resp.Header.Get("Content-Type")
	if respContentType == "" {
		respContentType = "text/plain"
	}

	env := agshctx.NewEnvelope(result, respContentType, "http:post")
	env.Meta.Tags["url"] = rawURL
	env.Meta.Tags["status"] = fmt.Sprintf("%d", resp.StatusCode)
	return env, nil
}

// extractPostParams gets URL, body, content type, and headers from the input envelope.
func extractPostParams(input agshctx.Envelope) (string, string, string, map[string]string, error) {
	headers := make(map[string]string)

	m, ok := input.Payload.(map[string]any)
	if !ok {
		return "", "", "", nil, fmt.Errorf("expected map payload with 'url' and 'body' keys, got %T", input.Payload)
	}

	rawURL, _ := m["url"].(string)
	if rawURL == "" {
		return "", "", "", nil, fmt.Errorf("missing 'url' in payload")
	}

	body, _ := m["body"].(string)

	contentType := "application/json"
	if ct, ok := m["content_type"].(string); ok && ct != "" {
		contentType = ct
	}

	if h, ok := m["headers"].(map[string]any); ok {
		for k, val := range h {
			if s, ok := val.(string); ok {
				headers[k] = s
			}
		}
	}

	return rawURL, body, contentType, headers, nil
}
