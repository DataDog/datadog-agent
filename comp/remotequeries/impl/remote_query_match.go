// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remotequeriesimpl implements Remote Queries POC endpoints.
package remotequeriesimpl

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.uber.org/fx"
	"gopkg.in/yaml.v3"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

const (
	// postgresIntegration is the integration whose target resolution is
	// integration-owned: its database eligibility is autodiscovery-aware and lives
	// only in the loaded Python check instance, so the Agent asks the bridge
	// resolver once per loaded check and never parses its instance config itself.
	// The name still scopes the proof-query allowlist.
	postgresIntegration = "postgres"
	// clickhouseIntegration is the one integration whose matching stays Agent-side
	// in this wave: its effective identity is fully config-derived (documented
	// defaults, no live discovery), so the Go matcher parses its instance config
	// exactly. This branch must not authorize Postgres: Postgres eligibility is
	// autodiscovery-aware and belongs to the resolver sweep.
	clickhouseIntegration = "clickhouse"

	// RemoteQueryMatchEndpointPath is mounted under /agent by the Agent command API.
	RemoteQueryMatchEndpointPath = "/remote-queries/match-check"
	// RemoteQueriesMatchEnabledConfig is disabled by default when the key is absent.
	RemoteQueriesMatchEnabledConfig = "remote_queries.match_check.enabled"

	statusOK      = "ok"
	statusMatched = "matched"
	// statusTargetNotFound, statusAmbiguous, and statusResolutionError are the
	// resolve-operation statuses of the match-before-execute contract; the
	// match-check diagnostic keeps its own statusOK vocabulary. statusResolutionError
	// reports that matching itself could not complete (internal or contract
	// error) — never a target miss. statusTargetResolutionStale is the execute-time
	// revalidation failure: the selected target no longer matches the
	// resolve-time fingerprint.
	statusTargetNotFound        = "target_not_found"
	statusAmbiguous             = "ambiguous_target"
	statusResolutionError       = "resolution_error"
	statusTargetResolutionStale = "target_resolution_stale"
	statusInvalidRequest        = "invalid_request"
	statusBridgeDisabled        = "bridge_disabled"
)

// Requires defines dependencies for the Remote Queries POC endpoint provider.
type Requires struct {
	fx.In

	Cfg       config.Component
	Collector RemoteQueryCollector
}

// NewRemoteQueryMatchEndpointProvider registers the remote query match endpoint on the internal Agent API.
func NewRemoteQueryMatchEndpointProvider(reqs Requires) api.AgentEndpointProvider {
	h := &remoteQueryMatchHandler{
		collector: reqs.Collector,
		enabled:   reqs.Cfg.GetBool(RemoteQueriesMatchEnabledConfig),
	}
	return api.NewAgentEndpointProvider(h.handle, RemoteQueryMatchEndpointPath, http.MethodPost)
}

type remoteQueryMatchHandler struct {
	collector RemoteQueryCollector
	enabled   bool
}

// remoteQueryExecutionAdmission guards one complete integration-owned candidate
// sweep: fail fast when another remote query holds admission, never queue. It is
// the same mutex execute holds through resolution and execution, so the sweep
// never races concurrent Python bridge or check-state access.
func remoteQueryExecutionAdmission() bool {
	return remoteQueryExecution.TryLock()
}

// RemoteQueryCollector is the narrow collector surface Remote Queries needs.
// The Agent command provides its collector.Component as this interface at the application boundary
// so this package does not force Bazel onboarding for the full collector component package.
type RemoteQueryCollector interface {
	GetChecks() []check.Check
}

type matchResponse struct {
	Status       string          `json:"status"`
	MatchedCount int             `json:"matched_count"`
	Match        *sanitizedMatch `json:"match,omitempty"`
	Error        *responseError  `json:"error,omitempty"`
}

type sanitizedMatch struct {
	Integration    string `json:"integration"`
	Loader         string `json:"loader"`
	ConfigProvider string `json:"config_provider"`
	MatchKind      string `json:"match_kind"`
}

// matchKindExact marks a match whose selection criterion is fully resolved at
// match time: the integration-owned resolver's admitted match, or a ClickHouse
// exact tuple or database_instance identifier match. There is no endpoint
// fallback kind anymore: endpoint reachability alone is never a match.
const matchKindExact = "exact"

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type remoteQueryMatchRequest struct {
	Integration string
	Target      remoteQueryTarget
}

type remoteQueryMatchRequestJSON struct {
	Integration string                        `json:"integration"`
	Target      *remoteQueryTargetRequestJSON `json:"target"`
}

