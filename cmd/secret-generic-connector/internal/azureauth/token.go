// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package azureauth provides a managed identity token provider for Azure,
// replacing the azidentity SDK with raw HTTP calls.
package azureauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Token represents an Azure access token.
type Token struct {
	AccessToken string
	ExpiresOn   time.Time
}

// TokenProvider retrieves Azure access tokens.
type TokenProvider interface {
	GetToken(ctx context.Context, resource string) (Token, error)
}

type managedIdentityTokenProvider struct {
	clientID   string
	mu         sync.Mutex
	cached     Token
	httpClient *http.Client
}

// NewManagedIdentityTokenProvider returns a TokenProvider that acquires tokens
// using Azure managed identity (matching DefaultAzureCredential behaviour).
// If clientID is non-empty it is sent as `client_id` query parameter.
func NewManagedIdentityTokenProvider(clientID string) TokenProvider {
	return &managedIdentityTokenProvider{
		clientID:   clientID,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *managedIdentityTokenProvider) GetToken(ctx context.Context, resource string) (Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cached.AccessToken != "" && time.Until(p.cached.ExpiresOn) > 5*time.Minute {
		return p.cached, nil
	}

	tok, err := p.acquireToken(ctx, resource)
	if err != nil {
		return Token{}, err
	}
	p.cached = tok
	return tok, nil
}

// tokenResponse is the JSON shape returned by all Azure managed identity endpoints.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresOn   string `json:"expires_on"`
}

func parseTokenResponse(body []byte) (Token, error) {
	var resp tokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return Token{}, fmt.Errorf("failed to parse token response: %w", err)
	}
	if resp.AccessToken == "" {
		return Token{}, errors.New("empty access_token in response")
	}

	// expires_on can be a unix timestamp (number or string)
	expiresOn, err := strconv.ParseInt(resp.ExpiresOn, 10, 64)
	if err != nil {
		return Token{}, fmt.Errorf("failed to parse expires_on %q: %w", resp.ExpiresOn, err)
	}

	return Token{
		AccessToken: resp.AccessToken,
		ExpiresOn:   time.Unix(expiresOn, 0),
	}, nil
}

func (p *managedIdentityTokenProvider) acquireToken(ctx context.Context, resource string) (Token, error) {
	identityEndpoint := os.Getenv("IDENTITY_ENDPOINT")
	identityHeader := os.Getenv("IDENTITY_HEADER")
	msiEndpoint := os.Getenv("MSI_ENDPOINT")
	msiSecret := os.Getenv("MSI_SECRET")

	// 1. App Service
	if identityEndpoint != "" && identityHeader != "" {
		return p.appServiceToken(ctx, resource, identityEndpoint, identityHeader)
	}

	// 2. Azure ML
	if msiEndpoint != "" && msiSecret != "" {
		return p.azureMLToken(ctx, resource, msiEndpoint, msiSecret)
	}

	// 3. Cloud Shell
	if msiEndpoint != "" {
		return p.cloudShellToken(ctx, resource, msiEndpoint)
	}

	// 4. Azure Arc
	if identityEndpoint != "" {
		return p.arcToken(ctx, resource, identityEndpoint)
	}

	// 5. IMDS (fallback)
	return p.imdsToken(ctx, resource)
}

func (p *managedIdentityTokenProvider) clientIDParam() string {
	if p.clientID != "" {
		return "&client_id=" + url.QueryEscape(p.clientID)
	}
	return ""
}

func (p *managedIdentityTokenProvider) appServiceToken(ctx context.Context, resource, endpoint, header string) (Token, error) {
	reqURL := fmt.Sprintf("%s?api-version=2019-08-01&resource=%s%s", endpoint, url.QueryEscape(resource), p.clientIDParam())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("X-IDENTITY-HEADER", header)

	return p.doTokenRequest(req)
}

func (p *managedIdentityTokenProvider) azureMLToken(ctx context.Context, resource, endpoint, secret string) (Token, error) {
	reqURL := fmt.Sprintf("%s?api-version=2017-09-01&resource=%s%s", endpoint, url.QueryEscape(resource), p.clientIDParam())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Secret", secret)

	return p.doTokenRequest(req)
}

func (p *managedIdentityTokenProvider) cloudShellToken(ctx context.Context, resource, endpoint string) (Token, error) {
	body := "resource=" + url.QueryEscape(resource)
	if p.clientID != "" {
		body += "&client_id=" + url.QueryEscape(p.clientID)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(body))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Metadata", "true")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return p.doTokenRequest(req)
}

func (p *managedIdentityTokenProvider) arcToken(ctx context.Context, resource, endpoint string) (Token, error) {
	reqURL := fmt.Sprintf("%s?api-version=2020-06-01&resource=%s%s", endpoint, url.QueryEscape(resource), p.clientIDParam())

	// First request: expect 401 with WWW-Authenticate header pointing to secret file.
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Metadata", "true")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("Azure Arc challenge request failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		return Token{}, fmt.Errorf("Azure Arc: expected 401, got %d", resp.StatusCode)
	}

	wwwAuth := resp.Header.Get("WWW-Authenticate")
	const prefix = "Basic realm="
	if !strings.HasPrefix(wwwAuth, prefix) {
		return Token{}, fmt.Errorf("Azure Arc: unexpected WWW-Authenticate header: %s", wwwAuth)
	}
	secretFilePath := strings.TrimPrefix(wwwAuth, prefix)

	secretContent, err := os.ReadFile(secretFilePath)
	if err != nil {
		return Token{}, fmt.Errorf("Azure Arc: failed to read secret file %s: %w", secretFilePath, err)
	}

	// Retry with Authorization header.
	req2, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return Token{}, err
	}
	req2.Header.Set("Metadata", "true")
	req2.Header.Set("Authorization", "Basic "+string(secretContent))

	return p.doTokenRequest(req2)
}

func (p *managedIdentityTokenProvider) imdsToken(ctx context.Context, resource string) (Token, error) {
	reqURL := fmt.Sprintf("http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=%s%s", url.QueryEscape(resource), p.clientIDParam())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Metadata", "true")

	return p.doTokenRequest(req)
}

func (p *managedIdentityTokenProvider) doTokenRequest(req *http.Request) (Token, error) {
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Token{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return Token{}, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return parseTokenResponse(body)
}
