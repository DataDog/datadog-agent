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
	"net/url"
	"regexp"
	"strconv"
	"strings"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

const (
	// RemoteQueryExecuteEndpointPath is mounted under /agent by the Agent command API.
	RemoteQueryExecuteEndpointPath = "/remote-queries/execute"
	// RemoteQueriesExecuteEnabledConfig is disabled by default when the key is absent.
	RemoteQueriesExecuteEnabledConfig = "remote_queries.execute.enabled"

	// RemoteQueryOperationProduceJSONPages is the one supported integration operation:
	// produce bounded JSON page files and upload them directly to its-agent-intake.
	RemoteQueryOperationProduceJSONPages = "produce_json_pages"

	// RemoteQueryArtifactVersion is the page artifact contract version injected by the
	// backend. The Agent rejects drift early; the integration pins it too.
	RemoteQueryArtifactVersion = 1

	statusExecutorUnavailable = "executor_unavailable"
)

// Hard forwarding caps for the backend-injected upload instructions. The backend owns the
// effective values; the Agent enforces these ceilings fail-closed so an oversized or
// malformed handle never reaches the integration. The page cap ceiling matches the
// its-agent-intake platform ceiling; the total cap matches the backend-owned 100 GiB result
// ceiling.
const (
	// Typed int64: the 100 GiB total cap overflows 32-bit int (armhf) wherever an
	// untyped use would default to int (comparisons, fmt.Errorf arguments).
	remoteQueryUploadMaxFileBytes   int64 = 128 << 20 // 128 MiB hard page cap ceiling
	remoteQueryUploadMaxResultBytes int64 = 100 << 30 // 100 GiB hard total cap
)

// RemoteQueryResultDeliveryProtocolSessionlessV1 is the explicit opt-in discriminator
// of the sessionless POC result-delivery and receipt shape. The session-backed shape
// carries no protocol key at all, so absence is the default and an unknown value is
// rejected rather than falling back to either shape.
const RemoteQueryResultDeliveryProtocolSessionlessV1 = "sessionless-v1"

// Experiment caps of the sessionless-v1 delivery limits, tighter than the session-backed
// forwarding ceilings above. The backend injects smaller worker limits; the Agent
// refuses to forward anything above these.
const (
	sessionlessMaxFileBytes   int64 = 16 << 20 // 16 MiB per page
	sessionlessMaxResultBytes int64 = 1 << 30  // 1 GiB per result
	sessionlessMaxPages             = 128
	sessionlessMaxColumns           = 1024
	sessionlessMaxSchemaBytes int64 = 1 << 20 // 1 MiB descriptor

	// sessionlessMaxReceiptObjects and sessionlessMaxReceiptBytes bound the final
	// sessionless-v1 upload receipt crossing the bridge.
	sessionlessMaxReceiptObjects = 128
	sessionlessMaxReceiptBytes   = 16384
)

var (
	remoteQueryUploadIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

	// remoteQueryObjectIDPattern is the canonical lowercase hyphenated UUID form of a
	// landing object reference. References are never normalized: an uppercase alias is
	// rejected, not lowercased.
	remoteQueryObjectIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

	errDeliveryUnknownField = errors.New("resultDelivery contains unknown field")
	errDeliveryMustBeObject = errors.New("resultDelivery must be an object")
)

var (
	errLimitsUnknownField = errors.New("limits contains unknown field")
	errLimitsMustBeObject = errors.New("limits must be an object")
)

// RemoteQueryTraceContext carries the optional trace-continuation metadata the
// private-action-runner attaches at the Remote Queries action boundary: the active
// trace ID, the action.run span's own ID as the downstream parent, and the effective
// sampling priority — restricted to the positive keep priorities 1 and 2, the exact
// domain the integration's strict carrier validation accepts. It is observability
// metadata only — never signed task data and never an authorization, routing, or
// result-validation input — and it is optional end to end: absent or invalid values
// leave the query executing exactly as before.
type RemoteQueryTraceContext struct {
	TraceID          uint64
	SpanID           uint64
	SamplingPriority int
}

const (
	// remoteQuerySamplingPriorityMin and remoteQuerySamplingPriorityMax bound the
	// only sampling-priority values that may cross the bridge: the tracer's positive
	// keep priorities, AutoKeep (1) and UserKeep (2), which are exactly the values the
	// integration's strict carrier validation accepts. A propagated priority outside
	// the keep domain — the tracer's drop priorities UserDrop (-1) and AutoDrop (0), or
	// any other integer — is unsupported trace context and is dropped, never fatal.
	remoteQuerySamplingPriorityMin = 1
	remoteQuerySamplingPriorityMax = 2
)