type remoteQueryTargetRequestJSON struct {
	Host                string  `json:"host"`
	Port                *int    `json:"port"`
	DBName              string  `json:"dbname"`
	DatabaseInstance    *string `json:"database_instance"`
	hostSet             bool
	portSet             bool
	dbnameSet           bool
	databaseInstanceSet bool
}

type remoteQueryTarget struct {
	Host             string
	Port             int
	DBName           string
	DatabaseInstance string
}

type requestParseError struct {
	status  string
	message string
}

func (e requestParseError) Error() string {
	return e.message
}

func invalidRequestError(message string) error {
	return requestParseError{status: statusInvalidRequest, message: message}
}

var integrationNamePattern = regexp.MustCompile(`^[a-z0-9_]+$`)

// integrationInstanceTarget is the effective target of one loaded check instance
// for the one Agent-matched integration (clickhouse). integration records which
// config shape parsed it: only the ClickHouse shape is taught, and ClickHouse
// compares the effective database exactly.
type integrationInstanceTarget struct {
	integration      string
	host             string
	port             int
	dbname           string
	databaseInstance string
}

func (h *remoteQueryMatchHandler) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !h.enabled {
		writeMatchResponse(w, http.StatusServiceUnavailable, statusBridgeDisabled, 0, nil, "remote queries bridge is disabled")
		return
	}

	req, err := parseMatchRequest(r)
	if err != nil {
		writeMatchParseError(w, err)
		return
	}

	// The diagnostic reuses the exact resolution execute uses — never a second
	// matcher — so it sweeps the integration-owned resolver under the same
	// admission the resolve and execute paths hold, and fails fast when busy.
	if !remoteQueryExecutionAdmission() {
		writeMatchResponse(w, http.StatusServiceUnavailable, statusResolutionError, 0, nil, "another remote query is running on this Agent")
		return
	}
	// Admission covers the sweep; the answer is built from values the sweep already
	// captured, so the deferred release is panic-safe and still bounded.
	defer remoteQueryExecution.Unlock()
	resolution := resolveIntegrationTargets(h.collector, req.Integration, req.Target)
	switch resolution.status {
	case statusMatched:
		writeMatchResponse(w, http.StatusOK, statusOK, 1, &resolution.matches[0].sanitized, "")
	case statusTargetNotFound:
		writeMatchResponse(w, http.StatusNotFound, statusTargetNotFound, 0, nil, resolution.message)
	case statusAmbiguous:
		writeMatchResponse(w, http.StatusConflict, statusAmbiguous, len(resolution.matches), nil, resolution.message)
	default:
		writeMatchResponse(w, http.StatusFailedDependency, statusResolutionError, 0, nil, resolution.message)
	}
}

func parseMatchRequest(r *http.Request) (remoteQueryMatchRequest, error) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return remoteQueryMatchRequest{}, invalidRequestError("content-type must be application/json")
	}

	defer r.Body.Close()
	var wireReq remoteQueryMatchRequestJSON
	if err := decodeStrictJSON(r.Body, &wireReq); err != nil {
		return remoteQueryMatchRequest{}, parseJSONRequestError(err)
	}

	integration, err := parseIntegration(wireReq.Integration)
	if err != nil {
		return remoteQueryMatchRequest{}, err
	}
	target, err := parseTarget(wireReq.Target)
	if err != nil {
		return remoteQueryMatchRequest{}, err
	}
	return remoteQueryMatchRequest{Integration: integration, Target: target}, nil
}

func parseIntegration(integration string) (string, error) {
	integration = strings.ToLower(strings.TrimSpace(integration))
	if integration == "" {
		return "", errors.New("integration is required")
	}
	if !integrationNamePattern.MatchString(integration) {
		return "", invalidRequestError("integration contains invalid characters")
	}
	return integration, nil
}

func isJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/json"
}

var (
	errMultipleJSONValues = errors.New("multiple JSON values")
	errTargetUnknownField = errors.New("target contains unknown field")
	errTargetMustBeObject = errors.New("target must be an object")
)

