// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

const (
	// RemoteQueryExecuteEndpointPath is mounted under /agent by the Agent command API.
	RemoteQueryExecuteEndpointPath = "/remote-queries/execute"
	// RemoteQueriesExecuteEnabledConfig is disabled by default when the key is absent.
	RemoteQueriesExecuteEnabledConfig = "remote_queries.execute.enabled"
	// RemoteQueriesEnableQueryAllowlistConfig controls the proof-query allowlist; missing defaults to enabled.
	RemoteQueriesEnableQueryAllowlistConfig = "remote_queries.execute.enable_query_allowlist"
	// RemoteQueriesExecuteTimeoutMsConfig is the agent-owned statement timeout for remote queries, in
	// milliseconds. Unlike the size limits, the timeout is DB-protective — it caps how long the
	// integration's read-only transaction holds a snapshot on the customer's database — so it belongs to
	// the Agent, not the backend worker: a positive value overrides the injected limits.timeoutMs.
	RemoteQueriesExecuteTimeoutMsConfig = "remote_queries.execute.timeout_ms"

	// RemoteQueryOperationProduceJSONPages is the one supported integration operation:
	// produce bounded JSON page files and upload them directly to its-agent-intake.
	RemoteQueryOperationProduceJSONPages = "produce_json_pages"

	// RemoteQueryArtifactVersion is the page artifact contract version injected by the
	// backend. The Agent rejects drift early; the integration pins it too.
	RemoteQueryArtifactVersion = 1

	remoteQueryProofSeedQuery           = "SELECT 1 AS value"
	remoteQueryFixtureTableProofQuery   = "SELECT city, country FROM cities ORDER BY city"
	remoteQueryMatrixIdentityProofQuery = "SELECT current_database() AS current_db, expected_agent_hostname, expected_postgres_host, expected_postgres_port, expected_dbname, marker FROM remote_query_identity"

	statusExecutorUnavailable = "executor_unavailable"
)

const remoteQueryBinaryPayloadProofQuery = "SELECT decode('00ff80', 'hex') AS payload"

var remoteQueryLargePayloadProofQueries = map[string]int{
	"SELECT repeat('x', 1048576) AS payload":  1 << 20,  // 1 MiB.
	"SELECT repeat('x', 2097152) AS payload":  2 << 20,  // 2 MiB.
	"SELECT repeat('x', 4194304) AS payload":  4 << 20,  // 4 MiB.
	"SELECT repeat('x', 8388608) AS payload":  8 << 20,  // 8 MiB.
	"SELECT repeat('x', 16777216) AS payload": 16 << 20, // 16 MiB.
	"SELECT repeat('x', 33554432) AS payload": 32 << 20, // 32 MiB.
}

// Hard forwarding caps for the backend-injected upload instructions. The backend owns the
// effective values; the Agent enforces these ceilings fail-closed so an oversized or
// malformed handle never reaches the integration. The page cap ceiling matches the
// its-agent-intake platform ceiling; the total cap matches the backend-owned 10 GiB result
// ceiling; the part cap matches the 128 MiB multipart part clamp.
const (
	remoteQueryUploadMaxPartBytes   = 128 << 20 // 128 MiB hard part cap
	remoteQueryUploadMaxFileBytes   = 128 << 20 // 128 MiB hard page cap ceiling
	remoteQueryUploadMaxResultBytes = 10 << 30  // 10 GiB hard total cap
)

var (
	remoteQueryUploadIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

	errDeliveryUnknownField = errors.New("resultDelivery contains unknown field")
	errDeliveryMustBeObject = errors.New("resultDelivery must be an object")
)

var (
	errLimitsUnknownField = errors.New("limits contains unknown field")
	errLimitsMustBeObject = errors.New("limits must be an object")
)

type remoteQueryStreamRunner interface {
	RunRemoteQueryStream(integration string, requestJSON string, emit func(check.RemoteQueryStreamEvent) error) error
}

// RemoteQueriesQueryAllowlistEnabled returns the effective proof-query allowlist setting.
func RemoteQueriesQueryAllowlistEnabled(cfg interface {
	IsConfigured(key string) bool
	GetBool(key string) bool
}) bool {
	if cfg == nil || !cfg.IsConfigured(RemoteQueriesEnableQueryAllowlistConfig) {
		return true
	}
	return cfg.GetBool(RemoteQueriesEnableQueryAllowlistConfig)
}

