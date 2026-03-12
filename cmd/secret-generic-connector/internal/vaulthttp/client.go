// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package vaulthttp provides a minimal HTTP client for Hashicorp Vault,
// replacing the vault/api SDK with raw HTTP calls.
package vaulthttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Secret is the response from a Vault read or write operation.
type Secret struct {
	RequestID     string                 `json:"request_id"`
	LeaseID       string                 `json:"lease_id"`
	Renewable     bool                   `json:"renewable"`
	LeaseDuration int                    `json:"lease_duration"`
	Data          map[string]interface{} `json:"data"`
	Warnings      []string               `json:"warnings"`
	Auth          *SecretAuth            `json:"auth"`
}

// SecretAuth contains the authentication information from a Vault login.
type SecretAuth struct {
	ClientToken string `json:"client_token"`
}

// TokenID returns the client token from a Vault auth response.
func (s *Secret) TokenID() (string, error) {
	if s == nil {
		return "", errors.New("nil secret")
	}
	if s.Auth == nil {
		return "", errors.New("secret has no auth information")
	}
	return s.Auth.ClientToken, nil
}

// MountOutput describes a mounted secrets engine.
type MountOutput struct {
	Type    string            `json:"type"`
	Options map[string]string `json:"options"`
}

// TLSConfig holds TLS configuration for the Vault client.
type TLSConfig struct {
	CACert        string
	CAPath        string
	ClientCert    string
	ClientKey     string
	TLSServerName string
	Insecure      bool
}

// Client is a minimal HTTP client for Vault.
type Client struct {
	addr       string
	token      string
	httpClient *http.Client
}

// NewClient creates a new Vault HTTP client.
func NewClient(addr string, tlsCfg *TLSConfig) (*Client, error) {
	httpClient := &http.Client{}

	if tlsCfg != nil {
		tlsConfig, err := buildTLSConfig(tlsCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to configure TLS: %w", err)
		}
		httpClient.Transport = &http.Transport{TLSClientConfig: tlsConfig}
	}

	return &Client{
		addr:       strings.TrimRight(addr, "/"),
		httpClient: httpClient,
	}, nil
}

// SetToken sets the Vault token used for requests.
func (c *Client) SetToken(token string) {
	c.token = token
}

// Token returns the current Vault token.
func (c *Client) Token() string {
	return c.token
}

// Address returns the Vault server address.
func (c *Client) Address() string {
	return c.addr
}

// Read performs a GET request to Vault (equivalent to Logical().Read).
func (c *Client) Read(ctx context.Context, path string) (*Secret, error) {
	url := fmt.Sprintf("%s/v1/%s", c.addr, strings.TrimPrefix(path, "/"))
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	return c.doVaultRequest(req)
}

// Write performs a POST request to Vault (equivalent to Logical().Write).
func (c *Client) Write(ctx context.Context, path string, data map[string]interface{}) (*Secret, error) {
	url := fmt.Sprintf("%s/v1/%s", c.addr, strings.TrimPrefix(path, "/"))

	var body io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		body = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.doVaultRequest(req)
}

// ListMounts returns the mounted secrets engines (GET /v1/sys/mounts).
func (c *Client) ListMounts(ctx context.Context) (map[string]*MountOutput, error) {
	url := c.addr + "/v1/sys/mounts"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseVaultError(resp.StatusCode, respBody)
	}

	// The sys/mounts response has mount paths as top-level keys, each containing
	// the mount info. There are also other keys like "request_id", etc.
	// We parse the raw JSON and extract only mount entries.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse mounts response: %w", err)
	}

	result := make(map[string]*MountOutput)
	for key, val := range raw {
		// Skip known non-mount keys.
		switch key {
		case "request_id", "lease_id", "renewable", "lease_duration", "data", "wrap_info", "warnings", "auth":
			continue
		}
		var mount MountOutput
		if err := json.Unmarshal(val, &mount); err != nil {
			continue // skip entries that don't parse as a mount
		}
		if mount.Type != "" {
			result[key] = &mount
		}
	}

	return result, nil
}

func (c *Client) setHeaders(req *http.Request) {
	if c.token != "" {
		req.Header.Set("X-Vault-Token", c.token)
	}
}

func (c *Client) doVaultRequest(req *http.Request) (*Secret, error) {
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Vault returns 404 for non-existent secrets. The official SDK translates
	// this to a nil *Secret (no error). We do the same for compatibility.
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseVaultError(resp.StatusCode, body)
	}

	// Vault returns 200 with empty body for some operations (e.g. write to KV v1).
	if len(body) == 0 {
		return &Secret{}, nil
	}

	var secret Secret
	if err := json.Unmarshal(body, &secret); err != nil {
		return nil, fmt.Errorf("failed to parse Vault response: %w", err)
	}

	return &secret, nil
}

// vaultErrorResponse is the error format returned by Vault.
type vaultErrorResponse struct {
	Errors []string `json:"errors"`
}

func parseVaultError(statusCode int, body []byte) error {
	var errResp vaultErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && len(errResp.Errors) > 0 {
		return fmt.Errorf("vault returned %d: %s", statusCode, strings.Join(errResp.Errors, ", "))
	}
	return fmt.Errorf("vault returned %d: %s", statusCode, string(body))
}

func buildTLSConfig(cfg *TLSConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: cfg.Insecure, //nolint:gosec // user-configured
		ServerName:         cfg.TLSServerName,
	}

	// Load CA certificates.
	if cfg.CACert != "" || cfg.CAPath != "" {
		rootCAs := x509.NewCertPool()

		if cfg.CACert != "" {
			pem, err := os.ReadFile(cfg.CACert)
			if err != nil {
				return nil, fmt.Errorf("failed to read CA cert %s: %w", cfg.CACert, err)
			}
			if !rootCAs.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("failed to parse CA cert %s", cfg.CACert)
			}
		}

		if cfg.CAPath != "" {
			entries, err := os.ReadDir(cfg.CAPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read CA path %s: %w", cfg.CAPath, err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				pem, err := os.ReadFile(filepath.Join(cfg.CAPath, entry.Name()))
				if err != nil {
					continue
				}
				rootCAs.AppendCertsFromPEM(pem)
			}
		}

		tlsConfig.RootCAs = rootCAs
	}

	// Load client certificate.
	if cfg.ClientCert != "" && cfg.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert/key: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}
