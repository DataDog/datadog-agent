// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package opms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/env"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	actionsclientpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/actionsclient"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

const (
	dequeuePath     = "/api/v2/on-prem-management-service/workflow-tasks/dequeue"
	taskUpdatePath  = "/api/v2/on-prem-management-service/workflow-tasks/publish-task-update"
	heartbeat       = "/api/v2/on-prem-management-service/workflow-tasks/heartbeat"
	healthCheckPath = "/api/v2/on-prem-management-service/runner/health-check"

	serverTimeHeader = "X-Server-Time"
)

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
	ServerTime *time.Time `json:"server_time,omitempty"`
}

// Client is the OPMS client interface
// Enrollment is intentionally omitted from this OPMS interface as the client requires a config.
// Ensure enrollment is completed before instantiating this client.
type Client interface {
	DequeueTask(ctx context.Context) (*types.Task, error)
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
	config     *config.Config
	httpClient *http.Client
}

func NewClient(cfg *config.Config) Client {
	return &client{
		httpClient: &http.Client{
			Timeout: time.Millisecond * time.Duration(cfg.OpmsRequestTimeout),
		},
		config: cfg,
	}
}

func (c *client) DequeueTask(ctx context.Context) (*types.Task, error) {
	u := &url.URL{
		Scheme: "https",
		Host:   c.config.DDApiHost,
		Path:   dequeuePath,
	}

	body, _, err := c.makeRequest(ctx, http.MethodPost, u.String(), nil, nil, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("error making request to dequeue task: %w", err)
	}

	if len(body) == 0 {
		return nil, nil
	}

	res := &types.Task{
		Raw: body,
	}
	if err := json.Unmarshal(body, res); err != nil {
		return nil, fmt.Errorf("error unmarshaling dequeue task response: %w", err)
	}

	return res, nil
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

	u := &url.URL{
		Scheme: "https",
		Host:   c.config.DDApiHost,
		Path:   taskUpdatePath,
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

	if _, err = c.makeTaskUpdateRequest(ctx, http.MethodPost, u.String(), request); err != nil {
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
	u := &url.URL{
		Scheme: "https",
		Host:   c.config.DDApiHost,
		Path:   taskUpdatePath,
	}

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

	if _, err := c.makeTaskUpdateRequest(ctx, http.MethodPost, u.String(), request); err != nil {
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

func createHealthCheckData(headers http.Header) *HealthCheckData {
	response := &HealthCheckData{}

	if headers != nil {
		if serverTimeStr := headers.Get(serverTimeHeader); serverTimeStr != "" {
			if serverTime, err := time.Parse(time.RFC3339, serverTimeStr); err == nil {
				response.ServerTime = &serverTime
			}
		}
	}

	return response
}

func (c *client) HealthCheck(ctx context.Context) (*HealthCheckData, error) {
	u := &url.URL{
		Scheme: "https",
		Host:   c.config.DDApiHost,
		Path:   healthCheckPath,
	}

	query := u.Query()
	query.Add(app.RunnerVersionQueryParam, c.config.Version)
	modesStr := modes.ToStrings(c.config.Modes)
	query.Add(app.ModesQueryParam, strings.Join(modesStr, ","))
	query.Add(app.PlatformQueryParam, runtime.GOOS)
	query.Add(app.ArchitectureQueryParam, runtime.GOARCH)
	query.Add(app.FlavorQueryParam, flavor.GetFlavor())
	query.Add(app.ContainerizedQueryParam, strconv.FormatBool(env.IsContainerized()))
	u.RawQuery = query.Encode()

	_, resHeaders, err := c.makeRequest(ctx, http.MethodGet, u.String(), nil, nil, http.StatusOK)
	if err != nil {
		response := createHealthCheckData(resHeaders)
		return response, fmt.Errorf("error making request to health check endpoint: %w", err)
	}

	response := createHealthCheckData(resHeaders)
	return response, nil
}

func (c *client) Heartbeat(ctx context.Context, client actionsclientpb.Client, taskID, actionFQN, jobID string) error {
	u := &url.URL{
		Scheme: "https",
		Host:   c.config.DDApiHost,
		Path:   heartbeat,
	}

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

	if _, err := c.makeHeartbeatRequest(ctx, http.MethodPost, u.String(), request); err != nil {
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
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating HTTP request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	signedJWT, err := util.GeneratePARJWT(c.config.OrgId, c.config.RunnerId, c.config.PrivateKey, extraJwtClaims)
	if err != nil {
		return nil, nil, fmt.Errorf("error signing JWT for request: %w", err)
	}
	req.Header.Set(app.JwtHeaderName, signedJWT)
	req.Header.Set(app.VersionHeaderName, c.config.Version)
	modesStr := modes.ToStrings(c.config.Modes)
	req.Header.Set(app.ModeHeaderName, strings.Join(modesStr, ","))
	req.Header.Set(app.PlatformHeaderName, runtime.GOOS)
	req.Header.Set(app.ArchitectureHeaderName, runtime.GOARCH)
	req.Header.Set(app.FlavorHeaderName, flavor.GetFlavor())
	req.Header.Set(app.ContainerizedHeaderName, strconv.FormatBool(env.IsContainerized()))
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
		return nil, res.Header, fmt.Errorf("request failed with status code %d and body %s", res.StatusCode, resBody)
	}

	return resBody, res.Header, nil
}
