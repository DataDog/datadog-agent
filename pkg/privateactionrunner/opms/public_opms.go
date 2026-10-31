// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

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
	"github.com/go-jose/go-jose/v4"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/parversion"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/par"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// ErrEnrollmentRejected is returned when enrollment fails with a client error
// that retrying cannot fix. The request is the same on every attempt, so the
// runner must stop instead of re-enrolling until the configuration is fixed and
// it is restarted.
var ErrEnrollmentRejected = errors.New("enrollment rejected")

// ErrEnrollmentUnauthorized is the ErrEnrollmentRejected returned for rejected credentials.
var ErrEnrollmentUnauthorized = fmt.Errorf("%w: credentials not accepted", ErrEnrollmentRejected)

const (
	createPARPath           = "/api/unstable/on_prem_runners"
	createPARApiKeyOnlyPath = "/api/unstable/on_prem_runners/api_key_only"
	createPARWorkloadPath   = "/api/unstable/on_prem_runners/workload_token"

	enrollmentInitialBackoff = 1 * time.Second
	enrollmentMaxBackoff     = 30 * time.Second
)

// PublicClient exposes enrollment endpoints authenticated with API keys or workload tokens.
type PublicClient interface {
	MigrateWithWorkloadToken(ctx context.Context, token, runnerID, proof string) (*par.CreateRunnerResponse, error)
	EnrollWithWorkloadToken(ctx context.Context, token string, request par.CreateRunnerRequest) (*par.CreateRunnerResponse, error)

	EnrollWithApiKey(
		ctx context.Context,
		apiKey string,
		appKey string,
		runnerName string,
		runnerModes []modes.Mode,
		publicJwk *jose.JSONWebKey,
		agentHostname string,
		orchClusterID string,
		agentFlavor string,
	) (*par.CreateRunnerResponse, error)

	EnrollWithApiKeyOnly(
		ctx context.Context,
		apiKey string,
		runnerName string,
		runnerModes []modes.Mode,
		publicJwk *jose.JSONWebKey,
		agentHostname string,
		orchClusterID string,
		agentFlavor string,
	) (*par.CreateRunnerResponse, error)
}

type publicClient struct {
	ddBaseURL    string
	httpClient   *http.Client
	extraHeaders map[string]string
}

func NewPublicClient(cfg model.Reader, ddBaseURL string, extraHeaders map[string]string) PublicClient {
	return &publicClient{
		ddBaseURL: strings.TrimSuffix(ddBaseURL, "/"),
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: httputils.CreateHTTPTransport(cfg),
		},
		extraHeaders: extraHeaders,
	}
}

func (p *publicClient) EnrollWithApiKey(
	ctx context.Context,
	apiKey string,
	appKey string,
	runnerName string,
	runnerModes []modes.Mode,
	publicJwk *jose.JSONWebKey,
	agentHostname string,
	orchClusterID string,
	agentFlavor string,
) (*par.CreateRunnerResponse, error) {
	return p.enroll(ctx, createPARPath, apiKey, appKey, runnerName, runnerModes, publicJwk, agentHostname, orchClusterID, agentFlavor)
}

func (p *publicClient) EnrollWithApiKeyOnly(
	ctx context.Context,
	apiKey string,
	runnerName string,
	runnerModes []modes.Mode,
	publicJwk *jose.JSONWebKey,
	agentHostname string,
	orchClusterID string,
	agentFlavor string,
) (*par.CreateRunnerResponse, error) {
	return p.enroll(ctx, createPARApiKeyOnlyPath, apiKey, "", runnerName, runnerModes, publicJwk, agentHostname, orchClusterID, agentFlavor)
}

func (p *publicClient) enroll(
	ctx context.Context,
	path string,
	apiKey string,
	appKey string,
	runnerName string,
	runnerModes []modes.Mode,
	publicJwk *jose.JSONWebKey,
	agentHostname string,
	orchClusterID string,
	agentFlavor string,
) (*par.CreateRunnerResponse, error) {
	publicKeyPEM, err := util.JWKToPEM(publicJwk)
	if err != nil {
		return nil, fmt.Errorf("failed to convert public key to PEM: %w", err)
	}

	createRunnerURL := p.ddBaseURL + path

	request := par.CreateRunnerRequest{
		RunnerName:    runnerName,
		RunnerModes:   runnerModes,
		PublicKeyPEM:  publicKeyPEM,
		AgentHostname: agentHostname,
		OrchClusterID: orchClusterID,
		AgentFlavor:   agentFlavor,
	}

	requestBodyJSON, err := jsonapi.Marshal(request, jsonapi.MarshalClientMode())
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	respBody, err := p.doEnrollRequestWithRetry(ctx, createRunnerURL, requestBodyJSON, apiKey, appKey)
	if err != nil {
		return nil, err
	}

	createRunnerResponse := new(par.CreateRunnerResponse)
	err = jsonapi.Unmarshal(respBody, createRunnerResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal runner creation response: %w", err)
	}
	return createRunnerResponse, nil
}

