package http

import (
	gocontext "context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/platform"
)

// GetCommand implements http:get — performs an HTTP GET request with domain allowlisting.
type GetCommand struct {
	allowedDomains []string
	httpClient     *http.Client
}

// NewGetCommand creates a new http:get command with domain restrictions.
func NewGetCommand(allowedDomains []string) *GetCommand {
	return &GetCommand{
		allowedDomains: allowedDomains,
		httpClient:     &http.Client{},
	}
}

func (c *GetCommand) Name() string        { return "http:get" }
func (c *GetCommand) Description() string { return "Perform an HTTP GET request" }
func (c *GetCommand) Namespace() string   { return "http" }

func (c *GetCommand) InputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"url":     {Type: "string", Description: "URL to fetch"},
			"headers": {Type: "object", Description: "Optional HTTP headers"},
		},
		Required: []string{"url"},
	}
}

func (c *GetCommand) OutputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"status_code": {Type: "integer", Description: "HTTP status code"},
			"body":        {Type: "string", Description: "Response body"},
			"headers":     {Type: "object", Description: "Response headers"},
		},
	}
}

func (c *GetCommand) RequiredCredentials() []string { return nil }

func (c *GetCommand) Execute(ctx gocontext.Context, input agshctx.Envelope, _ agshctx.ContextStore) (agshctx.Envelope, error) {
	rawURL, headers, err := extractHTTPParams(input)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("http:get: %w", err)
	}

	if err := checkAllowedDomain(rawURL, c.allowedDomains); err != nil {
		return agshctx.Envelope{}, fmt.Errorf("http:get: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("http:get: create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("http:get: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("http:get: read body: %w", err)
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

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain"
	}

	env := agshctx.NewEnvelope(result, contentType, "http:get")
	env.Meta.Tags["url"] = rawURL
	env.Meta.Tags["status"] = fmt.Sprintf("%d", resp.StatusCode)
	return env, nil
}

// extractHTTPParams gets URL and optional headers from the input envelope.
func extractHTTPParams(input agshctx.Envelope) (string, map[string]string, error) {
	headers := make(map[string]string)

	switch v := input.Payload.(type) {
	case string:
		if v == "" {
			return "", nil, fmt.Errorf("empty URL")
		}
		return v, headers, nil
	case map[string]any:
		rawURL, _ := v["url"].(string)
		if rawURL == "" {
			return "", nil, fmt.Errorf("missing 'url' in payload")
		}
		if h, ok := v["headers"].(map[string]any); ok {
			for k, val := range h {
				if s, ok := val.(string); ok {
					headers[k] = s
				}
			}
		}
		return rawURL, headers, nil
	}
	return "", nil, fmt.Errorf("cannot extract URL from payload type %T", input.Payload)
}

// checkAllowedDomain verifies the URL's domain is in the allowlist.
// If no allowed domains are configured, all domains are permitted.
func checkAllowedDomain(rawURL string, allowedDomains []string) error {
	if len(allowedDomains) == 0 {
		return nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}

	host := parsed.Hostname()
	for _, d := range allowedDomains {
		if host == d {
			return nil
		}
	}
	return fmt.Errorf("domain %q is not in the allowed list", host)
}
