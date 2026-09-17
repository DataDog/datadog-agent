// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package opms

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/par"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/jsonapi"
)

const workloadEnrollmentPath = "/api/unstable/on_prem_runners/workload_identity"

// ExchangeWorkloadIdentity performs one enrollment attempt. The caller refreshes
// the ETS assertion before retrying, preserving the same durable runner key.
func ExchangeWorkloadIdentity(ctx context.Context, cfg model.Reader, baseURL, assertion, runnerID, proof string, expectedVersion int64, request *par.CreateRunnerRequest, extraHeaders map[string]string) (*par.CreateRunnerResponse, error) {
	path := workloadEnrollmentPath
	var body []byte
	var err error
	if runnerID == "" {
		body, err = jsonapi.Marshal(request, jsonapi.MarshalClientMode())
	} else {
		if strings.ContainsAny(runnerID, "/?#") {
			return nil, errors.New("invalid runner ID")
		}
		path = "/api/unstable/on_prem_runners/" + runnerID + "/reauthorize"
		body, err = jsonapi.Marshal(struct {
			ExpectedAuthorizationVersion int64  `json:"expected_authorization_version" jsonapi:"attribute"`
			ID                           string `jsonapi:"primary,reauthorizeRunnerRequest"`
			PublicKeyPEM                 string `json:"public_key_pem" jsonapi:"attribute"`
		}{ExpectedAuthorizationVersion: expectedVersion, PublicKeyPEM: request.PublicKeyPEM}, jsonapi.MarshalClientMode())
	}
	if err != nil {
		return nil, errors.New("failed to encode workload enrollment request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("invalid workload enrollment endpoint")
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	req.Header.Del("DD-API-KEY")
	req.Header.Del("DD-APPLICATION-KEY")
	req.Header.Set("Content-Type", "application/vnd.api+json")
	req.Header.Set("Accept", "application/vnd.api+json")
	req.Header.Set("Authorization", "Bearer "+assertion)
	if proof != "" {
		req.Header.Set("X-Datadog-PAR-Proof", proof)
	}
	transport := httputils.CreateHTTPTransport(cfg)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("workload enrollment request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("workload enrollment failed with HTTP status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, errors.New("invalid workload enrollment response")
	}
	response := new(par.CreateRunnerResponse)
	if jsonapi.Unmarshal(raw, response) != nil || response.RunnerID == "" || response.OrgID <= 0 || response.AuthorizationVersion <= 0 {
		return nil, errors.New("invalid workload enrollment response")
	}
	if runnerID != "" && response.RunnerID != runnerID {
		return nil, errors.New("workload reauthorization changed runner identity")
	}
	return response, nil
}