// remoteQueryDeliveryWithStatementTimeout applies the agent-owned statement timeout to the
// backend-injected delivery limits. The timeout is the DB-protective limit class — it caps how long
// the integration's read-only transaction can hold a snapshot on the customer's database — so
// the Agent config owns it: a positive remote_queries.execute.timeout_ms replaces the injected
// limits.timeoutMs, and a non-positive or missing value falls back to the injected limit. The
// returned delivery is a copy, so the caller's request payload stays untouched.
func remoteQueryDeliveryWithStatementTimeout(delivery *RemoteQueryResultDelivery, cfg config.Component) *RemoteQueryResultDelivery {
	if delivery == nil || delivery.Limits == nil || cfg == nil {
		return delivery
	}
	timeoutMs := cfg.GetInt(RemoteQueriesExecuteTimeoutMsConfig)
	if timeoutMs <= 0 {
		return delivery
	}
	overridden := *delivery
	limits := *delivery.Limits
	limits.TimeoutMs = timeoutMs
	overridden.Limits = &limits
	return &overridden
}

func isRemoteQueryAllowedProofQuery(query string) bool {
	switch query {
	case remoteQueryProofSeedQuery, remoteQueryFixtureTableProofQuery, remoteQueryMatrixIdentityProofQuery, remoteQueryBinaryPayloadProofQuery:
		return true
	default:
		_, ok := remoteQueryLargePayloadProofQueries[query]
		return ok
	}
}

type remoteQueryCheckUnwrapper interface {
	Unwrap() check.Check
}

func remoteQueryStreamRunnerFor(chk check.Check) (remoteQueryStreamRunner, bool) {
	for chk != nil {
		if runner, ok := chk.(remoteQueryStreamRunner); ok {
			return runner, true
		}
		unwrapper, ok := chk.(remoteQueryCheckUnwrapper)
		if !ok {
			break
		}
		unwrapped := unwrapper.Unwrap()
		if unwrapped == chk {
			break
		}
		chk = unwrapped
	}
	return nil, false
}

// NewRemoteQueryExecuteEndpointProvider registers the remote query execute endpoint on the internal Agent API.
func NewRemoteQueryExecuteEndpointProvider(reqs Requires) api.AgentEndpointProvider {
	h := &remoteQueryExecuteHandler{
		service: NewRemoteQueryExecuteService(reqs.Collector, reqs.Cfg.GetBool(RemoteQueriesExecuteEnabledConfig), RemoteQueriesQueryAllowlistEnabled(reqs.Cfg), reqs.Cfg),
		cfg:     reqs.Cfg,
	}
	return api.NewAgentEndpointProvider(h.handle, RemoteQueryExecuteEndpointPath, http.MethodPost)
}

type remoteQueryExecuteHandler struct {
	service               *RemoteQueryExecuteService
	collector             RemoteQueryCollector
	enabled               bool
	queryAllowlistEnabled bool
	cfg                   config.Component
}

// RemoteQueryExecuteService executes credential-free Remote Queries requests through loaded checks.
type RemoteQueryExecuteService struct {
	collector             RemoteQueryCollector
	enabled               bool
	queryAllowlistEnabled bool
	cfg                   config.Component
}

// NewRemoteQueryExecuteService creates the shared executor used by the HTTP POC endpoint and AgentSecure RPC.
func NewRemoteQueryExecuteService(collector RemoteQueryCollector, enabled bool, queryAllowlistEnabled bool, cfg config.Component) *RemoteQueryExecuteService {
	return &RemoteQueryExecuteService{collector: collector, enabled: enabled, queryAllowlistEnabled: queryAllowlistEnabled, cfg: cfg}
}

// RemoteQueryExecuteTarget identifies the datastore target without carrying credentials.
type RemoteQueryExecuteTarget struct {
	Host             string
	Port             int
	DBName           string
	DatabaseInstance string
}

// RemoteQueryUploadLimits contains the backend-injected effective limits of the paged-JSON
// result contract. Every value is server-owned — the Agent forwards them opaquely and the
// integration enforces them — except TimeoutMs, the DB-protective statement timeout class,
// which is agent-owned: the Agent overrides it with remote_queries.execute.timeout_ms
// before the request reaches the integration.
type RemoteQueryUploadLimits struct {
	MaxFileBytes   int
	MaxResultBytes int
	MaxRowBytes    int
	MaxColumns     int
	MaxSchemaBytes int
	MaxPages       int
	TimeoutMs      int
}