func (t *remoteQueryTargetRequestJSON) UnmarshalJSON(data []byte) error {
	if !isJSONObject(data) {
		return errTargetMustBeObject
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	type targetAlias remoteQueryTargetRequestJSON
	var target targetAlias
	if err := decodeStrictJSON(bytes.NewReader(data), &target); err != nil {
		if isUnknownJSONFieldError(err) {
			return errTargetUnknownField
		}
		return err
	}
	_, target.hostSet = raw["host"]
	_, target.portSet = raw["port"]
	_, target.dbnameSet = raw["dbname"]
	_, target.databaseInstanceSet = raw["database_instance"]
	*t = remoteQueryTargetRequestJSON(target)
	return nil
}

func parseTarget(target *remoteQueryTargetRequestJSON) (remoteQueryTarget, error) {
	if target == nil {
		return remoteQueryTarget{}, errors.New("target is required")
	}

	host := normalizeHost(target.Host)

	if target.databaseInstanceSet {
		if target.DatabaseInstance == nil {
			return remoteQueryTarget{}, errors.New("target.database_instance is required")
		}
		databaseInstance := *target.DatabaseInstance
		if databaseInstance == "" {
			return remoteQueryTarget{}, errors.New("target.database_instance is required")
		}
		if strings.TrimSpace(databaseInstance) != databaseInstance {
			return remoteQueryTarget{}, errors.New("target.database_instance must not contain surrounding whitespace")
		}
		// database_instance selects one loaded check and execution stays on that
		// check's materialized configured database: host/port and dbname each name a
		// different selector mode. dbname alongside database_instance would select a
		// check identity and then override its configured database, bypassing the
		// configured-scope contract, so its presence is rejected even when the value
		// is empty or null.
		if target.hostSet || target.portSet {
			return remoteQueryTarget{}, errors.New("target must specify exactly one selector mode")
		}
		if target.dbnameSet {
			return remoteQueryTarget{}, errors.New("target.database_instance must not be combined with dbname")
		}
		return remoteQueryTarget{DatabaseInstance: databaseInstance}, nil
	}

	if host == "" {
		return remoteQueryTarget{}, errors.New("target.host is required")
	}

	port, err := parseRequiredPort(target.Port)
	if err != nil {
		return remoteQueryTarget{}, err
	}

	if target.DBName == "" {
		return remoteQueryTarget{}, errors.New("target.dbname is required")
	}

	return remoteQueryTarget{Host: host, Port: port, DBName: target.DBName}, nil
}

func parseRequiredPort(port *int) (int, error) {
	if port == nil {
		return 0, errors.New("target.port is required")
	}
	if *port < 1 || *port > 65535 {
		return 0, errors.New("target.port is out of range")
	}
	return *port, nil
}

func decodeStrictJSON(r io.Reader, value any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errMultipleJSONValues
	}
	return nil
}

func parseJSONRequestError(err error) error {
	switch {
	case errors.Is(err, errMultipleJSONValues):
		return invalidRequestError("malformed JSON request")
	case errors.Is(err, errTargetUnknownField):
		return errTargetUnknownField
	case errors.Is(err, errTargetMustBeObject):
		return errTargetMustBeObject
	case errors.Is(err, errDeliveryUnknownField):
		return errDeliveryUnknownField
	case errors.Is(err, errDeliveryMustBeObject):
		return errDeliveryMustBeObject
	case errors.Is(err, errLimitsUnknownField):
		return errLimitsUnknownField
	case errors.Is(err, errLimitsMustBeObject):
		return errLimitsMustBeObject
	case isUnknownJSONFieldError(err):
		return invalidRequestError("request contains unknown field")
	}

	var deliveryTypeErr remoteQueryDeliveryTypeError
	if errors.As(err, &deliveryTypeErr) {
		return deliveryTypeErr
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		switch typeErr.Field {
		case "port", "target.port":
			return errors.New("target.port must be an integer")
		case "database_instance", "target.database_instance":
			return errors.New("target.database_instance must be a string")
		case "target":
			return errTargetMustBeObject
		case "includeSchema":
			return errors.New("includeSchema must be a boolean")
		case "resultDelivery":
			return errDeliveryMustBeObject
		}
	}

	return invalidRequestError("malformed JSON request")
}

func isUnknownJSONFieldError(err error) bool {
	return strings.HasPrefix(err.Error(), "json: unknown field ")
}

func isJSONObject(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimSpace(data), []byte("{"))
}

func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	return strings.TrimSuffix(host, ".")
}

type integrationCheckMatch struct {
	check     check.Check
	sanitized sanitizedMatch
	// identity is the matched check's effective identity: the integration-reported
	// sanitized identity for resolver-swept integrations (postgres), or the
	// Go-parsed effective config for the explicitly Agent-matched integration
	// (clickhouse). It carries no credentials and no raw config: it feeds the match
	// fingerprint and nothing else.
	identity remoteQueryMatchIdentity
}

