// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_http

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/httpclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

const (
	oauth2ExpirySkew = 30 * time.Second
	// oauth2DefaultLifetime applies when the token response omits expires_in.
	oauth2DefaultLifetime       = 5 * time.Minute
	maxOAuth2TokenResponseBytes = 1024 * 1024
)

// oauth2Tokens caches client-credentials access tokens for the life of the
// runner. Tokens never leave the runner: they are fetched here with the
// connection's client secret and added to requests for the connection's host.
type oauth2Tokens struct {
	mu      sync.Mutex
	entries map[[sha256.Size]byte]*oauth2Entry
	now     func() time.Time
}

type oauth2Entry struct {
	// mu makes concurrent tasks share one token request per client.
	mu          sync.Mutex
	accessToken string
	expiry      time.Time
}

func newOAuth2Tokens() *oauth2Tokens {
	return &oauth2Tokens{entries: map[[sha256.Size]byte]*oauth2Entry{}, now: time.Now}
}

func oauth2Key(details *privateconnection.OAuth2Details) [sha256.Size]byte {
	// The secret is part of the key so a rotated secret gets a new token.
	return sha256.Sum256([]byte(strings.Join([]string{
		details.TokenURL, details.ClientID, details.ClientSecret,
		strings.Join(details.Scopes, " "), details.ClientAuthMethod,
	}, "\x00")))
}

func (o *oauth2Tokens) entry(details *privateconnection.OAuth2Details) *oauth2Entry {
	o.mu.Lock()
	defer o.mu.Unlock()
	key := oauth2Key(details)
	if o.entries[key] == nil {
		o.entries[key] = &oauth2Entry{}
	}
	return o.entries[key]
}

// invalidate drops the cached token after the server rejected it.
func (o *oauth2Tokens) invalidate(details *privateconnection.OAuth2Details) {
	if details == nil {
		return
	}
	entry := o.entry(details)
	entry.mu.Lock()
	entry.accessToken = ""
	entry.mu.Unlock()
}

// oauth2Authorization returns the Authorization header value for an OAuth2
// connection, requesting a token from the identity provider when none is cached.
func (h *Handler) oauth2Authorization(ctx context.Context, details *privateconnection.OAuth2Details) (string, error) {
	if err := validateOAuth2Details(details); err != nil {
		return "", err
	}
	entry := h.oauth2.entry(details)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.accessToken != "" && h.oauth2.now().Before(entry.expiry) {
		return "Bearer " + entry.accessToken, nil
	}
	accessToken, lifetime, err := h.requestClientCredentialsToken(ctx, details)
	if err != nil {
		return "", err
	}
	entry.accessToken = accessToken
	entry.expiry = tokenExpiry(h.oauth2.now(), lifetime)
	return "Bearer " + accessToken, nil
}

// tokenExpiry renews oauth2ExpirySkew early, or halfway through a token shorter than twice that.
func tokenExpiry(now time.Time, lifetime time.Duration) time.Time {
	skew := oauth2ExpirySkew
	if lifetime < 2*skew {
		skew = lifetime / 2
	}
	return now.Add(lifetime - skew)
}

func validateOAuth2Details(details *privateconnection.OAuth2Details) error {
	if details.GrantType != privateconnection.OAuth2ClientCredentialsGrant {
		return fmt.Errorf("unsupported OAuth2 grant type %q on the runner", details.GrantType)
	}
	if details.ClientID == "" || details.ClientSecret == "" {
		return errors.New("OAuth2 connection requires a client ID and a client secret")
	}
	tokenURL, err := url.Parse(details.TokenURL)
	if err != nil || tokenURL.Scheme != "https" || tokenURL.Host == "" || tokenURL.User != nil {
		return errors.New("OAuth2 token URL must be an absolute https URL")
	}
	switch details.ClientAuthMethod {
	case "", privateconnection.OAuth2ClientSecretBasic, privateconnection.OAuth2ClientSecretPost:
	default:
		return fmt.Errorf("unsupported OAuth2 client authentication method %q", details.ClientAuthMethod)
	}
	return nil
}

type oauth2TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Error       string `json:"error"`
}

// requestClientCredentialsToken runs the RFC 6749 section 4.4 token request
// through the runner's own HTTP client, so the runner allowlist and the cloud
// metadata blocker apply, and redirects are never followed.
func (h *Handler) requestClientCredentialsToken(ctx context.Context, details *privateconnection.OAuth2Details) (string, time.Duration, error) {
	form := url.Values{"grant_type": {privateconnection.OAuth2ClientCredentialsGrant}}
	if len(details.Scopes) > 0 {
		form.Set("scope", strings.Join(details.Scopes, " "))
	}
	if details.ClientAuthMethod == privateconnection.OAuth2ClientSecretPost {
		form.Set("client_id", details.ClientID)
		form.Set("client_secret", details.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, details.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, errors.New("cannot build the OAuth2 token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Datadog")
	if details.ClientAuthMethod != privateconnection.OAuth2ClientSecretPost {
		// RFC 6749 section 2.3.1 form-encodes both parts before Basic encoding.
		req.SetBasicAuth(url.QueryEscape(details.ClientID), url.QueryEscape(details.ClientSecret))
	}

	client, err := h.httpClientProvider.NewClient(&httpclient.RunnerHttpClientConfig{
		AllowIMDSEndpoints: h.allowIMDSEndpoints,
		HTTPTimeout:        h.httpTimeout,
		DisableRedirects:   true,
	})
	if err != nil {
		return "", 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("OAuth2 token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOAuth2TokenResponseBytes))
	if err != nil {
		return "", 0, errors.New("cannot read the OAuth2 token response")
	}
	var token oauth2TokenResponse
	_ = json.Unmarshal(body, &token)
	if resp.StatusCode != http.StatusOK {
		if token.Error != "" {
			return "", 0, fmt.Errorf("OAuth2 token request returned status %d: %s", resp.StatusCode, token.Error)
		}
		return "", 0, fmt.Errorf("OAuth2 token request returned status %d", resp.StatusCode)
	}
	if token.AccessToken == "" {
		return "", 0, errors.New("OAuth2 token response has no access_token")
	}
	if !strings.EqualFold(token.TokenType, "bearer") {
		return "", 0, fmt.Errorf("unsupported OAuth2 token type %q", token.TokenType)
	}
	lifetime := oauth2DefaultLifetime
	if token.ExpiresIn > 0 {
		lifetime = time.Duration(token.ExpiresIn) * time.Second
	}
	return token.AccessToken, lifetime, nil
}