// RemoteQueryResultDelivery carries the backend-injected upload-session instructions for
// one run: the authoritative run/task identity used by every page envelope, the page
// artifact contract version, the scoped its-agent-intake session handle (including the
// base URL and upload token), and the effective limits. The Agent forwards the full
// handle, including BaseURL and Token, to the integration check, which performs the HTTP
// upload to its-agent-intake itself; the Agent performs no URL allowlisting or HTTP
// transport and never logs the handle.
type RemoteQueryResultDelivery struct {
	RunID           string
	TaskID          string
	ArtifactVersion int
	UploadID        string
	BaseURL         string
	Token           string
	PartBytes       int
	Limits          *RemoteQueryUploadLimits
}

// RemoteQueryExecuteRequest is the typed request shared by the HTTP and gRPC callers. The
// result contract is fixed: operation produce_json_pages with a required result delivery;
// there is no inline result-byte path and no caller-provided format.
type RemoteQueryExecuteRequest struct {
	Integration    string
	Target         RemoteQueryExecuteTarget
	Query          string
	IncludeSchema  bool
	ResultDelivery *RemoteQueryResultDelivery
}

// validateRemoteQueryResultDelivery validates the backend-injected upload instructions.
// The handle is required: every run uploads bounded JSON page files directly to
// its-agent-intake. The Agent does not allowlist the base URL (the intake mints and owns
// it) and never logs the token; it only checks shape and the forwarding caps fail-closed.
func validateRemoteQueryResultDelivery(delivery *RemoteQueryResultDelivery) (*RemoteQueryResultDelivery, error) {
	if delivery == nil {
		return nil, errors.New("result_delivery is required")
	}
	if delivery.RunID == "" {
		return nil, errors.New("result_delivery.runId is required")
	}
	if delivery.TaskID == "" {
		return nil, errors.New("result_delivery.taskId is required")
	}
	if delivery.ArtifactVersion != RemoteQueryArtifactVersion {
		return nil, fmt.Errorf("result_delivery.artifactVersion must be %d", RemoteQueryArtifactVersion)
	}
	if delivery.UploadID == "" {
		return nil, errors.New("result_delivery.uploadId is required")
	}
	if !remoteQueryUploadIDPattern.MatchString(delivery.UploadID) {
		return nil, errors.New("result_delivery.uploadId contains invalid characters")
	}
	if delivery.BaseURL == "" {
		return nil, errors.New("result_delivery.baseUrl is required")
	}
	if delivery.Token == "" {
		return nil, errors.New("result_delivery.token is required")
	}
	if delivery.PartBytes < 1 {
		return nil, errors.New("result_delivery.partBytes must be at least 1")
	}
	if delivery.PartBytes > remoteQueryUploadMaxPartBytes {
		return nil, fmt.Errorf("result_delivery.partBytes must not exceed %d", remoteQueryUploadMaxPartBytes)
	}
	limits := delivery.Limits
	if limits == nil {
		return nil, errors.New("result_delivery.limits is required")
	}
	if limits.MaxFileBytes < 1 {
		return nil, errors.New("result_delivery.limits.maxFileBytes must be at least 1")
	}
	if limits.MaxFileBytes > remoteQueryUploadMaxFileBytes {
		return nil, fmt.Errorf("result_delivery.limits.maxFileBytes must not exceed %d", remoteQueryUploadMaxFileBytes)
	}
	if limits.MaxResultBytes < 1 {
		return nil, errors.New("result_delivery.limits.maxResultBytes must be at least 1")
	}
	if limits.MaxResultBytes > remoteQueryUploadMaxResultBytes {
		return nil, fmt.Errorf("result_delivery.limits.maxResultBytes must not exceed %d", remoteQueryUploadMaxResultBytes)
	}
	if limits.MaxRowBytes < 1 {
		return nil, errors.New("result_delivery.limits.maxRowBytes must be at least 1")
	}
	if limits.MaxColumns < 1 {
		return nil, errors.New("result_delivery.limits.maxColumns must be at least 1")
	}
	if limits.MaxSchemaBytes < 1 {
		return nil, errors.New("result_delivery.limits.maxSchemaBytes must be at least 1")
	}
	if limits.MaxPages < 1 {
		return nil, errors.New("result_delivery.limits.maxPages must be at least 1")
	}
	if limits.TimeoutMs < 1 {
		return nil, errors.New("result_delivery.limits.timeoutMs must be at least 1")
	}
	if limits.MaxRowBytes > limits.MaxFileBytes {
		return nil, errors.New("result_delivery.limits.maxRowBytes must not exceed maxFileBytes")
	}
	if limits.MaxFileBytes > limits.MaxResultBytes {
		return nil, errors.New("result_delivery.limits.maxFileBytes must not exceed maxResultBytes")
	}
	if delivery.PartBytes > limits.MaxFileBytes {
		return nil, errors.New("result_delivery.partBytes must not exceed limits.maxFileBytes")
	}
	return delivery, nil
}

