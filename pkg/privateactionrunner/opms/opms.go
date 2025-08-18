package opms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/utils"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	dequeuePath     = "/api/v2/on-prem-management-service/workflow-tasks/dequeue"
	taskUpdatePath  = "/api/v2/on-prem-management-service/workflow-tasks/publish-task-update"
	heartbeat       = "/api/v2/on-prem-management-service/workflow-tasks/heartbeat"
	healthCheckPath = "/api/v2/on-prem-management-service/runner/health-check"
	enrollmentPath  = "/api/v2/on-prem-management-service/enrollments/complete"
)

type PublishTaskUpdateJSONRequestPayload struct {
	Branch       string                            `json:"branch,omitempty"`
	Outputs      interface{}                       `json:"outputs,omitempty"`
	ErrorCode    errorcode.ActionPlatformErrorCode `json:"error_code,omitempty"`
	ErrorDetails string                            `json:"error_details,omitempty"`
	APIError     string                            `json:"api_error,omitempty"`
}

type PublishTaskUpdateJSONRequestAttributes struct {
	TaskID    string                               `json:"task_id,omitempty"`
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
	TaskID    string `json:"task_id,omitempty"`
	ActionFQN string `json:"action_fqn,omitempty"`
	JobId     string `json:"job_id,omitempty"`
}

type HeartbeatJSONData struct {
	Type       string                          `json:"type,omitempty"`
	ID         string                          `json:"id,omitempty"`
	Attributes *HeartbeatJSONRequestAttributes `json:"attributes,omitempty"`
}

type HeartbeatJSONRequest struct {
	Data *HeartbeatJSONData `json:"data,omitempty"`
}

// EnrollmentRequest represents the enrollment request payload
type EnrollmentRequest struct {
	AccountBinding string `json:"accountBinding"`
	PublicKey      string `json:"publicKey"`
}

// EnrollmentResponseData represents the data section of the JSONAPI response
type EnrollmentResponseData struct {
	Type       string                    `json:"type"`
	ID         string                    `json:"id"`
	Attributes EnrollmentResponseAttribs `json:"attributes"`
}

// EnrollmentResponseAttribs represents the attributes section of the JSONAPI response
type EnrollmentResponseAttribs struct {
	RunnerId         string   `json:"runner_id"`
	OrgId            int64    `json:"org_id"`
	RunnerModes      []string `json:"runner_modes"`
	ActionsAllowlist []string `json:"actions_allowlist"`
}

// EnrollmentJSONAPIResponse represents the full JSONAPI response
type EnrollmentJSONAPIResponse struct {
	Data EnrollmentResponseData `json:"data"`
}

// EnrollmentResponse represents the simplified enrollment response
type EnrollmentResponse struct {
	ID               string
	RunnerId         string
	OrgId            int64
	Modes            []string
	ActionsAllowlist []string
}

// Client is the OPMS client interface
// Enrollment is intentionally omitted from this OPMS interface as the client requires a config.
// Ensure enrollment is completed before instantiating this client.
type Client interface {
	DequeueTask(ctx context.Context) (*types.Task, error)
	PublishSuccess(
		ctx context.Context,
		taskID string,
		jobID string,
		actionFQN string,
		output interface{},
		branch string,
	) error
	PublishFailure(
		ctx context.Context,
		taskID string,
		jobID string,
		actionFQN string,
		errorCode errorcode.ActionPlatformErrorCode,
		errorDetails string,
		apiError string,
	) error
	HealthCheck(ctx context.Context) error
	Heartbeat(ctx context.Context, taskID, actionFQN, jobID string) error
}

