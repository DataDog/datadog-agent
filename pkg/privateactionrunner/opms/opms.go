// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package opms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/jsonapi"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	actionsclientpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/actionsclient"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	dequeuePath     = "/api/v2/on-prem-management-service/workflow-tasks/dequeue"
	taskUpdatePath  = "/api/v2/on-prem-management-service/workflow-tasks/publish-task-update"
	heartbeat       = "/api/v2/on-prem-management-service/workflow-tasks/heartbeat"
	healthCheckPath = "/api/v2/on-prem-management-service/runner/health-check"

	serverTimeHeader   = "X-Server-Time"
	retryAfterMsHeader = "X-Retry-After-Ms"

	// maxRetryAfter caps the X-Retry-After-Ms value the server can request, so
	// a misconfigured or malicious server cannot push the runner into long
	// idle stretches.
	maxRetryAfter = 2 * time.Minute
)

// ErrJobNotFound means the task no longer exists remotely; callers stop heartbeating.
var ErrJobNotFound = errors.New("job not found")

// HTTPError carries the HTTP status code of an unexpected response.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("request failed with status code %d and body %s", e.StatusCode, e.Body)
}

type DequeueJSONRequest struct {
	ID                 string `jsonapi:"primary,dequeue"`
	RunnerStartedAt    string `json:"runner_started_at,omitempty" jsonapi:"attribute"`
	LastTaskReceivedAt string `json:"last_task_received_at,omitempty" jsonapi:"attribute"`
}

type PublishTaskUpdateJSONRequestPayload struct {
	Branch       string                            `json:"branch,omitempty"`
	Outputs      interface{}                       `json:"outputs,omitempty"`
	ErrorCode    aperrorpb.ActionPlatformErrorCode `json:"error_code,omitempty"`
	ErrorDetails string                            `json:"error_details,omitempty"`
	APIError     string                            `json:"api_error,omitempty"`
}

type PublishTaskUpdateJSONRequestAttributes struct {
	TaskID    string                               `json:"task_id,omitempty"`
	Client    actionsclientpb.Client               `json:"client,omitempty"`
	ActionFQN string                               `json:"action_fqn,omitempty"`
	JobId     string                               `json:"job_id,omitempty"`
	Payload   *PublishTaskUpdateJSONRequestPayload `json:"payload,omitempty"`
}

type PublishTaskUpdateJSONData struct {
	Type       string                                  `json:"type,omitempty"`
	ID         string                                  `json:"id,omitempty"`
	Attributes *PublishTaskUpdateJSONRequestAttributes `json:"attributes,omitempty"`
}

type PublishTaskUpdateJSONRequest struct {
	Data *PublishTaskUpdateJSONData `json:"data,omitempty"`
}

type HeartbeatJSONRequestAttributes struct {
	TaskID    string                 `json:"task_id,omitempty"`
	Client    actionsclientpb.Client `json:"client,omitempty"`
	ActionFQN string                 `json:"action_fqn,omitempty"`
	JobId     string                 `json:"job_id,omitempty"`
}

type HeartbeatJSONData struct {
	Type       string                          `json:"type,omitempty"`
	ID         string                          `json:"id,omitempty"`
	Attributes *HeartbeatJSONRequestAttributes `json:"attributes,omitempty"`
}

type HeartbeatJSONRequest struct {
	Data *HeartbeatJSONData `json:"data,omitempty"`
}

type HealthCheckData struct {
	ServerTime *time.Time    `json:"server_time,omitempty"`
	RetryAfter time.Duration `json:"retry_after_ms,omitempty"`
}

// Client is the OPMS client interface
// Enrollment is intentionally omitted from this OPMS interface as the client requires a config.
// Ensure enrollment is completed before instantiating this client.
type Client interface {
	// DequeueTask fetches the next pending task. The returned duration is the
	// server-requested retry delay from the X-Retry-After-Ms response header; a
	// zero value means no hint was given and the caller should use its default
	// interval.
	DequeueTask(ctx context.Context) (*types.Task, time.Duration, error)
	PublishSuccess(
		ctx context.Context,
		client actionsclientpb.Client,
		taskID string,
		jobID string,
		actionFQN string,
		output interface{},
		branch string,
	) error
	PublishFailure(
		ctx context.Context,
		client actionsclientpb.Client,
		taskID string,
		jobID string,
		actionFQN string,
		errorCode aperrorpb.ActionPlatformErrorCode,
		errorDetails string,
		apiError string,
	) error
	HealthCheck(ctx context.Context) (*HealthCheckData, error)
	Heartbeat(ctx context.Context, client actionsclientpb.Client, taskID, actionFQN, jobID string) error
}