// NewRemoteQueryExecuteRequest validates and normalizes a typed paged-JSON request.
func NewRemoteQueryExecuteRequest(integration string, target RemoteQueryExecuteTarget, query string, includeSchema bool, resultDelivery *RemoteQueryResultDelivery) (RemoteQueryExecuteRequest, error) {
	parsedIntegration, err := parseIntegration(integration)
	if err != nil {
		return RemoteQueryExecuteRequest{}, err
	}
	parsedTarget, err := parseExecuteTarget(target)
	if err != nil {
		return RemoteQueryExecuteRequest{}, err
	}
	if query == "" {
		return RemoteQueryExecuteRequest{}, errors.New("query is required")
	}
	normalizedDelivery, err := validateRemoteQueryResultDelivery(resultDelivery)
	if err != nil {
		return RemoteQueryExecuteRequest{}, err
	}
	req := remoteQueryExecuteRequestFromInternal(remoteQueryExecuteRequest{
		Integration:    parsedIntegration,
		Target:         parsedTarget,
		Query:          query,
		IncludeSchema:  includeSchema,
		ResultDelivery: normalizedDelivery,
	})
	return req, nil
}

// RemoteQueryExecuteError is a sanitized remote query bridge error.
type RemoteQueryExecuteError struct {
	Code    string
	Message string
}

// RemoteQueryExecuteResult is the service result.
type RemoteQueryExecuteResult struct {
	HTTPStatus int
	Status     string
	Error      *RemoteQueryExecuteError
}

const (
	// RemoteQueryStatusInvalidRequest reports a malformed or disallowed request.
	RemoteQueryStatusInvalidRequest = statusInvalidRequest
	// RemoteQueryStatusExecutorUnavailable reports an unavailable matched executor or bridge dependency.
	RemoteQueryStatusExecutorUnavailable = statusExecutorUnavailable
)

type remoteQueryExecuteRequest struct {
	Integration    string
	Target         remoteQueryTarget
	Query          string
	IncludeSchema  bool
	ResultDelivery *RemoteQueryResultDelivery
}

type remoteQueryExecuteRequestJSON struct {
	Integration    string                                `json:"integration"`
	Target         *remoteQueryTargetRequestJSON         `json:"target"`
	Query          string                                `json:"query"`
	IncludeSchema  bool                                  `json:"includeSchema"`
	ResultDelivery *remoteQueryResultDeliveryRequestJSON `json:"resultDelivery"`
}

// remoteQueryResultDeliveryRequestJSON is the HTTP wire shape of the backend-injected
// upload instructions. Pointers distinguish an omitted field from an explicit value so
// required-field checks fail closed instead of silently defaulting.
type remoteQueryResultDeliveryRequestJSON struct {
	RunID           string                              `json:"runId"`
	TaskID          string                              `json:"taskId"`
	ArtifactVersion *int                                `json:"artifactVersion"`
	UploadID        string                              `json:"uploadId"`
	BaseURL         string                              `json:"baseUrl"`
	Token           string                              `json:"token"`
	PartBytes       *int                                `json:"partBytes"`
	Limits          *remoteQueryUploadLimitsRequestJSON `json:"limits"`
}

type remoteQueryUploadLimitsRequestJSON struct {
	MaxFileBytes   *int `json:"maxFileBytes"`
	MaxResultBytes *int `json:"maxResultBytes"`
	MaxRowBytes    *int `json:"maxRowBytes"`
	MaxColumns     *int `json:"maxColumns"`
	MaxSchemaBytes *int `json:"maxSchemaBytes"`
	MaxPages       *int `json:"maxPages"`
	TimeoutMs      *int `json:"timeoutMs"`
}

