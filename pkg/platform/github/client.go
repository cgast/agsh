package github

import (
	"fmt"
	"net/http"

	gh "github.com/google/go-github/v60/github"
)

// Client wraps the GitHub API client with token authentication.
type Client struct {
	inner *gh.Client
	token string
}

// NewClient creates a GitHub API client with the given token.
func NewClient(token string) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("github token is required")
	}
	httpClient := &http.Client{
		Transport: &tokenTransport{token: token},
	}
	client := gh.NewClient(httpClient)
	return &Client{inner: client, token: token}, nil
}

// tokenTransport adds Bearer token auth to HTTP requests.
type tokenTransport struct {
	token string
}

func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(req)
}