// doEnrollRequestWithRetry sends the enrollment POST and retries transient
// failures with exponential backoff. Other failures are returned immediately as
// ErrEnrollmentRejected. Retries are unbounded; the caller's context
// cancellation is the only exit other than success or a rejection. Enrollment
// is required for the runner to function, so we keep trying rather than
// crashing the agent.
func (p *publicClient) doEnrollRequestWithRetry(ctx context.Context, url string, body []byte, apiKey, appKey string) ([]byte, error) {
	return util.RetryHTTPRequest(ctx, func() ([]byte, int, error) {
		return p.doEnrollRequest(ctx, url, body, apiKey, appKey)
	}, util.RetryHTTPOptions{
		InitialInterval: enrollmentInitialBackoff,
		MaxInterval:     enrollmentMaxBackoff,
		// MaxElapsedTime: 0 — retry forever until ctx is cancelled.
	})
}

// doEnrollRequest sends a single enrollment POST. Returns the response body on
// success. On non-2xx responses, returns the status code so the caller can
// decide whether to retry.
func (p *publicClient) doEnrollRequest(ctx context.Context, url string, body []byte, apiKey, appKey string) ([]byte, int, error) {
	req, err := newEnrollmentRequest(ctx, url, body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build runner creation request: %w", err)
	}

	req.Header.Set("DD-API-KEY", apiKey)
	if appKey != "" {
		req.Header.Set("DD-APPLICATION-KEY", appKey)
	}
	for k, v := range p.extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to send runner creation request: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Error("error closing runner creation response body", log.ErrorField(cerr))
		}
	}()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, resp.StatusCode, fmt.Errorf("%w (HTTP %d): check the API/application key and its required scopes, then restart the Private Action Runner", ErrEnrollmentUnauthorized, resp.StatusCode)
	}

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr == nil && resp.StatusCode == http.StatusOK {
		return respBody, resp.StatusCode, nil
	}

	if readErr != nil {
		err = fmt.Errorf("runner creation failed with HTTP status code %d and failed to read HTTP response with error %w", resp.StatusCode, readErr)
	} else {
		err = fmt.Errorf("runner creation failed with HTTP status code %d and response %s", resp.StatusCode, string(respBody))
	}
	// Classify on the status alone, so a rejection whose body cannot be read
	// still stops the runner.
	if !util.IsRetryableHTTPStatus(resp.StatusCode) {
		err = fmt.Errorf("%w: %w; check the site and Private Action Runner configuration, then restart the Private Action Runner", ErrEnrollmentRejected, err)
	}
	return nil, resp.StatusCode, err
}

// EnrollWithWorkloadToken makes one enrollment attempt using an ETS workload
// token bound to request.PublicKeyPEM. It sends no API/application key and never
// falls back to API-key enrollment. The caller owns identity persistence and
// retries: a short-lived token must be refreshed before a later attempt, and an
// ambiguous response must not cause a new key/runner to be created.
// Use MigrateWithWorkloadToken to move an existing runner to WIF.
func (p *publicClient) EnrollWithWorkloadToken(ctx context.Context, token string, request par.CreateRunnerRequest) (*par.CreateRunnerResponse, error) {
	if request.ID != "" {
		return nil, errors.New("workload enrollment cannot specify an existing runner ID")
	}
	payload, err := jsonapi.Marshal(request, jsonapi.MarshalClientMode())
	if err != nil {
		return nil, fmt.Errorf("marshal workload enrollment request: %w", err)
	}
	return p.sendWorkloadEnrollment(ctx, token, "", "", payload)
}

// MigrateWithWorkloadToken changes the authorization of an existing runner.
// The proof must be signed by that runner's key and bound to this workload token.
func (p *publicClient) MigrateWithWorkloadToken(ctx context.Context, token, runnerID, proof string) (*par.CreateRunnerResponse, error) {
	if runnerID == "" || proof == "" {
		return nil, errors.New("workload migration requires a runner ID and ownership proof")
	}
	return p.sendWorkloadEnrollment(ctx, token, runnerID, proof, nil)
}

func (p *publicClient) sendWorkloadEnrollment(ctx context.Context, token, runnerID, proof string, payload []byte) (*par.CreateRunnerResponse, error) {
	if token == "" {
		return nil, errors.New("workload enrollment token is required")
	}
	endpoint := p.ddBaseURL + createPARWorkloadPath
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("workload enrollment endpoint must use HTTPS")
	}
	if runnerID != "" {
		parsed.RawQuery = url.Values{"existing_runner_id": {runnerID}}.Encode()
	}
	req, err := newEnrollmentRequest(ctx, parsed.String(), payload)
	if err != nil {
		return nil, err
	}
	for k, v := range p.extraHeaders {
		req.Header.Set(k, v)
	}
	// Extra headers cannot override workload authentication or add API keys.
	req.Header.Del("DD-API-KEY")
	req.Header.Del("DD-APPLICATION-KEY")
	req.Header.Set("Authorization", "Bearer "+token)
	if proof != "" {
		req.Header.Set("X-Datadog-PAR-Proof", proof)
	}
	// Share the configured transport and timeout without changing the legacy
	// client's redirect policy. A workload token must not follow redirects.
	client := *p.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
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

// newEnrollmentRequest shares the JSON:API POST setup. Each enrollment path
// supplies its own credentials and controls its retry and redirect policy.
func newEnrollmentRequest(ctx context.Context, url string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/vnd.api+json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(app.VersionHeaderName, parversion.RunnerVersion)
	return req, nil
}