func normalizeIntegrationName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// remoteQueryMatchIdentity is the sanitized effective identity of one matched
// check: its endpoint, its materialized configured database, the database the
// resolver admitted for this target, and the rendered database identifier when the
// integration has one. For resolver-swept integrations the integration reports
// every field; the Go side never derives, defaults, or renders them.
type remoteQueryMatchIdentity struct {
	host             string
	port             int
	configuredDBName string
	resolvedDBName   string
	databaseInstance string
}

// remoteQueryResolution is the aggregate outcome of one integration-owned target
// resolution: per-check verdicts from the resolver sweep — or the explicitly
// integration-specific Agent-side branch — reduced to the single outcome the match
// diagnostic, resolve, and execute paths share. matches carries every reported
// MATCHED verdict: exactly one element on statusMatched, all of them on
// statusAmbiguous. A failed sweep carries no matches: it is fail-closed and never
// reports a partial answer.
type remoteQueryResolution struct {
	matches []integrationCheckMatch
	status  string
	message string
}

// matchedResolution reduces the collected matches to the zero/one/many outcome. An
// empty sweep reduces the same way: no loaded check of the integration is genuinely
// no match, never an error.
func matchedResolution(matches []integrationCheckMatch) remoteQueryResolution {
	switch len(matches) {
	case 0:
		return remoteQueryResolution{status: statusTargetNotFound, message: "no matching integration check found"}
	case 1:
		return remoteQueryResolution{matches: matches, status: statusMatched}
	default:
		return remoteQueryResolution{matches: matches, status: statusAmbiguous, message: "multiple matching integration checks found"}
	}
}

func failedResolution(message string) remoteQueryResolution {
	return remoteQueryResolution{status: statusResolutionError, message: message}
}

// resolveIntegrationTargets resolves the requested target through the
// integration-owned resolver. It asks EVERY currently loaded check of the requested
// integration — no raw-YAML prefilter, no exact/endpoint tiers — and the
// integration answers match/no-match per check with its own sanitized effective
// identity. Any inability to establish a check's eligible set fails the whole
// sweep as a resolution error, never a silent skip: a loaded check that cannot
// provide the bridge resolver is a resolution error, not a no-match. ClickHouse is
// the one explicitly sanctioned exception: its matching stays Agent-side in this
// wave because its effective identity is fully config-derived.
func resolveIntegrationTargets(collector RemoteQueryCollector, integration string, target remoteQueryTarget) remoteQueryResolution {
	var checks []check.Check
	for _, chk := range collector.GetChecks() {
		if normalizeIntegrationName(chk.String()) == integration {
			checks = append(checks, chk)
		}
	}
	if len(checks) == 0 {
		return matchedResolution(nil)
	}
	if integration == clickhouseIntegration {
		return clickHouseExactResolution(checks, integration, target)
	}
	return sweepIntegrationChecks(checks, integration, target)
}

// clickHouseExactResolution is the integration-specific Agent-side branch the pinned
// contract sanctions for ClickHouse in this wave. ClickHouse has no live database
// discovery here, so its effective server/port/db (documented defaults included)
// and its rendered identifier are fully config-derived and exact matching stays in
// the Agent. This branch must not authorize Postgres: Postgres eligibility is
// autodiscovery-aware and belongs to the bridge resolver sweep.
func clickHouseExactResolution(checks []check.Check, integration string, target remoteQueryTarget) remoteQueryResolution {
	matches := make([]integrationCheckMatch, 0, len(checks))
	for _, chk := range checks {
		instanceTarget, ok := parseIntegrationInstanceTarget(integration, chk.InstanceConfig())
		if !ok || !instanceTarget.matches(target) {
			continue
		}
		// Exact matching means the effective database is both the configured and the
		// admitted database; the identifier renders exactly as the check emits it.
		matches = append(matches, newIntegrationCheckMatch(chk, integration, matchKindExact, remoteQueryMatchIdentity{
			host:             instanceTarget.host,
			port:             instanceTarget.port,
			configuredDBName: instanceTarget.dbname,
			resolvedDBName:   instanceTarget.dbname,
			databaseInstance: instanceTarget.databaseInstance,
		}))
	}
	return matchedResolution(matches)
}