func (d *remoteQueryResultDeliveryRequestJSON) UnmarshalJSON(data []byte) error {
	if !isJSONObject(data) {
		return errDeliveryMustBeObject
	}

	type deliveryAlias remoteQueryResultDeliveryRequestJSON
	var delivery deliveryAlias
	if err := decodeStrictJSON(bytes.NewReader(data), &delivery); err != nil {
		if isUnknownJSONFieldError(err) {
			return errDeliveryUnknownField
		}
		return mapResultDeliveryTypeError(err)
	}
	*d = remoteQueryResultDeliveryRequestJSON(delivery)
	return nil
}

func (l *remoteQueryUploadLimitsRequestJSON) UnmarshalJSON(data []byte) error {
	if !isJSONObject(data) {
		return errLimitsMustBeObject
	}

	type limitsAlias remoteQueryUploadLimitsRequestJSON
	var limits limitsAlias
	if err := decodeStrictJSON(bytes.NewReader(data), &limits); err != nil {
		if isUnknownJSONFieldError(err) {
			return errLimitsUnknownField
		}
		return mapResultDeliveryLimitsTypeError(err)
	}
	*l = remoteQueryUploadLimitsRequestJSON(limits)
	return nil
}

// remoteQueryDeliveryTypeError is the mapped type error for a resultDelivery field.
type remoteQueryDeliveryTypeError struct {
	message string
}

func (e remoteQueryDeliveryTypeError) Error() string {
	return e.message
}

func mapResultDeliveryTypeError(err error) error {
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) {
		return err
	}
	switch typeErr.Field {
	case "limits":
		return errLimitsMustBeObject
	case "runId", "taskId", "uploadId", "baseUrl", "token":
		return remoteQueryDeliveryTypeError{message: fmt.Sprintf("resultDelivery.%s must be a string", typeErr.Field)}
	case "artifactVersion", "partBytes":
		return remoteQueryDeliveryTypeError{message: fmt.Sprintf("resultDelivery.%s must be an integer", typeErr.Field)}
	default:
		return err
	}
}

func mapResultDeliveryLimitsTypeError(err error) error {
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) {
		return err
	}
	return remoteQueryDeliveryTypeError{message: fmt.Sprintf("resultDelivery.limits.%s must be an integer", typeErr.Field)}
}

// remoteQueryProduceJSONPagesRequestJSON is the integration request marshaled for the
// rtloader bridge. Keys are the camelCase integration contract and the shape carries no
// credentials: the integration reads the org API/application keys from Agent config.
type remoteQueryProduceJSONPagesRequestJSON struct {
	Operation      string                         `json:"operation"`
	Target         remoteQueryTargetJSON          `json:"target"`
	Query          string                         `json:"query"`
	IncludeSchema  bool                           `json:"includeSchema"`
	ResultDelivery *remoteQueryResultDeliveryJSON `json:"resultDelivery"`
}

// remoteQueryResultDeliveryJSON is the result-delivery handle forwarded to the
// integration check inside the request JSON. It carries baseUrl and token so the
// integration can perform the HTTP page uploads to its-agent-intake directly.
type remoteQueryResultDeliveryJSON struct {
	RunID           string                      `json:"runId"`
	TaskID          string                      `json:"taskId"`
	ArtifactVersion int                         `json:"artifactVersion"`
	UploadID        string                      `json:"uploadId"`
	BaseURL         string                      `json:"baseUrl"`
	Token           string                      `json:"token"`
	PartBytes       int                         `json:"partBytes"`
	Limits          remoteQueryUploadLimitsJSON `json:"limits"`
}

type remoteQueryUploadLimitsJSON struct {
	MaxFileBytes   int `json:"maxFileBytes"`
	MaxResultBytes int `json:"maxResultBytes"`
	MaxRowBytes    int `json:"maxRowBytes"`
	MaxColumns     int `json:"maxColumns"`
	MaxSchemaBytes int `json:"maxSchemaBytes"`
	MaxPages       int `json:"maxPages"`
	TimeoutMs      int `json:"timeoutMs"`
}

type remoteQueryTargetJSON struct {
	Host             string `json:"host,omitempty"`
	Port             int    `json:"port,omitempty"`
	DBName           string `json:"dbname,omitempty"`
	DatabaseInstance string `json:"database_instance,omitempty"`
}

