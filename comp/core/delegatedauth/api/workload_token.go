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

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// GetWorkloadToken exchanges a fresh, request-bound proof for an opaque workload
// token. It does not fetch or change an intake API key. The consumer (e.g. PAR)
// verifies the token and its grant; the Agent must not treat decoded claims as
// authorization. Each call is one attempt, so callers can obtain a fresh proof
// and token when retrying enrollment rather than retrying an expired token.
func GetWorkloadToken(ctx context.Context, cfg pkgconfigmodel.Reader, provider common.WorkloadProvider, auth *common.AuthConfig, request common.WorkloadTokenRequest, targetSite string) (string, error) {
	if err := request.Validate(); err != nil {
		return "", err
	}
	if provider == nil {
		return "", errors.New("workload proof provider is required")
	}
	endpoint, err := resolveExchangeURL(cfg, targetSite, "/api/v2/delegated-token?token_type=workload")
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("workload token endpoint must use HTTPS")
	}
	proof, err := provider.GenerateWorkloadAuthProof(ctx, cfg, auth, request)
	if err != nil {
		return "", fmt.Errorf("generate workload proof: %w", err)
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
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
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
		return "", fmt.Errorf("request workload token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Do not include response bodies or credentials in errors that may be logged.
		return "", fmt.Errorf("workload token request failed (HTTP %d)", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize+1))
	if err != nil {
		return "", fmt.Errorf("read workload token response: %w", err)
	}
	if len(raw) > maxResponseBodySize {
		return "", errors.New("workload token response exceeds size limit")
	}
	var response struct {
		Data struct {
			Type       string `json:"type"`
			Attributes struct {
				AccessToken string `json:"access_token"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Data.Type != "token" || response.Data.Attributes.AccessToken == "" {
		return "", errors.New("invalid workload token response")
	}
	return response.Data.Attributes.AccessToken, nil
}