// sweepIntegrationChecks asks every loaded check through the remote-query bridge:
// one resolve_target request per check instance, classified against the pinned
// verdict contract, aggregated zero/one/many. Any per-check failure fails the whole
// sweep as a resolution error — even if another check matched.
func sweepIntegrationChecks(checks []check.Check, integration string, target remoteQueryTarget) remoteQueryResolution {
	matches := make([]integrationCheckMatch, 0, len(checks))
	for _, chk := range checks {
		runner, ok := remoteQueryStreamRunnerFor(chk)
		if !ok {
			return failedResolution("loaded integration check does not support remote query resolution")
		}
		verdict, err := askIntegrationResolver(runner, integration, target)
		if err != nil {
			return failedResolution(err.Error())
		}
		if !verdict.matched {
			continue
		}
		matches = append(matches, newIntegrationCheckMatch(chk, integration, matchKindExact, verdict.identity))
	}
	return matchedResolution(matches)
}

// newIntegrationCheckMatch builds one sanitized match entry. The identity carries
// no credentials or raw config: it feeds the match fingerprint and nothing else.
func newIntegrationCheckMatch(chk check.Check, integration string, matchKind string, identity remoteQueryMatchIdentity) integrationCheckMatch {
	return integrationCheckMatch{
		check: chk,
		sanitized: sanitizedMatch{
			Integration:    integration,
			Loader:         chk.Loader(),
			ConfigProvider: chk.ConfigProvider(),
			MatchKind:      matchKind,
		},
		identity: identity,
	}
}

// remoteQueryResolveVerdict is one check's answer to a resolve_target request.
type remoteQueryResolveVerdict struct {
	matched  bool
	identity remoteQueryMatchIdentity
}

// RemoteQueryOperationResolveTarget is the side-effect-free per-check resolution
// operation of the resolve-over-bridge contract. The request carries only the
// operation and the target — no query, no result delivery, no fingerprint — and the
// integration answers whether the requested target belongs to this check's
// effective monitoring scope, reporting its sanitized effective identity when it
// does. The Python entry point dispatches on this operation; the bridge transport
// is unchanged.
const RemoteQueryOperationResolveTarget = "resolve_target"

// remoteQueryResolveTargetRequestJSON is the bridge wire shape of the resolution
// request: the operation and the target only, under the generic target keys.
type remoteQueryResolveTargetRequestJSON struct {
	Operation string                `json:"operation"`
	Target    remoteQueryTargetJSON `json:"target"`
}

func marshalResolveTargetRequest(target remoteQueryTarget) (string, error) {
	requestJSON, err := json.Marshal(remoteQueryResolveTargetRequestJSON{
		Operation: RemoteQueryOperationResolveTarget,
		Target:    remoteQueryTargetJSON{Host: target.Host, Port: target.Port, DBName: target.DBName, DatabaseInstance: target.DatabaseInstance},
	})
	if err != nil {
		return "", err
	}
	return string(requestJSON), nil
}

// askIntegrationResolver performs one bridge resolve call against one loaded check
// and classifies its verdict. The pinned verdict is exactly one event: a final
// MATCHED with the sanitized match identity, or an existing-style error with
// error.code target_not_found. A second event fails the emit callback so the
// bridge call itself fails instead of buffering an unbounded stream.
func askIntegrationResolver(runner remoteQueryStreamRunner, integration string, target remoteQueryTarget) (remoteQueryResolveVerdict, error) {
	requestJSON, err := marshalResolveTargetRequest(target)
	if err != nil {
		return remoteQueryResolveVerdict{}, errors.New("could not encode the remote query resolve request")
	}
	events := make([]check.RemoteQueryStreamEvent, 0, 1)
	if err := runner.RunRemoteQueryStream(integration, requestJSON, func(event check.RemoteQueryStreamEvent) error {
		if len(events) > 0 {
			return errors.New("remote query resolver emitted more than one verdict event")
		}
		events = append(events, event)
		return nil
	}); err != nil {
		return remoteQueryResolveVerdict{}, errors.New("remote query resolver bridge call failed")
	}
	return classifyResolveVerdict(events, target)
}

var errInvalidResolveVerdict = errors.New("remote query resolver returned an invalid verdict")

// remoteQueryResolveFinalJSON is the strict wire shape of the matched verdict's
// final event: exactly the status and the sanitized match identity.
type remoteQueryResolveFinalJSON struct {
	Status string          `json:"status"`
	Match  json.RawMessage `json:"match"`
}

// remoteQueryResolveMatchJSON is the integration-reported sanitized match identity.
// Pointer fields distinguish a genuinely absent field (nil, omitted from the
// fingerprint) from a present-but-empty or present-but-invalid one, which fails
// closed.
type remoteQueryResolveMatchJSON struct {
	Host             *string `json:"host,omitempty"`
	Port             *int    `json:"port,omitempty"`
	ConfiguredDBName *string `json:"configuredDbname,omitempty"`
	ResolvedDBName   *string `json:"resolvedDbname,omitempty"`
	DatabaseInstance *string `json:"databaseInstance,omitempty"`
}

