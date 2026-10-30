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
	"net/url"
	"strings"
	"time"

	"github.com/DataDog/jsonapi"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/parversion"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/par"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// EnrollWithWorkloadToken makes one enrollment attempt using an ETS workload
// token bound to request.PublicKeyPEM. It sends no API/application key and never
// falls back to API-key enrollment. The caller owns identity persistence and
// retries: a short-lived token must be refreshed before a later attempt, and an
// ambiguous response must not cause a new key/runner to be created.
// This endpoint creates new runners; it does not migrate an existing runner.
func EnrollWithWorkloadToken(ctx context.Context, cfg model.Reader, ddBaseURL, token string, request par.CreateRunnerRequest) (*par.CreateRunnerResponse, error) {
	if token == "" {
		return nil, errors.New("workload enrollment token is required")
	}
	if request.ID != "" {
		return nil, errors.New("workload enrollment cannot specify an existing runner ID")
	}
	endpoint := strings.TrimSuffix(ddBaseURL, "/") + "/api/unstable/on_prem_runners/workload_token"
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("workload enrollment endpoint must use HTTPS")
	}
	payload, err := jsonapi.Marshal(request, jsonapi.MarshalClientMode())
	if err != nil {
		return nil, fmt.Errorf("marshal workload enrollment request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/vnd.api+json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(app.VersionHeaderName, parversion.RunnerVersion)
	transport := httputils.CreateHTTPTransport(cfg)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send workload enrollment request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Do not echo credentials or the response body into Agent logs.
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("%w (HTTP %d): check the workload token and the WIF mapping's PAR enrollment permission", ErrEnrollmentUnauthorized, resp.StatusCode)
		}
		err := fmt.Errorf("workload enrollment failed (HTTP %d)", resp.StatusCode)
		if resp.StatusCode < http.StatusBadRequest || !util.IsRetryableHTTPStatus(resp.StatusCode) {
			err = fmt.Errorf("%w: %w", ErrEnrollmentRejected, err)
		}
		return nil, err
	}
	const maxResponseSize = 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("read workload enrollment response: %w", err)
	}
	if len(body) > maxResponseSize {
		return nil, errors.New("workload enrollment response exceeds size limit")
	}
	response := new(par.CreateRunnerResponse)
	if jsonapi.Unmarshal(body, response) != nil || response.RunnerID == "" || response.OrgID <= 0 {
		return nil, errors.New("invalid workload enrollment response")
	}
	return response, nil
}
