// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

const workloadAuthorizationPath = "/api/unstable/workload-authorization"

// GetWorkloadAuthorization exchanges a fresh provider proof for a PAR enrollment
// assertion bound to the runner key. It never writes API keys or configuration.
func GetWorkloadAuthorization(ctx context.Context, cfg pkgconfigmodel.Reader, proof, targetSite, orgUUID, jwkThumbprint string) (*common.WorkloadAuthorization, error) {
	thumbprint, err := base64.RawURLEncoding.DecodeString(jwkThumbprint)
	if err != nil || len(thumbprint) != 32 || base64.RawURLEncoding.EncodeToString(thumbprint) != jwkThumbprint {
		return nil, errors.New("invalid runner JWK thumbprint")
	}
	if proof == "" || orgUUID == "" {
		return nil, errors.New("workload proof and organization are required")
	}
	body, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"type":       "workload_authorization_request",
			"attributes": map[string]string{"purpose": common.PAREnrollmentPurpose, "jwk_thumbprint": jwkThumbprint},
		},
	})
	if err != nil {
		return nil, errors.New("failed to encode workload authorization request")
	}
	url := strings.TrimSuffix(resolveTokenURL(cfg, targetSite), "/api/v2/intake-key") + workloadAuthorizationPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("failed to create workload authorization request")
	}
	req.Header.Set(contentTypeHeader, "application/vnd.api+json")
	req.Header.Set("Accept", "application/vnd.api+json")
	req.Header.Set(authorizationHeader, authorizationType+" "+proof)
	transport := httputils.CreateHTTPTransport(cfg)
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport, Timeout: httpClientTimeout,
		// Never forward a provider proof to a redirect destination.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("workload authorization request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Do not include the response body; an error may echo a proof or assertion.
		return nil, fmt.Errorf("workload authorization failed with HTTP status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize+1))
	if err != nil || len(raw) > maxResponseBodySize {
		return nil, errors.New("failed to read workload authorization response")
	}
	return parseWorkloadAuthorization(raw, orgUUID, time.Now())
}

func parseWorkloadAuthorization(raw []byte, orgUUID string, now time.Time) (*common.WorkloadAuthorization, error) {
	var response struct {
		Data struct {
			Type       string `json:"type"`
			Attributes struct {
				AccessToken string `json:"access_token"`
				common.WorkloadAuthorization
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, errors.New("invalid workload authorization response")
	}
	a := response.Data.Attributes.WorkloadAuthorization
	a.Token = response.Data.Attributes.AccessToken
	if response.Data.Type != "workload_authorization" || a.Token == "" || a.OrgID == 0 ||
		!strings.EqualFold(a.OrgUUID, orgUUID) || a.Provider != "aws" || a.IntakeMappingID == "" ||
		a.StablePrincipal == "" || !a.ExpiresAt.After(now) || a.ExpiresAt.After(now.Add(5*time.Minute+30*time.Second)) {
		return nil, errors.New("invalid or expired workload authorization response")
	}
	return &a, nil
}