type client struct {
	config *config.Config

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
		Host:   c.config.DDHost,
		Path:   dequeuePath,
	}

	body, err := c.makeRequest(ctx, http.MethodPost, u.String(), nil, nil, http.StatusOK)
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
		Host:   c.config.DDHost,
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
	taskID string,
	jobID string,
	actionFQN string,
	errorCode errorcode.ActionPlatformErrorCode,
	errorDetails string,
	apiError string,
) error {
	u := &url.URL{
		Scheme: "https",
		Host:   c.config.DDHost,
		Path:   taskUpdatePath,
	}

	request := &PublishTaskUpdateJSONData{
		Type: "taskUpdate",
		ID:   "fail_task",
		Attributes: &PublishTaskUpdateJSONRequestAttributes{
			TaskID:    taskID,
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

	return c.makeRequest(
		ctx,
		method,
		url,
		bytes.NewReader(body),
		nil,
	)
}

func (c *client) HealthCheck(ctx context.Context) error {
	u := &url.URL{
		Scheme: "https",
		Host:   c.config.DDHost,
		Path:   healthCheckPath,
	}

	query := u.Query()
	query.Add("runnerVersion", c.config.Version)
	query.Add("modes", strings.Join(c.config.Modes, ","))
	u.RawQuery = query.Encode()

	if _, err := c.makeRequest(ctx, http.MethodGet, u.String(), nil, nil, http.StatusOK); err != nil {
		return fmt.Errorf("error making request to health check endpoint: %w", err)
	}

	return nil
}

func (c *client) Heartbeat(ctx context.Context, taskID, actionFQN, jobID string) error {
	u := &url.URL{
		Scheme: "https",
		Host:   c.config.DDHost,
		Path:   heartbeat,
	}

	request := &HeartbeatJSONData{
		Type: "heartbeat",
		ID:   taskID,
		Attributes: &HeartbeatJSONRequestAttributes{
			TaskID:    taskID,
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

	return c.makeRequest(
		ctx,
		method,
		url,
		bytes.NewReader(body),
		nil,
		http.StatusOK,
	)
}

func (c *client) makeRequest(
	ctx context.Context,
	method string,
	url string,
	body io.Reader,
	extraJwtClaims map[string]any,
	expectedStatusCodes ...int,
) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	signedJWT, err := utils.GeneratePARJWT(c.config.OrgId, c.config.RunnerId, c.config.PrivateKey, extraJwtClaims)
	if err != nil {
		return nil, fmt.Errorf("error signing JWT for request: %w", err)
	}
	req.Header.Set(utils.JwtHeaderName, signedJWT)
	req.Header.Set(utils.VersionHeaderName, c.config.Version)
	req.Header.Set(utils.ModeHeaderName, strings.Join(c.config.Modes, ","))
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making HTTP request: %w", err)
	}
	defer func() {
		err = res.Body.Close()
		if err != nil {
			log.Errorf("error closing request body: %v", err)
		}
	}()

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading body of HTTP response: %w", err)
	}

	if len(expectedStatusCodes) != 0 && !slices.Contains(expectedStatusCodes, res.StatusCode) {
		return nil, fmt.Errorf("request failed with status code %d and body %s", res.StatusCode, resBody)
	}

	return resBody, nil
}

// EnrollmentClient provides enrollment functionality without requiring a full config
type EnrollmentClient struct {
	httpClient *http.Client
	host       string
}

// NewEnrollmentClient creates a new enrollment client
func NewEnrollmentClient(host string) *EnrollmentClient {
	return &EnrollmentClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		host: host,
	}
}

// SendEnrollmentJWT sends a JWT string as enrollment request body (following reference implementation)
func (c *EnrollmentClient) SendEnrollmentJWT(ctx context.Context, jwtBody string) (*EnrollmentResponse, error) {
	u := &url.URL{
		Scheme: "https",
		Host:   c.host,
		Path:   enrollmentPath,
	}

	// Create HTTP request with JWT string as body
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(jwtBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/jose+json")
	httpReq.Header.Set(utils.VersionHeaderName, version.AgentVersion)

	// Send request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send enrollment request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("enrollment request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSONAPI response
	var jsonapiResp EnrollmentJSONAPIResponse
	if err := json.Unmarshal(body, &jsonapiResp); err != nil {
		return nil, fmt.Errorf("failed to decode JSONAPI enrollment response: %w", err)
	}

	// Convert to simplified response structure
	enrollmentResp := &EnrollmentResponse{
		ID:               jsonapiResp.Data.ID,
		RunnerId:         jsonapiResp.Data.Attributes.RunnerId,
		OrgId:            jsonapiResp.Data.Attributes.OrgId,
		Modes:            jsonapiResp.Data.Attributes.RunnerModes,
		ActionsAllowlist: jsonapiResp.Data.Attributes.ActionsAllowlist,
	}

	return enrollmentResp, nil
}