type client struct {
	config     parconfig.Config
	identity   *parconfig.Identity
	httpClient *http.Client

	runnerStartedAt    time.Time
	lastTaskReceivedAt atomic.Pointer[time.Time]
}

func NewClient(coreCfg parconfig.RuntimeConfig, identity *parconfig.Identity) Client {
	return &client{
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: coreCfg.HTTPTransport(),
		},
		config:          coreCfg,
		identity:        identity,
		runnerStartedAt: time.Now().UTC(),
	}
}

func EndpointURL(cfg parconfig.Config, path string) (string, error) {
	endpoint, err := cfg.APIBaseURL()
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(endpoint, "/") + path, nil
}

func (c *client) DequeueTask(ctx context.Context) (*types.Task, time.Duration, error) {
	reqBody, err := c.buildDequeueRequestBody()
	if err != nil {
		return nil, 0, fmt.Errorf("error building dequeue request body: %w", err)
	}

	body, headers, err := c.makeRequest(ctx, http.MethodPost, dequeuePath, bytes.NewReader(reqBody), nil, http.StatusOK)
	retryAfter := parseRetryAfterMs(headers)
	if err != nil {
		return nil, retryAfter, fmt.Errorf("error making request to dequeue task: %w", err)
	}

	if len(body) == 0 {
		return nil, retryAfter, nil
	}

	res := &types.Task{
		Raw: body,
	}
	if err := json.Unmarshal(body, res); err != nil {
		return nil, retryAfter, fmt.Errorf("error unmarshaling dequeue task response: %w", err)
	}

	now := time.Now().UTC()
	c.lastTaskReceivedAt.Store(&now)

	return res, retryAfter, nil
}

func (c *client) buildDequeueRequestBody() ([]byte, error) {
	req := &DequeueJSONRequest{
		RunnerStartedAt: c.runnerStartedAt.Format(time.RFC3339),
	}
	if t := c.lastTaskReceivedAt.Load(); t != nil {
		req.LastTaskReceivedAt = t.Format(time.RFC3339)
	}
	return jsonapi.Marshal(req, jsonapi.MarshalClientMode())
}

func (c *client) PublishSuccess(
	ctx context.Context,
	client actionsclientpb.Client,
	taskID string,
	jobID string,
	actionFQN string,
	output interface{},
	branch string,
) error {
	outputJson, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("error marshaling output: %w", err)
	}

	var asMap interface{}
	if err = json.Unmarshal(outputJson, &asMap); err != nil {
		return fmt.Errorf("error converting output to map: %w", err)
	}

	if branch == "" {
		branch = "main"
	}

	request := &PublishTaskUpdateJSONData{
		Type: "taskUpdate",
		ID:   "succeed_task",
		Attributes: &PublishTaskUpdateJSONRequestAttributes{
			TaskID:    taskID,
			Client:    client,
			ActionFQN: actionFQN,
			Payload: &PublishTaskUpdateJSONRequestPayload{
				Branch:  branch,
				Outputs: asMap,
			},
			JobId: jobID,
		},
	}

	if _, err = c.makeTaskUpdateRequest(ctx, http.MethodPost, taskUpdatePath, request); err != nil {
		return fmt.Errorf("error updating success task status: %w", err)
	}

	return nil
}

func (c *client) PublishFailure(
	ctx context.Context,
	client actionsclientpb.Client,
	taskID string,
	jobID string,
	actionFQN string,
	errorCode aperrorpb.ActionPlatformErrorCode,
	errorDetails string,
	apiError string,
) error {
	request := &PublishTaskUpdateJSONData{
		Type: "taskUpdate",
		ID:   "fail_task",
		Attributes: &PublishTaskUpdateJSONRequestAttributes{
			TaskID:    taskID,
			Client:    client,
			ActionFQN: actionFQN,
			Payload: &PublishTaskUpdateJSONRequestPayload{
				ErrorCode:    errorCode,
				ErrorDetails: errorDetails,
			},
			JobId: jobID,
		},
	}

	if _, err := c.makeTaskUpdateRequest(ctx, http.MethodPost, taskUpdatePath, request); err != nil {
		return fmt.Errorf("error updating success task status: %w", err)
	}

	return nil
}

func (c *client) makeTaskUpdateRequest(
	ctx context.Context,
	method string,
	url string,
	data *PublishTaskUpdateJSONData,
) ([]byte, error) {
	request := &PublishTaskUpdateJSONRequest{
		Data: data,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error marshaling body for JSON request: %w", err)
	}

	resBody, _, err := c.makeRequest(
		ctx,
		method,
		url,
		bytes.NewReader(body),
		nil,
	)
	return resBody, err
}