func (h *remoteQueryExecuteHandler) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	service := h.service
	if service == nil {
		service = NewRemoteQueryExecuteService(h.collector, h.enabled, h.queryAllowlistEnabled, h.cfg)
	}
	if service == nil || !service.enabled {
		writeExecuteError(w, http.StatusServiceUnavailable, statusBridgeDisabled, "remote queries bridge is disabled")
		return
	}

	req, err := parseExecuteRequest(r)
	if err != nil {
		writeExecuteParseError(w, err)
		return
	}

	result := service.Execute(req)
	if result.Error != nil {
		writeExecuteError(w, result.HTTPStatus, result.Error.Code, result.Error.Message)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `{"status":"SUCCEEDED"}`)
}

func parseExecuteRequest(r *http.Request) (RemoteQueryExecuteRequest, error) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return RemoteQueryExecuteRequest{}, invalidRequestError("content-type must be application/json")
	}

	defer r.Body.Close()
	var wireReq remoteQueryExecuteRequestJSON
	if err := decodeStrictJSON(r.Body, &wireReq); err != nil {
		return RemoteQueryExecuteRequest{}, parseJSONRequestError(err)
	}

	// The wire target carries field-presence information, so selector-mode validation
	// happens here; the reconstructed typed target is then unambiguous.
	parsedTarget, err := parseTarget(wireReq.Target)
	if err != nil {
		return RemoteQueryExecuteRequest{}, err
	}
	if wireReq.Query == "" {
		return RemoteQueryExecuteRequest{}, errors.New("query is required")
	}
	delivery, err := resultDeliveryFromWire(wireReq.ResultDelivery)
	if err != nil {
		return RemoteQueryExecuteRequest{}, err
	}
	return NewRemoteQueryExecuteRequest(
		wireReq.Integration,
		RemoteQueryExecuteTarget(parsedTarget),
		wireReq.Query,
		wireReq.IncludeSchema,
		delivery,
	)
}

// resultDeliveryFromWire builds the typed delivery from the strict HTTP wire shape. The
// wire pointers make omitted required fields explicit errors instead of silent zeros.
func resultDeliveryFromWire(delivery *remoteQueryResultDeliveryRequestJSON) (*RemoteQueryResultDelivery, error) {
	if delivery == nil {
		return nil, errors.New("result_delivery is required")
	}
	if delivery.ArtifactVersion == nil {
		return nil, errors.New("result_delivery.artifactVersion is required")
	}
	if delivery.PartBytes == nil {
		return nil, errors.New("result_delivery.partBytes is required")
	}
	limits := delivery.Limits
	if limits == nil {
		return nil, errors.New("result_delivery.limits is required")
	}
	if limits.MaxFileBytes == nil {
		return nil, errors.New("result_delivery.limits.maxFileBytes is required")
	}
	if limits.MaxResultBytes == nil {
		return nil, errors.New("result_delivery.limits.maxResultBytes is required")
	}
	if limits.MaxRowBytes == nil {
		return nil, errors.New("result_delivery.limits.maxRowBytes is required")
	}
	if limits.MaxColumns == nil {
		return nil, errors.New("result_delivery.limits.maxColumns is required")
	}
	if limits.MaxSchemaBytes == nil {
		return nil, errors.New("result_delivery.limits.maxSchemaBytes is required")
	}
	if limits.MaxPages == nil {
		return nil, errors.New("result_delivery.limits.maxPages is required")
	}
	if limits.TimeoutMs == nil {
		return nil, errors.New("result_delivery.limits.timeoutMs is required")
	}
	return &RemoteQueryResultDelivery{
		RunID:           delivery.RunID,
		TaskID:          delivery.TaskID,
		ArtifactVersion: *delivery.ArtifactVersion,
		UploadID:        delivery.UploadID,
		BaseURL:         delivery.BaseURL,
		Token:           delivery.Token,
		PartBytes:       *delivery.PartBytes,
		Limits: &RemoteQueryUploadLimits{
			MaxFileBytes:   *limits.MaxFileBytes,
			MaxResultBytes: *limits.MaxResultBytes,
			MaxRowBytes:    *limits.MaxRowBytes,
			MaxColumns:     *limits.MaxColumns,
			MaxSchemaBytes: *limits.MaxSchemaBytes,
			MaxPages:       *limits.MaxPages,
			TimeoutMs:      *limits.TimeoutMs,
		},
	}, nil
}