// NewRemoteQueryTraceContext validates the optional propagated trace context and
// returns the normalized value, or nil when the context is invalid: a zero trace or
// parent ID cannot identify a real trace, and only the positive keep sampling
// priorities 1 and 2 are supported — the integration's strict carrier validation
// accepts exactly those, so anything else (including the tracer's drop priorities
// -1 and 0) must never be emitted. Invalid context is dropped without failing the
// request, so an untraced or old caller executes exactly as before, and optional
// invalid metadata never becomes a strict query-validation failure downstream.
func NewRemoteQueryTraceContext(traceID, spanID uint64, samplingPriority int) *RemoteQueryTraceContext {
	if traceID == 0 || spanID == 0 {
		return nil
	}
	if samplingPriority < remoteQuerySamplingPriorityMin || samplingPriority > remoteQuerySamplingPriorityMax {
		return nil
	}
	return &RemoteQueryTraceContext{
		TraceID:          traceID,
		SpanID:           spanID,
		SamplingPriority: samplingPriority,
	}
}

// normalizeRemoteQueryTraceContext revalidates an already-typed trace context at the
// integration request boundary, so a hand-assembled typed request can emit only a
// supported context on the wire.
func normalizeRemoteQueryTraceContext(traceContext *RemoteQueryTraceContext) *RemoteQueryTraceContext {
	if traceContext == nil {
		return nil
	}
	return NewRemoteQueryTraceContext(traceContext.TraceID, traceContext.SpanID, traceContext.SamplingPriority)
}

type remoteQueryStreamRunner interface {
	RunRemoteQueryStream(integration string, requestJSON string, emit func(check.RemoteQueryStreamEvent) error) error
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
		service: NewRemoteQueryExecuteService(reqs.Collector, reqs.Cfg.GetBool(RemoteQueriesExecuteEnabledConfig), reqs.Cfg),
		cfg:     reqs.Cfg,
	}
	return api.NewAgentEndpointProvider(h.handle, RemoteQueryExecuteEndpointPath, http.MethodPost)
}

type remoteQueryExecuteHandler struct {
	service   *RemoteQueryExecuteService
	collector RemoteQueryCollector
	enabled   bool
	cfg       config.Component
}

// RemoteQueryExecuteService executes credential-free Remote Queries requests through loaded checks.
type RemoteQueryExecuteService struct {
	collector RemoteQueryCollector
	enabled   bool
	cfg       config.Component
}

// NewRemoteQueryExecuteService creates the shared executor used by the HTTP POC endpoint and AgentSecure RPC.
func NewRemoteQueryExecuteService(collector RemoteQueryCollector, enabled bool, cfg config.Component) *RemoteQueryExecuteService {
	return &RemoteQueryExecuteService{collector: collector, enabled: enabled, cfg: cfg}
}

// RemoteQueryExecuteTarget identifies the datastore target without carrying credentials.
type RemoteQueryExecuteTarget struct {
	Host             string
	Port             int
	DBName           string
	DatabaseInstance string
}

// RemoteQueryUploadLimits contains the backend-injected effective limits of the paged-JSON
// result contract. Every value is server-owned: the Agent forwards them opaquely and the
// integration enforces them.
type RemoteQueryUploadLimits struct {
	// Byte limits are int64 so the 10 GiB total cap is representable on 32-bit
	// platforms (armhf); row/column/page counts and the timeout stay int.
	MaxFileBytes   int64
	MaxResultBytes int64
	MaxRowBytes    int64
	MaxColumns     int
	MaxSchemaBytes int64
	MaxPages       int
	TimeoutMs      int
}

// RemoteQueryResultDelivery carries the backend-injected upload-session instructions for
// one run: the authoritative run/task identity used by every page envelope, the page
// artifact contract version, the scoped its-agent-intake session handle (including the
// base URL), and the effective limits. The session has one identifier, upload_id: there
// is no per-session upload token, so the Agent has no token to forward. The Agent
// forwards the handle, including BaseURL, to the integration check, which performs the
// HTTP upload to its-agent-intake itself and authenticates it with the Agent's org API
// and application keys; the Agent therefore only forwards a BaseURL that is on the
// Agent's own site (see validateResultDeliveryBaseURL), performs no HTTP transport, and
// never logs the handle.
//
// Two mutually exclusive shapes exist. The default session-backed shape has an empty
// Protocol, requires UploadID and carries no ScanSensitiveData. The opt-in sessionless
// shape has Protocol RemoteQueryResultDeliveryProtocolSessionlessV1, requires a
// non-nil ScanSensitiveData and forbids UploadID: it carries no upload ID, bucket,
// account routing, token, or trusted-scanning stamp. RunID keeps its wire name but is the
// fresh per-delivery crawl ID in that shape, not the ITS run ID.
type RemoteQueryResultDelivery struct {
	Protocol          string
	RunID             string
	TaskID            string
	ArtifactVersion   int
	UploadID          string
	BaseURL           string
	ScanSensitiveData *bool
	Limits            *RemoteQueryUploadLimits
}