// parseRetryAfterMs reads the X-Retry-After-Ms header and returns the
// corresponding duration. Returns 0 if the header is absent, zero-valued, or
// cannot be parsed — callers should treat 0 as "use default behaviour".
func parseRetryAfterMs(headers http.Header) time.Duration {
	if headers == nil {
		return 0
	}
	val := headers.Get(retryAfterMsHeader)
	if val == "" {
		return 0
	}
	ms, err := strconv.ParseInt(val, 10, 64)
	if err != nil || ms <= 0 {
		return 0
	}
	return min(time.Duration(ms)*time.Millisecond, maxRetryAfter)
}

func createHealthCheckData(headers http.Header) *HealthCheckData {
	response := &HealthCheckData{}

	if headers != nil {
		if serverTimeStr := headers.Get(serverTimeHeader); serverTimeStr != "" {
			if serverTime, err := time.Parse(time.RFC3339, serverTimeStr); err == nil {
				response.ServerTime = &serverTime
			}
		}
		response.RetryAfter = parseRetryAfterMs(headers)
	}

	return response
}

func (c *client) HealthCheck(ctx context.Context) (*HealthCheckData, error) {
	_, resHeaders, err := c.makeRequest(ctx, http.MethodGet, healthCheckPath, nil, nil, http.StatusOK)
	if err != nil {
		response := createHealthCheckData(resHeaders)
		return response, fmt.Errorf("error making request to health check endpoint: %w", err)
	}

	response := createHealthCheckData(resHeaders)
	return response, nil
}

func (c *client) Heartbeat(ctx context.Context, client actionsclientpb.Client, taskID, actionFQN, jobID string) error {
	request := &HeartbeatJSONData{
		Type: "heartbeat",
		ID:   taskID,
		Attributes: &HeartbeatJSONRequestAttributes{
			TaskID:    taskID,
			Client:    client,
			ActionFQN: actionFQN,
			JobId:     jobID,
		},
	}

	if _, err := c.makeHeartbeatRequest(ctx, http.MethodPost, heartbeat, request); err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return fmt.Errorf("%w: %v", ErrJobNotFound, err)
		}
		return fmt.Errorf("error sending heartbeat: %w", err)
	}

	return nil
}

func (c *client) makeHeartbeatRequest(
	ctx context.Context,
	method string,
	url string,
	data *HeartbeatJSONData,
) ([]byte, error) {
	request := &HeartbeatJSONRequest{
		Data: data,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error marshaling body for heartbeat request: %w", err)
	}

	resBody, _, err := c.makeRequest(
		ctx,
		method,
		url,
		bytes.NewReader(body),
		nil,
		http.StatusOK,
	)
	return resBody, err
}

func (c *client) makeRequest(
	ctx context.Context,
	method string,
	url string,
	body io.Reader,
	extraJwtClaims map[string]any,
	expectedStatusCodes ...int,
) ([]byte, http.Header, error) {
	endpoint, err := EndpointURL(c.config, url)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating HTTP request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	signedJWT, err := util.GeneratePARJWT(c.identity.OrgId, c.identity.RunnerId, c.identity.PrivateKey, extraJwtClaims)
	if err != nil {
		return nil, nil, fmt.Errorf("error signing JWT for request: %w", err)
	}
	req.Header.Set(app.JwtHeaderName, signedJWT)
	req.Header.Set(app.VersionHeaderName, version.AgentVersion)
	modesStr := modes.ToStrings([]modes.Mode{modes.ModePull})
	req.Header.Set(app.ModeHeaderName, strings.Join(modesStr, ","))
	req.Header.Set(app.PlatformHeaderName, runtime.GOOS)
	req.Header.Set(app.ArchitectureHeaderName, runtime.GOARCH)
	req.Header.Set(app.FlavorHeaderName, flavor.GetFlavor())
	req.Header.Set(app.ContainerizedHeaderName, strconv.FormatBool(env.IsContainerized()))
	for k, v := range c.config.OPMSExtraHeaders() {
		req.Header.Set(k, v)
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("error making HTTP request: %w", err)
	}
	defer func() {
		err = res.Body.Close()
		if err != nil {
			log.FromContext(ctx).Errorf("error closing request body: %v", err)
		}
	}()

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, res.Header, fmt.Errorf("error reading body of HTTP response: %w", err)
	}

	if len(expectedStatusCodes) != 0 && !slices.Contains(expectedStatusCodes, res.StatusCode) {
		return nil, res.Header, &HTTPError{StatusCode: res.StatusCode, Body: string(resBody)}
	}

	return resBody, res.Header, nil
}
