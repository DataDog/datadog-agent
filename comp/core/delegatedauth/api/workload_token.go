// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// GetWorkloadToken exchanges a fresh, request-bound proof for an opaque workload
// token. It does not fetch or change an intake API key. The receiving service's
// authentication path verifies the token; the Agent must not treat decoded claims as
// authorization. Each call is one attempt, so callers can obtain a fresh proof
// and token when retrying enrollment rather than retrying an expired token.
func GetWorkloadToken(ctx context.Context, cfg pkgconfigmodel.Reader, provider common.WorkloadProvider, auth *common.AuthConfig, request common.WorkloadTokenRequest, targetSite string) (string, error) {
	tok, err := GetWorkloadTokenWithExpiry(ctx, cfg, provider, auth, request, targetSite)
	if err != nil {
		return "", err
	}
	return tok.Token, nil
}

// GetWorkloadTokenWithExpiry exchanges a fresh, request-bound proof for an opaque
// workload token and parses the backend-reported expiration from attributes.expires
// (ETS ExternalToken encodes expiry as a Unix seconds string). Missing or
// unparseable expiry values leave ExpiresAt zero so callers can apply a safe fallback.
func GetWorkloadTokenWithExpiry(ctx context.Context, cfg pkgconfigmodel.Reader, provider common.WorkloadProvider, auth *common.AuthConfig, request common.WorkloadTokenRequest, targetSite string) (common.WorkloadToken, error) {
	if err := request.Validate(); err != nil {
		return common.WorkloadToken{}, err
	}
	if provider == nil {
		return common.WorkloadToken{}, errors.New("workload proof provider is required")
	}
	endpoint, err := resolveExchangeURL(cfg, targetSite, "/api/v2/delegated-token?token_type=workload")
	if err != nil {
		return common.WorkloadToken{}, err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return common.WorkloadToken{}, errors.New("workload token endpoint must use HTTPS")
	}
	proof, err := provider.GenerateWorkloadAuthProof(ctx, cfg, auth, request)
	if err != nil {
		return common.WorkloadToken{}, fmt.Errorf("generate workload proof: %w", err)
	}
	body := struct {
		Data struct {
			Type       string                      `json:"type"`
			Attributes common.WorkloadTokenRequest `json:"attributes"`
		} `json:"data"`
	}{}
	body.Data.Type = "delegated_token_request"
	body.Data.Attributes = request
	payload, err := json.Marshal(body)
	if err != nil {
		return common.WorkloadToken{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return common.WorkloadToken{}, err
	}
	req.Header.Set(contentTypeHeader, "application/vnd.api+json")
	req.Header.Set(authorizationHeader, authorizationType+" "+proof)
	transport := httputils.CreateHTTPTransport(cfg)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: httpClientTimeout,
		// Never forward a cloud proof to a redirect target or change this POST to GET.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return common.WorkloadToken{}, fmt.Errorf("request workload token: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize+1))
	if err != nil {
		return common.WorkloadToken{}, fmt.Errorf("read workload token response: %w", err)
	}
	if len(raw) > maxResponseBodySize {
		return common.WorkloadToken{}, errors.New("workload token response exceeds size limit")
	}
	if resp.StatusCode != http.StatusOK {
		// Surface only the server's JSON:API error message, never the raw response body.
		return common.WorkloadToken{}, &common.WorkloadTokenHTTPError{
			StatusCode: resp.StatusCode,
			Detail:     parseErrorDetail(raw),
		}
	}
	var response struct {
		Data struct {
			Type       string `json:"type"`
			Attributes struct {
				AccessToken string          `json:"access_token"`
				Expires     json.RawMessage `json:"expires"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Data.Type != "token" || response.Data.Attributes.AccessToken == "" {
		return common.WorkloadToken{}, errors.New("invalid workload token response")
	}
	return common.WorkloadToken{
		Token:     response.Data.Attributes.AccessToken,
		ExpiresAt: parseWorkloadTokenExpiry(response.Data.Attributes.Expires),
	}, nil
}

// parseWorkloadTokenExpiry parses ETS's attributes.expires field (Unix seconds
// string in ExternalToken, or JSON integer / RFC3339 fallback). Returns zero
// time when omitted or unparseable so valid tokens are never rejected solely
// for expiry formatting.
func parseWorkloadTokenExpiry(raw json.RawMessage) time.Time {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return time.Time{}
	}
	var s string
	if err := json.Unmarshal(trimmed, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			return time.Time{}
		}
		if unix, err := strconv.ParseInt(s, 10, 64); err == nil {
			if unix > 0 {
				return time.Unix(unix, 0).UTC()
			}
			return time.Time{}
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil && !t.IsZero() {
			return t.UTC()
		}
		return time.Time{}
	}
	var unix int64
	if err := json.Unmarshal(trimmed, &unix); err == nil && unix > 0 {
		return time.Unix(unix, 0).UTC()
	}
	return time.Time{}
}