// remoteQueryResolveErrorJSON is the existing-style error verdict: an error object
// with a code, tolerating the envelope fields failed_event carries.
type remoteQueryResolveErrorJSON struct {
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Code string `json:"code"`
}

func classifyResolveVerdict(events []check.RemoteQueryStreamEvent, target remoteQueryTarget) (remoteQueryResolveVerdict, error) {
	if len(events) != 1 {
		return remoteQueryResolveVerdict{}, errInvalidResolveVerdict
	}
	switch events[0].Type {
	case "final":
		return parseMatchedResolveVerdict(events[0].MetadataJSON, target)
	case "error":
		return parseUnmatchedResolveVerdict(events[0].MetadataJSON)
	default:
		return remoteQueryResolveVerdict{}, errInvalidResolveVerdict
	}
}

func parseMatchedResolveVerdict(metadataJSON string, target remoteQueryTarget) (remoteQueryResolveVerdict, error) {
	var final remoteQueryResolveFinalJSON
	if err := decodeStrictJSON(strings.NewReader(metadataJSON), &final); err != nil {
		return remoteQueryResolveVerdict{}, errInvalidResolveVerdict
	}
	if final.Status != "MATCHED" || len(final.Match) == 0 {
		return remoteQueryResolveVerdict{}, errInvalidResolveVerdict
	}
	var matchJSON remoteQueryResolveMatchJSON
	if err := decodeStrictJSON(strings.NewReader(string(final.Match)), &matchJSON); err != nil {
		return remoteQueryResolveVerdict{}, errInvalidResolveVerdict
	}
	identity, err := validateResolveMatchIdentity(&matchJSON, target)
	if err != nil {
		return remoteQueryResolveVerdict{}, err
	}
	return remoteQueryResolveVerdict{matched: true, identity: identity}, nil
}

func parseUnmatchedResolveVerdict(metadataJSON string) (remoteQueryResolveVerdict, error) {
	var wire remoteQueryResolveErrorJSON
	if err := json.Unmarshal([]byte(metadataJSON), &wire); err != nil {
		return remoteQueryResolveVerdict{}, errInvalidResolveVerdict
	}
	code := wire.Code
	if wire.Error != nil {
		code = wire.Error.Code
	}
	if code == statusTargetNotFound {
		return remoteQueryResolveVerdict{}, nil
	}
	// Any other verdict — an invalid request, or an inability to establish this
	// check's eligible set — fails the aggregate resolution; it is never a silent
	// no-match.
	return remoteQueryResolveVerdict{}, errors.New("remote query resolution failed on a loaded integration check")
}

// validateResolveMatchIdentity enforces the pinned verdict contract: fields present
// must be non-empty and valid, and the fields the selector and the fingerprint need
// are required. A tuple target must identify its endpoint and the database the
// resolver admitted; a database_instance target must identify the rendered
// identifier it matched on and the materialized database execution uses.
func validateResolveMatchIdentity(matchJSON *remoteQueryResolveMatchJSON, target remoteQueryTarget) (remoteQueryMatchIdentity, error) {
	if matchJSON == nil {
		return remoteQueryMatchIdentity{}, errInvalidResolveVerdict
	}
	identity := remoteQueryMatchIdentity{}
	if matchJSON.Host != nil {
		if *matchJSON.Host == "" {
			return remoteQueryMatchIdentity{}, errInvalidResolveVerdict
		}
		identity.host = *matchJSON.Host
	}
	if matchJSON.Port != nil {
		if *matchJSON.Port < 1 || *matchJSON.Port > 65535 {
			return remoteQueryMatchIdentity{}, errInvalidResolveVerdict
		}
		identity.port = *matchJSON.Port
	}
	if matchJSON.ConfiguredDBName != nil {
		if *matchJSON.ConfiguredDBName == "" {
			return remoteQueryMatchIdentity{}, errInvalidResolveVerdict
		}
		identity.configuredDBName = *matchJSON.ConfiguredDBName
	}
	if matchJSON.ResolvedDBName != nil {
		if *matchJSON.ResolvedDBName == "" {
			return remoteQueryMatchIdentity{}, errInvalidResolveVerdict
		}
		identity.resolvedDBName = *matchJSON.ResolvedDBName
	}
	if matchJSON.DatabaseInstance != nil {
		if *matchJSON.DatabaseInstance == "" {
			return remoteQueryMatchIdentity{}, errInvalidResolveVerdict
		}
		identity.databaseInstance = *matchJSON.DatabaseInstance
	}
	if target.DatabaseInstance != "" {
		if identity.databaseInstance == "" || identity.resolvedDBName == "" {
			return remoteQueryMatchIdentity{}, errInvalidResolveVerdict
		}
	} else if identity.host == "" || identity.port == 0 || identity.resolvedDBName == "" {
		return remoteQueryMatchIdentity{}, errInvalidResolveVerdict
	}
	return identity, nil
}

