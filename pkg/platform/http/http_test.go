package http

import (
	"testing"

	agshctx "github.com/cgast/agsh/pkg/context"
)

func TestCheckAllowedDomain(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		allowedDomains []string
		wantErr        bool
	}{
		{
			name:           "allowed domain",
			url:            "https://api.github.com/repos",
			allowedDomains: []string{"api.github.com", "httpbin.org"},
			wantErr:        false,
		},
		{
			name:           "blocked domain",
			url:            "https://evil.com/hack",
			allowedDomains: []string{"api.github.com"},
			wantErr:        true,
		},
		{
			name:           "empty allowlist permits all",
			url:            "https://anything.com/path",
			allowedDomains: nil,
			wantErr:        false,
		},
		{
			name:           "invalid URL",
			url:            "://bad",
			allowedDomains: []string{"bad"},
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkAllowedDomain(tt.url, tt.allowedDomains)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestExtractHTTPParams(t *testing.T) {
	tests := []struct {
		name    string
		payload any
		wantURL string
		wantErr bool
	}{
		{
			name:    "string payload",
			payload: "https://example.com",
			wantURL: "https://example.com",
		},
		{
			name:    "map payload",
			payload: map[string]any{"url": "https://example.com"},
			wantURL: "https://example.com",
		},
		{
			name:    "map with headers",
			payload: map[string]any{"url": "https://example.com", "headers": map[string]any{"Accept": "application/json"}},
			wantURL: "https://example.com",
		},
		{
			name:    "empty string",
			payload: "",
			wantErr: true,
		},
		{
			name:    "nil payload",
			payload: nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := agshctx.NewEnvelope(tt.payload, "text/plain", "test")
			url, _, err := extractHTTPParams(env)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got url=%q", url)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if url != tt.wantURL {
				t.Errorf("url = %q, want %q", url, tt.wantURL)
			}
		})
	}
}

func TestCommandIdentity(t *testing.T) {
	get := NewGetCommand(nil)
	if get.Name() != "http:get" {
		t.Errorf("GetCommand.Name() = %q", get.Name())
	}
	if get.Namespace() != "http" {
		t.Errorf("GetCommand.Namespace() = %q", get.Namespace())
	}
	if len(get.RequiredCredentials()) != 0 {
		t.Errorf("GetCommand.RequiredCredentials() = %v", get.RequiredCredentials())
	}

	post := NewPostCommand(nil)
	if post.Name() != "http:post" {
		t.Errorf("PostCommand.Name() = %q", post.Name())
	}
}

func TestExtractPostParams(t *testing.T) {
	tests := []struct {
		name            string
		payload         any
		wantURL         string
		wantBody        string
		wantContentType string
		wantErr         bool
	}{
		{
			name:            "full payload",
			payload:         map[string]any{"url": "https://example.com", "body": `{"key":"val"}`, "content_type": "application/json"},
			wantURL:         "https://example.com",
			wantBody:        `{"key":"val"}`,
			wantContentType: "application/json",
		},
		{
			name:            "minimal payload",
			payload:         map[string]any{"url": "https://example.com"},
			wantURL:         "https://example.com",
			wantBody:        "",
			wantContentType: "application/json",
		},
		{
			name:    "missing url",
			payload: map[string]any{"body": "hello"},
			wantErr: true,
		},
		{
			name:    "string payload",
			payload: "https://example.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := agshctx.NewEnvelope(tt.payload, "text/plain", "test")
			url, body, ct, _, err := extractPostParams(env)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if url != tt.wantURL {
				t.Errorf("url = %q, want %q", url, tt.wantURL)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
			if ct != tt.wantContentType {
				t.Errorf("content_type = %q, want %q", ct, tt.wantContentType)
			}
		})
	}
}