func (d *RemoteQueryResultDelivery) sessionless() bool {
	return d != nil && d.Protocol == RemoteQueryResultDeliveryProtocolSessionlessV1
}

// RemoteQueryExecuteRequest is the typed request shared by the HTTP and gRPC callers. The
// result contract is fixed: operation produce_json_pages with a required result delivery;
// there is no inline result-byte path and no caller-provided format. There is no resolve-
// time binding: execute resolves the target fresh and executes
// on a unique match, whatever the check identity is at execute time.
type RemoteQueryExecuteRequest struct {
	Integration    string
	Target         RemoteQueryExecuteTarget
	Query          string
	IncludeSchema  bool
	ResultDelivery *RemoteQueryResultDelivery
	// TraceContext is the optional propagated trace-continuation metadata; nil
	// means the caller is untraced and the integration request omits the field.
	TraceContext *RemoteQueryTraceContext
}

// validateRemoteQueryResultDelivery validates the backend-injected upload instructions.
// The handle is required: every run uploads bounded JSON page files directly to
// its-agent-intake. This function checks the handle's shape and the forwarding caps
// fail-closed and never logs the handle. The base URL's site binding needs the Agent
// config, so it is enforced separately by validateResultDeliveryBaseURL before the
// handle is forwarded (ExecuteStream).
func validateRemoteQueryResultDelivery(delivery *RemoteQueryResultDelivery) (*RemoteQueryResultDelivery, error) {
	if delivery == nil {
		return nil, errors.New("result_delivery is required")
	}
	sessionless := false
	switch delivery.Protocol {
	case "":
		// The session-backed shape has no protocol key; a sessionless-only field
		// next to it is a mixed shape.
		if delivery.ScanSensitiveData != nil {
			return nil, errors.New("result_delivery.scanSensitiveData requires result_delivery.protocol")
		}
	case RemoteQueryResultDeliveryProtocolSessionlessV1:
		sessionless = true
		if delivery.UploadID != "" {
			return nil, fmt.Errorf("result_delivery.uploadId must not be set for protocol %s", RemoteQueryResultDeliveryProtocolSessionlessV1)
		}
		if delivery.ScanSensitiveData == nil {
			return nil, errors.New("result_delivery.scanSensitiveData is required")
		}
	default:
		return nil, fmt.Errorf("result_delivery.protocol must be omitted or %s", RemoteQueryResultDeliveryProtocolSessionlessV1)
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
	if !sessionless {
		if delivery.UploadID == "" {
			return nil, errors.New("result_delivery.uploadId is required")
		}
		if !remoteQueryUploadIDPattern.MatchString(delivery.UploadID) {
			return nil, errors.New("result_delivery.uploadId contains invalid characters")
		}
	}
	if delivery.BaseURL == "" {
		return nil, errors.New("result_delivery.baseUrl is required")
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
	if sessionless {
		if err := validateSessionlessLimits(limits); err != nil {
			return nil, err
		}
	}
	return delivery, nil
}

// validateSessionlessLimits enforces the tighter sessionless-v1 experiment caps on top of
// the shared limit invariants.
func validateSessionlessLimits(limits *RemoteQueryUploadLimits) error {
	if limits.MaxFileBytes > sessionlessMaxFileBytes {
		return fmt.Errorf("result_delivery.limits.maxFileBytes must not exceed %d for protocol %s", sessionlessMaxFileBytes, RemoteQueryResultDeliveryProtocolSessionlessV1)
	}
	if limits.MaxResultBytes > sessionlessMaxResultBytes {
		return fmt.Errorf("result_delivery.limits.maxResultBytes must not exceed %d for protocol %s", sessionlessMaxResultBytes, RemoteQueryResultDeliveryProtocolSessionlessV1)
	}
	if limits.MaxPages > sessionlessMaxPages {
		return fmt.Errorf("result_delivery.limits.maxPages must not exceed %d for protocol %s", sessionlessMaxPages, RemoteQueryResultDeliveryProtocolSessionlessV1)
	}
	if limits.MaxColumns > sessionlessMaxColumns {
		return fmt.Errorf("result_delivery.limits.maxColumns must not exceed %d for protocol %s", sessionlessMaxColumns, RemoteQueryResultDeliveryProtocolSessionlessV1)
	}
	if limits.MaxSchemaBytes > sessionlessMaxSchemaBytes {
		return fmt.Errorf("result_delivery.limits.maxSchemaBytes must not exceed %d for protocol %s", sessionlessMaxSchemaBytes, RemoteQueryResultDeliveryProtocolSessionlessV1)
	}
	return nil
}

// validateResultDeliveryBaseURL fails closed unless baseURL is safe to receive the
// Agent's org API and application keys: an absolute https URL with no userinfo, query or
// fragment, no port or port 443, whose host is the Agent's site or a subdomain of it. The
// path is not constrained. Errors name the field and the reason but never echo the URL:
// the handle is never logged, and url.Parse errors embed the input so they are not wrapped.
func validateResultDeliveryBaseURL(baseURL string, site string) error {
	const field = "result_delivery.baseUrl"
	if site == "" {
		return fmt.Errorf("%s cannot be checked: the Agent site is not configured", field)
	}
	if strings.ContainsAny(baseURL, "\\?#") || strings.ContainsFunc(baseURL, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return fmt.Errorf("%s must not contain a query, fragment, backslash, whitespace or control characters", field)
	}
	u, err := url.Parse(baseURL)
	if err != nil || !u.IsAbs() || u.Opaque != "" {
		return fmt.Errorf("%s must be an absolute URL", field)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%s must use the https scheme", field)
	}
	if u.User != nil {
		return fmt.Errorf("%s must not contain userinfo", field)
	}
	if port := u.Port(); port != "" && port != "443" {
		return fmt.Errorf("%s must not use a port other than 443", field)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || (host != site && !strings.HasSuffix(host, "."+site)) {
		return fmt.Errorf("%s must be on the Agent's site", field)
	}
	return nil
}

// site returns the Agent's configured site, trimmed and lower-cased. The config component
// defaults it to datadoghq.com; it is empty only when no config is available or the site
// was explicitly cleared, in which case validateResultDeliveryBaseURL fails closed.
func (s *RemoteQueryExecuteService) site() string {
	if s == nil || s.cfg == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(s.cfg.GetString("site")))
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
	TraceContext   *RemoteQueryTraceContext
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
//
// The session-backed shape carries no protocol key; the sessionless-v1 shape carries
// protocol and scanSensitiveData and no uploadId. The raw key presence is recorded by
// UnmarshalJSON so a mixed shape fails closed even when a stray value is null or empty.
type remoteQueryResultDeliveryRequestJSON struct {
	Protocol          *string                             `json:"protocol"`
	RunID             string                              `json:"runId"`
	TaskID            string                              `json:"taskId"`
	ArtifactVersion   *int                                `json:"artifactVersion"`
	UploadID          string                              `json:"uploadId"`
	BaseURL           string                              `json:"baseUrl"`
	ScanSensitiveData *bool                               `json:"scanSensitiveData"`
	Limits            *remoteQueryUploadLimitsRequestJSON `json:"limits"`

	// Wire key presence, set by UnmarshalJSON.
	protocolSet bool
	scanSet     bool
	uploadIDSet bool
}

type remoteQueryUploadLimitsRequestJSON struct {
	MaxFileBytes   *int64 `json:"maxFileBytes"`
	MaxResultBytes *int64 `json:"maxResultBytes"`
	MaxRowBytes    *int64 `json:"maxRowBytes"`
	MaxColumns     *int   `json:"maxColumns"`
	MaxSchemaBytes *int64 `json:"maxSchemaBytes"`
	MaxPages       *int   `json:"maxPages"`
	TimeoutMs      *int   `json:"timeoutMs"`
}

func (d *remoteQueryResultDeliveryRequestJSON) UnmarshalJSON(data []byte) error {
	if !isJSONObject(data) {
		return errDeliveryMustBeObject
	}
	// Go's decoder matches field names case-insensitively and keeps the last of a
	// duplicate, but the discriminator and mixed-shape checks below use exact key
	// presence. Refuse variant spellings and duplicates first, so a case alias can never
	// decode as a value yet read as absent and downgrade into the session shape.
	if err := checkExactJSONKeys(data, "resultDelivery", resultDeliveryWireKeys...); err != nil {
		return err
	}

	type deliveryAlias remoteQueryResultDeliveryRequestJSON
	var delivery deliveryAlias
	if err := decodeStrictJSON(bytes.NewReader(data), &delivery); err != nil {
		if isUnknownJSONFieldError(err) {
			return errDeliveryUnknownField
		}
		return mapResultDeliveryTypeError(err)
	}
	// Presence of the discriminating keys. A JSON null for protocol or
	// scanSensitiveData is a wrong type, not an omitted field.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return errDeliveryMustBeObject
	}
	*d = remoteQueryResultDeliveryRequestJSON(delivery)
	_, d.uploadIDSet = raw["uploadId"]
	if value, present := raw["protocol"]; present {
		if isJSONNull(value) {
			return remoteQueryDeliveryTypeError{message: "resultDelivery.protocol must be a string"}
		}
		d.protocolSet = true
	}
	if value, present := raw["scanSensitiveData"]; present {
		if isJSONNull(value) {
			return remoteQueryDeliveryTypeError{message: "resultDelivery.scanSensitiveData must be a boolean"}
		}
		d.scanSet = true
	}
	return nil
}

// resultDeliveryWireKeys and resultDeliveryLimitsWireKeys are the exact spellings of the
// resultDelivery and limits objects.
var (
	resultDeliveryWireKeys       = []string{"protocol", "runId", "taskId", "artifactVersion", "uploadId", "baseUrl", "scanSensitiveData", "limits"}
	resultDeliveryLimitsWireKeys = []string{"maxFileBytes", "maxResultBytes", "maxRowBytes", "maxColumns", "maxSchemaBytes", "maxPages", "timeoutMs"}
)

// checkExactJSONKeys guards the top level of one JSON object whose exact field names are
// part of a frozen shape. For the canonical names it rejects a duplicated key and a
// differently-cased spelling of a canonical name (which Go's decoder would otherwise match
// case-insensitively); unknown names are left to the caller's own unknown-field handling.
// Errors are sanitized: they name the object, never the offending key or value.
func checkExactJSONKeys(data []byte, object string, canonical ...string) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return remoteQueryDeliveryTypeError{message: object + " must be an object"}
	}
	seen := make(map[string]struct{}, len(canonical))
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return remoteQueryDeliveryTypeError{message: object + " is malformed"}
		}
		isCanonical := false
		for _, name := range canonical {
			if key == name {
				isCanonical = true
			} else if strings.EqualFold(key, name) {
				return remoteQueryDeliveryTypeError{message: object + " field names must be spelled exactly"}
			}
		}
		if isCanonical {
			if _, duplicate := seen[key]; duplicate {
				return remoteQueryDeliveryTypeError{message: object + " contains a duplicate field"}
			}
			seen[key] = struct{}{}
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return remoteQueryDeliveryTypeError{message: object + " is malformed"}
		}
	}
	return nil
}