// matches is the exact candidate predicate for the one Agent-matched integration
// (clickhouse). database_instance targets match on exact rendered identifier
// equality; tuple targets must name the database the instance actually monitors, so
// the effective database is compared exactly. No Postgres case exists here:
// Postgres matching is integration-owned.
func (t integrationInstanceTarget) matches(target remoteQueryTarget) bool {
	if target.DatabaseInstance != "" {
		return t.databaseInstance != "" && t.databaseInstance == target.DatabaseInstance
	}
	if t.integration != clickhouseIntegration {
		return false
	}
	return t.host == target.Host && t.port == target.Port && t.dbname == target.DBName
}

// parseIntegrationInstanceTarget parses the effective target of one loaded check
// for the integrations whose matching stays Agent-side in this wave. ClickHouse
// exact matching is config-derived — documented defaults, no live discovery — so
// the Agent parses its effective server/port/db and renders its identifier.
// Postgres is absent deliberately: its autodiscovery-aware eligibility belongs to
// the integration-owned resolver sweep, and this parser must not become a Go
// reimplementation of it. Any other integration fails closed.
func parseIntegrationInstanceTarget(integration string, instanceConfig string) (integrationInstanceTarget, bool) {
	var instanceTarget integrationInstanceTarget
	var ok bool
	switch integration {
	case clickhouseIntegration:
		instanceTarget, ok = parseClickHouseInstanceTarget(instanceConfig)
	default:
		return integrationInstanceTarget{}, false
	}
	if !ok {
		return integrationInstanceTarget{}, false
	}
	instanceTarget.integration = integration
	return instanceTarget, true
}

// The ClickHouse check's documented instance defaults: the HTTP interface port and
// the default database. The Agent applies them exactly like the check does when the
// config keys are absent, so instance matching sees the effective endpoint the check
// monitors.
const (
	clickhouseDefaultPort = 8123
	clickhouseDefaultDB   = "default"
)

// parseClickHouseInstanceTarget parses a ClickHouse check instance config. The wire
// tuple {host, port, dbname} is the normalized alias of the ClickHouse config triple
// {server, port, db}: wire host aliases config server and wire dbname aliases config
// db. The wire dbname stays required and is compared exactly against the effective
// database — an explicit db, or the check's `default` when the key is absent — so an
// instance only matches a target that names the database it actually monitors. Keys
// that are present but invalid fail closed; the deprecated config alias `host` is not
// accepted for server (the parser reads the canonical field only).
func parseClickHouseInstanceTarget(instanceConfig string) (integrationInstanceTarget, bool) {
	var fields map[string]any
	if err := yaml.Unmarshal([]byte(instanceConfig), &fields); err != nil || fields == nil {
		return integrationInstanceTarget{}, false
	}

	rawServer, ok := fields["server"].(string)
	if !ok {
		return integrationInstanceTarget{}, false
	}
	server := normalizeHost(rawServer)
	if server == "" {
		return integrationInstanceTarget{}, false
	}

	port := clickhouseDefaultPort
	if rawPort, present := fields["port"]; present {
		parsedPort, ok := yamlInt(rawPort)
		if !ok || parsedPort < 1 || parsedPort > 65535 {
			return integrationInstanceTarget{}, false
		}
		port = parsedPort
	}

	db := clickhouseDefaultDB
	if rawDB, present := fields["db"]; present {
		parsedDB, ok := rawDB.(string)
		if !ok || parsedDB == "" {
			return integrationInstanceTarget{}, false
		}
		db = parsedDB
	}

	// The identifier renders from the raw server string because the check templates
	// str(config.server) without normalization; the normalized server is used for
	// tuple matching against the wire host only.
	databaseInstance, _ := renderClickHouseDatabaseIdentifier(fields, rawServer, port, db)

	return integrationInstanceTarget{host: server, port: port, dbname: db, databaseInstance: databaseInstance}, true
}

// clickhouseDatabaseIdentifierDefaultTemplate mirrors the ClickHouse check's default
// database_identifier template.
const clickhouseDatabaseIdentifierDefaultTemplate = "$server:$port:$db"

