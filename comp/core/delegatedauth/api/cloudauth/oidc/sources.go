// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
)

const (
	gcpIdentityURL   = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity"
	azureIdentityURL = "http://169.254.169.254/metadata/identity/oauth2/token"
)

// FileSource reads a JWT maintained by kubelet or an external credential helper.
// The file must be rotated atomically. Mount the containing directory (not a
// Kubernetes subPath) so replacements remain visible.
type FileSource struct{ Path string }

// Token reads the current file. Its producer is responsible for requesting the
// intended audience; a file source cannot change claims in an existing JWT.
func (s *FileSource) Token(ctx context.Context, _ string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Reject special files before opening, since opening a FIFO can block.
	info, err := os.Stat(s.Path)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("OIDC token file must be a readable regular file")
	}
	f, err := os.Open(s.Path)
	if err != nil {
		return "", errors.New("cannot open OIDC token file")
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("OIDC token file must be a regular file")
	}
	b, err := readLimited(f)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return string(b), nil
}

// metadataSource uses a fixed cloud endpoint, bypasses proxies, rejects
// redirects, and never includes a response body in an error.
type metadataSource struct {
	client   *http.Client
	provider string
	clientID string
}

func (s *metadataSource) Token(ctx context.Context, audience string) (string, error) {
	endpoint := gcpIdentityURL
	q := url.Values{"audience": {audience}, "format": {"full"}}
	if s.provider == "azure" {
		endpoint = azureIdentityURL
		q = url.Values{"api-version": {"2018-02-01"}, "resource": {audience}}
		if s.clientID != "" {
			q.Set("client_id", s.clientID)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return "", errors.New("cannot create OIDC metadata request")
	}
	if s.provider == "azure" {
		req.Header.Set("Metadata", "true")
	} else {
		req.Header.Set("Metadata-Flavor", "Google")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errors.New("OIDC metadata request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OIDC metadata endpoint returned HTTP %d", resp.StatusCode)
	}
	body, err := readLimited(resp.Body)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", err
	}
	if s.provider == "gcp" {
		return string(body), nil
	}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(body, &result) != nil {
		return "", errors.New("invalid OIDC metadata response")
	}
	return result.AccessToken, nil
}

func readLimited(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxTokenSize+1))
	if err != nil {
		return nil, errors.New("cannot read OIDC token response")
	}
	if len(b) > maxTokenSize {
		return nil, errors.New("OIDC token response exceeds 64 KiB")
	}
	return b, nil
}

// FromConfig creates one of the built-in token sources. Third parties can use
// FileSource without rebuilding the Agent, or implement TokenSource in Go.
func FromConfig(c *cloudauthconfig.OIDCProviderConfig) (*Auth, error) {
	if c == nil {
		return nil, errors.New("OIDC configuration is required")
	}
	var source TokenSource
	switch c.Source {
	case "file":
		if !filepath.IsAbs(c.TokenFile) {
			return nil, errors.New("OIDC token_file must be an absolute path")
		}
		if c.ClientID != "" {
			return nil, errors.New("OIDC client_id is only supported for azure")
		}
		source = &FileSource{Path: c.TokenFile}
	case "gcp", "azure":
		if c.TokenFile != "" {
			return nil, errors.New("OIDC token_file is only supported for file sources")
		}
		if c.Source == "gcp" && c.ClientID != "" {
			return nil, errors.New("OIDC client_id is only supported for azure")
		}
		if c.Source == "azure" && c.Audience == "" {
			return nil, errors.New("Azure OIDC requires an audience (Entra application resource URI)")
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		source = &metadataSource{provider: c.Source, clientID: c.ClientID, client: &http.Client{
			Transport: transport, Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}}
	default:
		return nil, errors.New("OIDC source must be file, gcp, or azure")
	}
	return New(source, c.Audience, c.Source, c.OrgUUIDSource)
}