// Execute rejects inline HTTP execution: Remote Queries execute over the AgentSecure
// streaming RPC only, and bulk result bytes never cross any Agent boundary.
func (s *RemoteQueryExecuteService) Execute(_ RemoteQueryExecuteRequest) RemoteQueryExecuteResult {
	return remoteQueryExecuteErrorResult(http.StatusBadRequest, statusInvalidRequest, "remote queries execute only over the AgentSecure streaming RPC")
}

// ExecuteStream executes a paged-JSON request and emits metadata-only stream events. The
// Agent is a control-plane forwarder: it carries the backend-injected upload instructions
// through to the integration request JSON and passes the emit callback straight through.
// The one limit the Agent rewrites is the DB-protective statement timeout: a positive
// remote_queries.execute.timeout_ms config value overrides the injected limits.timeoutMs
// before the request reaches the integration. The integration uploads bounded JSON page
// files itself; only progress metadata, the final compact run receipt, and errors come back
// through the stream.
func (s *RemoteQueryExecuteService) ExecuteStream(_ctx context.Context, req RemoteQueryExecuteRequest, emit func(check.RemoteQueryStreamEvent) error) RemoteQueryExecuteResult {
	if emit == nil {
		return remoteQueryExecuteErrorResult(http.StatusFailedDependency, statusExecutorUnavailable, "remote query stream emitter is unavailable")
	}
	if s == nil || !s.enabled {
		return remoteQueryExecuteErrorResult(http.StatusServiceUnavailable, statusBridgeDisabled, "remote queries bridge is disabled")
	}
	if s.collector == nil {
		return remoteQueryExecuteErrorResult(http.StatusFailedDependency, statusExecutorUnavailable, "remote query executor is unavailable")
	}
	if req.Query == "" {
		return remoteQueryExecuteErrorResult(http.StatusBadRequest, statusInvalidRequest, "query is required")
	}
	if req.ResultDelivery == nil {
		return remoteQueryExecuteErrorResult(http.StatusBadRequest, statusInvalidRequest, "result_delivery is required")
	}
	if s.queryAllowlistEnabled && !isRemoteQueryAllowedProofQuery(req.Query) {
		return remoteQueryExecuteErrorResult(http.StatusBadRequest, statusInvalidRequest, "query is not allowed")
	}

	internal := req.internal()
	internal.ResultDelivery = remoteQueryDeliveryWithStatementTimeout(internal.ResultDelivery, s.cfg)
	match, result := s.matchExecutor(internal)
	if result.Error != nil {
		return result
	}
	runner, ok := remoteQueryStreamRunnerFor(match.check)
	if !ok {
		return remoteQueryExecuteErrorResult(http.StatusFailedDependency, statusExecutorUnavailable, "matched integration check does not support remote query streaming")
	}
	requestJSON, err := marshalExecuteRequest(internal)
	if err != nil {
		return remoteQueryExecuteErrorResult(http.StatusBadRequest, statusInvalidRequest, err.Error())
	}

	runErr := runner.RunRemoteQueryStream(internal.Integration, requestJSON, emit)
	if runErr != nil {
		return remoteQueryExecuteErrorResult(http.StatusBadGateway, statusExecutorUnavailable, "remote query stream executor failed")
	}
	return RemoteQueryExecuteResult{HTTPStatus: http.StatusOK, Status: "SUCCEEDED"}
}

func (s *RemoteQueryExecuteService) matchExecutor(internal remoteQueryExecuteRequest) (integrationCheckMatch, RemoteQueryExecuteResult) {
	matches := findIntegrationMatches(s.collector, internal.Integration, internal.Target)
	switch len(matches) {
	case 0:
		return integrationCheckMatch{}, remoteQueryExecuteErrorResult(http.StatusNotFound, statusTargetNotFound, "no matching integration check found")
	case 1:
		return matches[0], RemoteQueryExecuteResult{HTTPStatus: http.StatusOK}
	default:
		return integrationCheckMatch{}, remoteQueryExecuteErrorResult(http.StatusConflict, statusAmbiguous, "multiple matching integration checks found")
	}
}

func remoteQueryExecuteErrorResult(httpStatus int, status string, message string) RemoteQueryExecuteResult {
	return RemoteQueryExecuteResult{
		HTTPStatus: httpStatus,
		Status:     status,
		Error:      &RemoteQueryExecuteError{Code: status, Message: message},
	}
}