// renderClickHouseDatabaseIdentifier renders the database_instance identifier the
// ClickHouse check emits. Unlike Postgres, the check's default template
// $server:$port:$db is fully config-derived — server, port, and db are effective config
// values, none resolved at runtime — so the Agent can render it faithfully instead of
// failing closed like the Postgres default.
func renderClickHouseDatabaseIdentifier(fields map[string]any, server string, port int, db string) (string, bool) {
	template := clickhouseDatabaseIdentifierDefaultTemplate
	if rawIdentifier, ok := fields["database_identifier"]; ok {
		identifier, ok := rawIdentifier.(map[string]any)
		if !ok {
			return "", false
		}
		rawTemplate, ok := identifier["template"]
		if !ok {
			return "", false
		}
		parsedTemplate, ok := rawTemplate.(string)
		if !ok || parsedTemplate == "" {
			return "", false
		}
		template = parsedTemplate
	}

	values := clickHouseDatabaseIdentifierTemplateValues(fields, server, port, db)
	rendered, ok := renderPythonTemplate(template, values)
	if !ok || rendered == "" {
		return "", false
	}
	return rendered, true
}

func clickHouseDatabaseIdentifierTemplateValues(fields map[string]any, server string, port int, db string) map[string]string {
	values := tagTemplateValues(fields)
	values["server"] = server
	values["port"] = strconv.Itoa(port)
	values["db"] = db
	return values
}

// tagTemplateValues exposes each `key:value` tag as a template variable, matching the
// check-side identifier templating: duplicate keys join with commas after sorting the
// tags for a stable ordering.
func tagTemplateValues(fields map[string]any) map[string]string {
	values := make(map[string]string)
	if tags, ok := fields["tags"].([]any); ok {
		stringTags := make([]string, 0, len(tags))
		for _, rawTag := range tags {
			tag, ok := rawTag.(string)
			if !ok {
				continue
			}
			stringTags = append(stringTags, tag)
		}
		sort.Strings(stringTags)
		for _, tag := range stringTags {
			key, value, ok := strings.Cut(tag, ":")
			if !ok || key == "" {
				continue
			}
			if existing, found := values[key]; found {
				values[key] = existing + "," + value
			} else {
				values[key] = value
			}
		}
	}
	return values
}

func renderPythonTemplate(template string, values map[string]string) (string, bool) {
	var out strings.Builder
	for i := 0; i < len(template); {
		if template[i] != '$' {
			out.WriteByte(template[i])
			i++
			continue
		}
		if i+1 < len(template) && template[i+1] == '$' {
			out.WriteByte('$')
			i += 2
			continue
		}
		name := ""
		next := i + 1
		if next < len(template) && template[next] == '{' {
			end := next + 1
			for end < len(template) && template[end] != '}' {
				end++
			}
			if end >= len(template) || end == next+1 {
				return "", false
			}
			name = template[next+1 : end]
			next = end + 1
		} else {
			end := next
			if end >= len(template) || !isTemplateIdentifierStart(template[end]) {
				return "", false
			}
			end++
			for end < len(template) && isTemplateIdentifierChar(template[end]) {
				end++
			}
			name = template[next:end]
			next = end
		}
		value, ok := values[name]
		if !ok {
			return "", false
		}
		out.WriteString(value)
		i = next
	}
	return out.String(), true
}

func isTemplateIdentifierStart(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '_'
}

func isTemplateIdentifierChar(b byte) bool {
	return isTemplateIdentifierStart(b) || (b >= '0' && b <= '9')
}

func yamlInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case uint64:
		if v > uint64(^uint(0)>>1) {
			return 0, false
		}
		return int(v), true
	default:
		return 0, false
	}
}

func writeMatchParseError(w http.ResponseWriter, err error) {
	parseErr, ok := err.(requestParseError)
	if !ok {
		writeMatchResponse(w, http.StatusBadRequest, statusInvalidRequest, 0, nil, err.Error())
		return
	}

	writeMatchResponse(w, http.StatusBadRequest, parseErr.status, 0, nil, parseErr.message)
}

func writeMatchResponse(w http.ResponseWriter, httpStatus int, status string, matchedCount int, match *sanitizedMatch, message string) {
	w.WriteHeader(httpStatus)
	resp := matchResponse{
		Status:       status,
		MatchedCount: matchedCount,
		Match:        match,
	}
	if status != statusOK {
		resp.Error = &responseError{Code: status, Message: message}
	}
	_ = json.NewEncoder(w).Encode(resp)
}