func isJSONNull(data json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(data), []byte("null"))
}

func (l *remoteQueryUploadLimitsRequestJSON) UnmarshalJSON(data []byte) error {
	if !isJSONObject(data) {
		return errLimitsMustBeObject
	}
	if err := checkExactJSONKeys(data, "resultDelivery.limits", resultDeliveryLimitsWireKeys...); err != nil {
		return err
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
	case "protocol", "runId", "taskId", "uploadId", "baseUrl":
		return remoteQueryDeliveryTypeError{message: fmt.Sprintf("resultDelivery.%s must be a string", typeErr.Field)}
	case "scanSensitiveData":
		return remoteQueryDeliveryTypeError{message: "resultDelivery.scanSensitiveData must be a boolean"}
	case "artifactVersion":
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
	TraceContext   *remoteQueryTraceContextJSON   `json:"traceContext,omitempty"`
}

// remoteQueryTraceContextJSON is the optional trace-continuation object of the
// integration request. The IDs cross as unsigned decimal strings so the exact
// 64-bit value survives without numeric precision or format drift, and the
// sampling priority is an integer. Absence stays absent.
type remoteQueryTraceContextJSON struct {
	TraceID          string `json:"traceId"`
	ParentID         string `json:"parentId"`
	SamplingPriority int    `json:"samplingPriority"`
}

// remoteQueryResultDeliveryJSON is the result-delivery handle forwarded to the
// integration check inside the request JSON. It carries baseUrl so the integration
// can perform the HTTP page uploads to its-agent-intake directly; the session has one
// identifier, upload_id, and no upload token, so the handle cannot carry one.
//
// The session-backed shape is byte-for-byte unchanged: protocol and scanSensitiveData
// are omitted, uploadId is always present (validated non-empty). The sessionless-v1
// shape emits protocol and scanSensitiveData (an explicit false survives omitempty,
// because it is a pointer) and omits uploadId entirely — there is no dummy upload ID.
type remoteQueryResultDeliveryJSON struct {
	Protocol          string                      `json:"protocol,omitempty"`
	RunID             string                      `json:"runId"`
	TaskID            string                      `json:"taskId"`
	ArtifactVersion   int                         `json:"artifactVersion"`
	UploadID          string                      `json:"uploadId,omitempty"`
	BaseURL           string                      `json:"baseUrl"`
	ScanSensitiveData *bool                       `json:"scanSensitiveData,omitempty"`
	Limits            remoteQueryUploadLimitsJSON `json:"limits"`
}

type remoteQueryUploadLimitsJSON struct {
	MaxFileBytes   int64 `json:"maxFileBytes"`
	MaxResultBytes int64 `json:"maxResultBytes"`
	MaxRowBytes    int64 `json:"maxRowBytes"`
	MaxColumns     int   `json:"maxColumns"`
	MaxSchemaBytes int64 `json:"maxSchemaBytes"`
	MaxPages       int   `json:"maxPages"`
	TimeoutMs      int   `json:"timeoutMs"`
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
		service = NewRemoteQueryExecuteService(h.collector, h.enabled, h.cfg)
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
	req, err := NewRemoteQueryExecuteRequest(
		wireReq.Integration,
		RemoteQueryExecuteTarget(parsedTarget),
		wireReq.Query,
		wireReq.IncludeSchema,
		delivery,
	)
	if err != nil {
		return RemoteQueryExecuteRequest{}, err
	}
	return req, nil
}

// resultDeliveryFromWire builds the typed delivery from the strict HTTP wire shape. The
// wire pointers make omitted required fields explicit errors instead of silent zeros.
func resultDeliveryFromWire(delivery *remoteQueryResultDeliveryRequestJSON) (*RemoteQueryResultDelivery, error) {
	if delivery == nil {
		return nil, errors.New("result_delivery is required")
	}
	protocol := ""
	if delivery.protocolSet {
		if delivery.Protocol == nil || *delivery.Protocol != RemoteQueryResultDeliveryProtocolSessionlessV1 {
			return nil, fmt.Errorf("result_delivery.protocol must be omitted or %s", RemoteQueryResultDeliveryProtocolSessionlessV1)
		}
		protocol = *delivery.Protocol
	}
	// Mixed shapes are rejected by key presence, before any value is interpreted: the
	// session-backed shape cannot carry scanSensitiveData, the sessionless shape
	// cannot carry uploadId even as an empty string.
	if protocol == "" && delivery.scanSet {
		return nil, errors.New("result_delivery.scanSensitiveData requires result_delivery.protocol")
	}
	if protocol != "" && delivery.uploadIDSet {
		return nil, fmt.Errorf("result_delivery.uploadId must not be set for protocol %s", protocol)
	}
	if protocol != "" && delivery.ScanSensitiveData == nil {
		return nil, errors.New("result_delivery.scanSensitiveData is required")
	}
	if delivery.ArtifactVersion == nil {
		return nil, errors.New("result_delivery.artifactVersion is required")
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
		Protocol:          protocol,
		RunID:             delivery.RunID,
		TaskID:            delivery.TaskID,
		ArtifactVersion:   *delivery.ArtifactVersion,
		UploadID:          delivery.UploadID,
		BaseURL:           delivery.BaseURL,
		ScanSensitiveData: delivery.ScanSensitiveData,
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
// The integration uploads bounded JSON page files itself; only progress metadata, the
// final compact run receipt, and errors come back through the stream.
func (s *RemoteQueryExecuteService) ExecuteStream(ctx context.Context, req RemoteQueryExecuteRequest, emit func(check.RemoteQueryStreamEvent) error) RemoteQueryExecuteResult {
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
	// A typed request assembled without NewRemoteQueryExecuteRequest must not bypass the
	// discriminated shape checks: a mixed or unknown shape never reaches the integration.
	if _, err := validateRemoteQueryResultDelivery(req.ResultDelivery); err != nil {
		return remoteQueryExecuteErrorResult(http.StatusBadRequest, statusInvalidRequest, err.Error())
	}
	// The integration sends the Agent's org API and application keys to baseUrl, so it
	// must be on the Agent's own site; reject before target resolution or any forwarding.
	if err := validateResultDeliveryBaseURL(req.ResultDelivery.BaseURL, s.site()); err != nil {
		return remoteQueryExecuteErrorResult(http.StatusBadRequest, statusInvalidRequest, err.Error())
	}

	internal := req.internal()
	match, result := s.resolveExecutionTarget(internal)
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

	if ctx.Err() != nil {
		return remoteQueryExecuteErrorResult(http.StatusRequestTimeout, statusExecutorUnavailable, "remote query request was cancelled")
	}

	// A sessionless-v1 run reports its result only through the final upload receipt, so
	// the receipt is validated against the frozen shape before the final event is
	// forwarded. The session-backed stream is forwarded untouched.
	var receiptGuard *sessionlessReceiptGuard
	runEmit := emit
	if req.ResultDelivery.sessionless() {
		receiptGuard = &sessionlessReceiptGuard{includeSchema: req.IncludeSchema, limits: req.ResultDelivery.Limits, emit: emit}
		runEmit = receiptGuard.emitEvent
	}

	runErr := runner.RunRemoteQueryStream(internal.Integration, requestJSON, runEmit)
	if receiptGuard != nil && receiptGuard.err != nil {
		return remoteQueryExecuteErrorResult(http.StatusBadGateway, statusExecutorUnavailable, receiptGuard.err.Error())
	}
	if runErr != nil {
		return remoteQueryExecuteErrorResult(http.StatusBadGateway, statusExecutorUnavailable, "remote query stream executor failed")
	}
	if receiptGuard != nil && !receiptGuard.finalSeen {
		return remoteQueryExecuteErrorResult(http.StatusBadGateway, statusExecutorUnavailable, "sessionless remote query stream ended without a final event")
	}
	return RemoteQueryExecuteResult{HTTPStatus: http.StatusOK, Status: "SUCCEEDED"}
}

// sessionlessReceiptGuard validates the sessionless-v1 upload receipt carried by the
// integration's final event and forwards every event unchanged: the Agent only checks
// the receipt's shape and bounds, it never rewrites it, and the claims in it (object
// references and totals) stay untrusted until the backend reads the objects back.
type sessionlessReceiptGuard struct {
	includeSchema bool
	limits        *RemoteQueryUploadLimits
	emit          func(check.RemoteQueryStreamEvent) error
	// err is the sanitized reason the final receipt was refused; it never echoes
	// receipt content.
	err error
	// finalSeen records a validated final event: a sessionless run that ends without
	// one has no receipt to complete from.
	finalSeen bool
}

func (g *sessionlessReceiptGuard) emitEvent(event check.RemoteQueryStreamEvent) error {
	if event.Type == "final" {
		if err := validateSessionlessFinalEvent(event.MetadataJSON, g.includeSchema, g.limits); err != nil {
			g.err = err
			return err
		}
		g.finalSeen = true
	}
	return g.emit(event)
}

// sessionlessReceipt is the exact sessionless-v1 receipt shape. Pointers distinguish an
// omitted field from an explicit value, so every required field fails closed.
type sessionlessReceipt struct {
	Protocol   *string   `json:"protocol"`
	ObjectIDs  *[]string `json:"objectIds"`
	PageCount  *int64    `json:"pageCount"`
	TotalRows  *int64    `json:"totalRows"`
	TotalBytes *int64    `json:"totalBytes"`
}

var sessionlessReceiptKeys = []string{"protocol", "objectIds", "pageCount", "totalRows", "totalBytes"}

// ValidateSessionlessUploadReceipt validates a final-event upload_receipt object against
// the frozen sessionless-v1 shape and returns the ordered object references. It enforces
// exactly protocol, objectIds, pageCount, totalRows and totalBytes (unknown fields, a
// session-shaped uploadId, case-aliased or duplicated field names, wrong types and missing
// fields are rejected), canonical lowercase and unique UUID references (never normalized),
// at most 128 of them, a serialized size of at most 16 KiB, and
// that an empty reference list is only the zero-page, zero-row, zero-byte result of a
// run that requested no schema. pageCount must equal the number of references. limits is
// optional and, when set, bounds the claims by the run's own limits. It validates shape
// and bounds only: counts and totals are claims, never proof that the pages exist.
func ValidateSessionlessUploadReceipt(receiptJSON []byte, includeSchema bool, limits *RemoteQueryUploadLimits) ([]string, error) {
	if len(receiptJSON) > sessionlessMaxReceiptBytes {
		return nil, fmt.Errorf("sessionless upload receipt must not exceed %d bytes", sessionlessMaxReceiptBytes)
	}
	if !isJSONObject(receiptJSON) {
		return nil, errors.New("sessionless upload receipt must be an object")
	}
	// Exact field names, no duplicates: the decoder below would otherwise accept case
	// aliases and silently keep the last of a duplicated member.
	if err := checkExactJSONKeys(receiptJSON, "sessionless upload receipt", sessionlessReceiptKeys...); err != nil {
		return nil, err
	}
	var receipt sessionlessReceipt
	if err := decodeStrictJSON(bytes.NewReader(receiptJSON), &receipt); err != nil {
		var typeErr *json.UnmarshalTypeError
		switch {
		case isUnknownJSONFieldError(err):
			return nil, errors.New("sessionless upload receipt contains unknown field")
		case errors.As(err, &typeErr):
			return nil, fmt.Errorf("sessionless upload receipt field %s has the wrong type", typeErr.Field)
		}
		return nil, errors.New("sessionless upload receipt is malformed")
	}
	if receipt.Protocol == nil || *receipt.Protocol != RemoteQueryResultDeliveryProtocolSessionlessV1 {
		return nil, fmt.Errorf("sessionless upload receipt protocol must be %s", RemoteQueryResultDeliveryProtocolSessionlessV1)
	}
	if receipt.ObjectIDs == nil {
		return nil, errors.New("sessionless upload receipt objectIds is required")
	}
	if receipt.PageCount == nil || receipt.TotalRows == nil || receipt.TotalBytes == nil {
		return nil, errors.New("sessionless upload receipt pageCount, totalRows and totalBytes are required")
	}
	objectIDs := *receipt.ObjectIDs
	if len(objectIDs) > sessionlessMaxReceiptObjects {
		return nil, fmt.Errorf("sessionless upload receipt must not reference more than %d objects", sessionlessMaxReceiptObjects)
	}
	seen := make(map[string]struct{}, len(objectIDs))
	for _, id := range objectIDs {
		if !remoteQueryObjectIDPattern.MatchString(id) {
			return nil, errors.New("sessionless upload receipt objectIds must be UUIDs")
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, errors.New("sessionless upload receipt objectIds must be unique")
		}
		seen[id] = struct{}{}
	}
	if *receipt.PageCount < 0 || *receipt.TotalRows < 0 || *receipt.TotalBytes < 0 {
		return nil, errors.New("sessionless upload receipt counts must not be negative")
	}
	if *receipt.PageCount != int64(len(objectIDs)) {
		return nil, errors.New("sessionless upload receipt pageCount must equal the number of objectIds")
	}
	if len(objectIDs) == 0 {
		if includeSchema {
			return nil, errors.New("sessionless upload receipt without objects cannot satisfy a requested schema")
		}
		if *receipt.TotalRows != 0 || *receipt.TotalBytes != 0 {
			return nil, errors.New("sessionless upload receipt without objects must report zero rows and bytes")
		}
	}
	if limits != nil {
		if int64(len(objectIDs)) > int64(limits.MaxPages) {
			return nil, errors.New("sessionless upload receipt exceeds the delivery maxPages limit")
		}
		if *receipt.TotalBytes > limits.MaxResultBytes {
			return nil, errors.New("sessionless upload receipt exceeds the delivery maxResultBytes limit")
		}
	}
	return objectIDs, nil
}

// validateSessionlessFinalEvent requires the final event of a sessionless-v1 run to carry
// a valid upload_receipt, so a missing or session-shaped receipt never reaches the
// private action runner as a success.
func validateSessionlessFinalEvent(metadataJSON string, includeSchema bool, limits *RemoteQueryUploadLimits) error {
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return errors.New("sessionless final event metadata is malformed")
	}
	raw, ok := metadata["upload_receipt"]
	if !ok {
		return errors.New("sessionless final event is missing upload_receipt")
	}
	_, err := ValidateSessionlessUploadReceipt(raw, includeSchema, limits)
	return err
}

// resolveExecutionTarget resolves the execution target: one complete integration-owned candidate sweep, then the plain
// zero/one/many outcome. A unique match proceeds; zero matches answers
// target_not_found and multiple matches answer ambiguous_target before
// marshalExecuteRequest, any SQL dispatch, or any database work. There is no
// resolve-time match binding: the target is resolved fresh here, so a match whose
// check identity changed since a resolve call still executes as long as it is
// still the unique match. A sweep failure fails closed as executor_unavailable
// with the sweep's sanitized message.
func (s *RemoteQueryExecuteService) resolveExecutionTarget(internal remoteQueryExecuteRequest) (integrationCheckMatch, RemoteQueryExecuteResult) {
	resolution := resolveIntegrationTargets(s.collector, internal.Integration, internal.Target)
	switch resolution.status {
	case statusMatched:
		return resolution.matches[0], RemoteQueryExecuteResult{HTTPStatus: http.StatusOK}
	case statusTargetNotFound:
		return integrationCheckMatch{}, remoteQueryExecuteErrorResult(http.StatusNotFound, statusTargetNotFound, resolution.message)
	case statusAmbiguous:
		return integrationCheckMatch{}, remoteQueryExecuteErrorResult(http.StatusConflict, statusAmbiguous, resolution.message)
	default:
		return integrationCheckMatch{}, remoteQueryExecuteErrorResult(http.StatusBadGateway, statusExecutorUnavailable, resolution.message)
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
		TraceContext:  r.TraceContext,
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
		TraceContext:   req.TraceContext,
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
// including baseUrl — so the integration can upload page files directly; the session
// has no upload token, so the request cannot carry one. The integration reads the org
// API/application keys from Agent config, so they never appear on the request wire. The
// optional propagated trace context is revalidated here and emitted as the top-level
// traceContext object — decimal-string IDs and the integer sampling priority — only
// when a supported keep-priority context is present; absent stays absent and invalid
// — including any non-keep priority — is dropped without failing the run, so the
// integration never receives a carrier its strict validation would reject.
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
			Protocol:          req.ResultDelivery.Protocol,
			RunID:             req.ResultDelivery.RunID,
			TaskID:            req.ResultDelivery.TaskID,
			ArtifactVersion:   req.ResultDelivery.ArtifactVersion,
			UploadID:          req.ResultDelivery.UploadID,
			BaseURL:           req.ResultDelivery.BaseURL,
			ScanSensitiveData: req.ResultDelivery.ScanSensitiveData,
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
	if traceContext := normalizeRemoteQueryTraceContext(req.TraceContext); traceContext != nil {
		wireReq.TraceContext = &remoteQueryTraceContextJSON{
			TraceID:          strconv.FormatUint(traceContext.TraceID, 10),
			ParentID:         strconv.FormatUint(traceContext.SpanID, 10),
			SamplingPriority: traceContext.SamplingPriority,
		}
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