func (r RemoteQueryExecuteRequest) internal() remoteQueryExecuteRequest {
	internal := remoteQueryExecuteRequest{
		Integration:   r.Integration,
		Target:        remoteQueryTarget{Host: r.Target.Host, Port: r.Target.Port, DBName: r.Target.DBName, DatabaseInstance: r.Target.DatabaseInstance},
		Query:         r.Query,
		IncludeSchema: r.IncludeSchema,
	}
	if r.ResultDelivery != nil {
		internal.ResultDelivery = r.ResultDelivery
	}
	return internal
}

func remoteQueryExecuteRequestFromInternal(req remoteQueryExecuteRequest) RemoteQueryExecuteRequest {
	return RemoteQueryExecuteRequest{
		Integration:    req.Integration,
		Target:         RemoteQueryExecuteTarget{Host: req.Target.Host, Port: req.Target.Port, DBName: req.Target.DBName, DatabaseInstance: req.Target.DatabaseInstance},
		Query:          req.Query,
		IncludeSchema:  req.IncludeSchema,
		ResultDelivery: req.ResultDelivery,
	}
}

func parseExecuteTarget(target RemoteQueryExecuteTarget) (remoteQueryTarget, error) {
	wireTarget := &remoteQueryTargetRequestJSON{Host: target.Host, DBName: target.DBName}
	if target.Host != "" {
		wireTarget.hostSet = true
	}
	if target.Port != 0 {
		wireTarget.Port = &target.Port
		wireTarget.portSet = true
	}
	if target.DBName != "" {
		wireTarget.dbnameSet = true
	}
	if target.DatabaseInstance != "" {
		wireTarget.DatabaseInstance = &target.DatabaseInstance
		wireTarget.databaseInstanceSet = true
	}
	return parseTarget(wireTarget)
}

// marshalExecuteRequest builds the integration request JSON. It always emits the fixed
// operation and the explicit includeSchema flag, and carries the full upload handle —
// including baseUrl and token — so the integration can upload page files directly. The
// integration reads the org API/application keys from Agent config, so they never appear
// on the request wire.
func marshalExecuteRequest(req remoteQueryExecuteRequest) (string, error) {
	if req.ResultDelivery == nil {
		return "", errors.New("result_delivery is required")
	}
	limits := req.ResultDelivery.Limits
	if limits == nil {
		return "", errors.New("result_delivery.limits is required")
	}
	wireReq := remoteQueryProduceJSONPagesRequestJSON{
		Operation:     RemoteQueryOperationProduceJSONPages,
		Target:        remoteQueryTargetJSON{Host: req.Target.Host, Port: req.Target.Port, DBName: req.Target.DBName, DatabaseInstance: req.Target.DatabaseInstance},
		Query:         req.Query,
		IncludeSchema: req.IncludeSchema,
		ResultDelivery: &remoteQueryResultDeliveryJSON{
			RunID:           req.ResultDelivery.RunID,
			TaskID:          req.ResultDelivery.TaskID,
			ArtifactVersion: req.ResultDelivery.ArtifactVersion,
			UploadID:        req.ResultDelivery.UploadID,
			BaseURL:         req.ResultDelivery.BaseURL,
			Token:           req.ResultDelivery.Token,
			PartBytes:       req.ResultDelivery.PartBytes,
			Limits: remoteQueryUploadLimitsJSON{
				MaxFileBytes:   limits.MaxFileBytes,
				MaxResultBytes: limits.MaxResultBytes,
				MaxRowBytes:    limits.MaxRowBytes,
				MaxColumns:     limits.MaxColumns,
				MaxSchemaBytes: limits.MaxSchemaBytes,
				MaxPages:       limits.MaxPages,
				TimeoutMs:      limits.TimeoutMs,
			},
		},
	}
	requestJSON, err := json.Marshal(wireReq)
	if err != nil {
		return "", err
	}
	return string(requestJSON), nil
}

func writeExecuteParseError(w http.ResponseWriter, err error) {
	parseErr, ok := err.(requestParseError)
	if !ok {
		writeExecuteError(w, http.StatusBadRequest, statusInvalidRequest, err.Error())
		return
	}

	writeExecuteError(w, http.StatusBadRequest, parseErr.status, parseErr.message)
}

func writeExecuteError(w http.ResponseWriter, httpStatus int, status string, message string) {
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(struct {
		Status string         `json:"status"`
		Error  *responseError `json:"error"`
	}{
		Status: status,
		Error:  &responseError{Code: status, Message: message},
	})
}
