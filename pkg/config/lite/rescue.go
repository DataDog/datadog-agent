// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/pkg/config/schema"
)

// IssueID is the stable Agent Health issue identifier for any
// configuration-validation problem detected by lite mode. It MUST match the
// identifier used by the in-Fx invalidconfig issue module so the backend
// dedupes a rescue-path issue and a happy-path issue into the same record.
const IssueID = "invalid-config"

// MaxSchemaErrorsInPayload bounds the number of individual schema-validation
// error strings we ship in Extra. A pathologically broken config could
// generate hundreds; keeping the top N preserves UI readability while
// `error_count` still tells the customer the true scale.
const MaxSchemaErrorsInPayload = 20

// rescueHTTPTimeout caps the entire rescue HTTP round trip. The agent process
// is on its way out — we don't get to block here.
const rescueHTTPTimeout = 3 * time.Second

// rescueIntakePrefix mirrors the forwarder's prefix; site is appended after.
const rescueIntakePrefix = "https://agenthealth-intake."

// rescueIntakePath is the Agent Health intake endpoint path.
const rescueIntakePath = "/api/v2/agenthealth"

// rescueEventType matches the forwarder's event type so the backend treats
// both code paths identically.
const rescueEventType = "agent-health-issues"

// rescueFlavor is the service name we tag rescue payloads with. The full
// agent flavor isn't always knowable at rescue time (Fx hasn't initialised),
// so a static placeholder keeps backend dedup stable.
const rescueFlavor = "agent"

// errorKind is the discriminator we drop into Extra["error_kind"] so the
// backend / UI can render a different message per failure mode.
type errorKind string

const (
	errorKindYAMLParse        errorKind = "yaml_parse"
	errorKindSchemaValidation errorKind = "schema_validation"
	errorKindStartupFailure   errorKind = "startup_failure"
)

// Rescue is the one-shot orchestrator invoked from the agent's failure path
// (defer/recover or returned Fx error). It runs the resolver pipeline,
// schema-validates any config it could parse, and best-effort POSTs an Agent
// Health issue to the Datadog intake.
//
// Rescue never panics. On any internal failure it returns the wrapped error
// so the caller can log it, but it must not gate process exit on the result —
// the agent is already on its way down.
//
// startupErr is the original error or panic value the agent caught. It is
// included in the rescue payload when no YAML/schema problem is detected, so
// the customer still gets a signal that startup failed.
func Rescue(ctx context.Context, cliConfPath, defaultConfPath string, startupErr error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	rctx, cancel := context.WithTimeout(ctx, rescueHTTPTimeout)
	defer cancel()

	cfg := Extract(rctx, cliConfPath, defaultConfPath)

	issue := buildRescueIssue(cfg, startupErr)
	if issue == nil {
		// Nothing to report. This can happen if startupErr is nil and the
		// config parses + validates cleanly — i.e., Rescue was called by
		// mistake. Don't POST garbage.
		return nil
	}

	apiKey := cfg.APIKey.Value
	if !cfg.APIKey.resolved() {
		return fmt.Errorf("rescue: no usable api_key resolved (source=%s)", cfg.APIKey.Source)
	}

	report := &healthplatform.HealthReport{
		EventType: rescueEventType,
		EmittedAt: time.Now().UTC().Format(time.RFC3339),
		Service:   rescueFlavor,
		Host: &healthplatform.HostInfo{
			Hostname: hostnameOrEmpty(),
		},
		Issues: map[string]*healthplatform.Issue{
			"datadog.yaml": issue,
		},
	}

	return postRescue(rctx, cfg.Site.Value, apiKey, report)
}

// DefaultConfigPath returns the platform-default location of datadog.yaml.
// Callers can pass it as defaultConfPath when invoking Rescue.
func DefaultConfigPath() string {
	if p := os.Getenv("DD_CONFIG"); p != "" {
		return p
	}
	switch runtimeGOOS() {
	case "darwin":
		return "/opt/datadog-agent/etc"
	case "windows":
		return `C:\ProgramData\Datadog`
	default:
		return "/etc/datadog-agent"
	}
}

// buildRescueIssue inspects cfg + startupErr and produces the appropriate
// Issue payload. Returns nil if there is nothing worth reporting.
func buildRescueIssue(cfg LiteConfig, startupErr error) *healthplatform.Issue {
	switch {
	case cfg.YAMLParseErr != nil:
		return yamlParseIssue(cfg)
	case cfg.ParsedConfig != nil:
		errs, _ := schema.ValidateCoreConfig(cfg.ParsedConfig)
		if len(errs) > 0 {
			return schemaValidationIssue(cfg, errs)
		}
		if startupErr != nil {
			return startupFailureIssue(cfg, startupErr)
		}
		return nil
	case startupErr != nil:
		return startupFailureIssue(cfg, startupErr)
	default:
		return nil
	}
}

// yamlParseIssue builds the high-severity issue for "datadog.yaml does not
// parse as YAML at all". The customer's only path forward is to open the
// file and fix the syntax, so the remediation is concrete and short.
func yamlParseIssue(cfg LiteConfig) *healthplatform.Issue {
	path := cfg.ConfigFilePath
	if path == "" {
		path = "(no datadog.yaml found)"
	}
	parseMsg := ""
	if cfg.YAMLParseErr != nil {
		parseMsg = cfg.YAMLParseErr.Error()
	}

	extra := mustStruct(map[string]any{
		"error_kind":    string(errorKindYAMLParse),
		"config_path":   path,
		"error_message": parseMsg,
		"impact":        "The Datadog Agent cannot load its configuration and is running with defaults only. Telemetry will not be sent.",
	})

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "invalid_config",
		Title:       "Datadog Agent configuration is not valid YAML",
		Description: fmt.Sprintf("The Datadog Agent could not parse %s as YAML: %s", path, truncate(parseMsg, 400)),
		Category:    "config",
		Location:    "config",
		Severity:    "high",
		Source:      "config",
		DetectedAt:  time.Now().UTC().Format(time.RFC3339),
		Extra:       extra,
		Tags:        []string{"config", "yaml_parse"},
		Remediation: &healthplatform.Remediation{
			Summary: "Open the configuration file and fix the YAML syntax error, then restart the agent.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: fmt.Sprintf("Open %s in an editor.", path)},
				{Order: 2, Text: fmt.Sprintf("Look at the location reported by the parser: %s", truncate(parseMsg, 200))},
				{Order: 3, Text: "Fix the YAML syntax (check indentation, quoting, brackets)."},
				{Order: 4, Text: "Validate with: datadog-agent experimental check-config -c " + path},
				{Order: 5, Text: "Restart the agent: sudo systemctl restart datadog-agent (or your platform's equivalent)."},
			},
		},
	}
}

// schemaValidationIssue builds the medium-severity issue for "YAML is valid
// but does not pass the embedded JSON schema". We surface up to N specific
// errors verbatim in Extra and tell the customer the exact count.
func schemaValidationIssue(cfg LiteConfig, errs []string) *healthplatform.Issue {
	path := cfg.ConfigFilePath
	visible := errs
	if len(visible) > MaxSchemaErrorsInPayload {
		visible = visible[:MaxSchemaErrorsInPayload]
	}

	extra := mustStruct(map[string]any{
		"error_kind":  string(errorKindSchemaValidation),
		"config_path": path,
		"error_count": len(errs),
		"errors":      strings.Join(visible, "\n"),
		"truncated":   len(errs) > MaxSchemaErrorsInPayload,
		"impact":      "The Agent may apply defaults for incorrectly-typed fields and may not behave as configured.",
	})

	desc := fmt.Sprintf("Found %d schema violation(s) in %s.", len(errs), path)
	if len(visible) > 0 {
		desc += " First: " + truncate(visible[0], 240)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "invalid_config",
		Title:       fmt.Sprintf("Datadog Agent configuration has %d schema violation(s)", len(errs)),
		Description: desc,
		Category:    "config",
		Location:    "config",
		Severity:    "medium",
		Source:      "config",
		DetectedAt:  time.Now().UTC().Format(time.RFC3339),
		Extra:       extra,
		Tags:        []string{"config", "schema"},
		Remediation: &healthplatform.Remediation{
			Summary: "Fix each schema violation in the configuration file and restart the agent.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: fmt.Sprintf("Open %s in an editor.", path)},
				{Order: 2, Text: "Review the listed violations (see Extra.errors)."},
				{Order: 3, Text: "Validate after fixing: datadog-agent experimental check-config -c " + path},
				{Order: 4, Text: "Restart the agent."},
			},
		},
	}
}

// startupFailureIssue is a catch-all for "YAML and schema are fine but the
// agent still failed to start." It surfaces the original error so support /
// the customer have a starting point, even when the cause is not a config
// problem (port conflict, missing system file, permission, etc.).
func startupFailureIssue(cfg LiteConfig, startupErr error) *healthplatform.Issue {
	msg := startupErr.Error()
	extra := mustStruct(map[string]any{
		"error_kind":    string(errorKindStartupFailure),
		"config_path":   cfg.ConfigFilePath,
		"error_message": msg,
		"impact":        "The Datadog Agent process failed to start. No telemetry will be collected until the underlying problem is resolved.",
	})

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "invalid_config",
		Title:       "Datadog Agent failed to start",
		Description: fmt.Sprintf("Configuration is parseable but the agent could not complete startup: %s", truncate(msg, 400)),
		Category:    "config",
		Location:    "agent",
		Severity:    "high",
		Source:      "agent",
		DetectedAt:  time.Now().UTC().Format(time.RFC3339),
		Extra:       extra,
		Tags:        []string{"agent", "startup_failure"},
		Remediation: &healthplatform.Remediation{
			Summary: "Inspect the agent logs for the underlying cause and address it before restarting.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Check the agent log file (default /var/log/datadog/agent.log)."},
				{Order: 2, Text: "Look for the error message: " + truncate(msg, 200)},
				{Order: 3, Text: "Resolve the underlying issue (port conflicts, missing files, permissions, etc.)."},
				{Order: 4, Text: "Restart the agent."},
			},
		},
	}
}

// buildIntakeURL is the URL construction step extracted so tests can stub
// the destination without monkey-patching the rest of the function.
var buildIntakeURL = func(site string) string {
	if site == "" {
		site = DefaultSite
	}
	return rescueIntakePrefix + strings.TrimRight(site, "/") + rescueIntakePath
}

// postRescue marshals the report and POSTs it to the Agent Health intake.
// The request is bounded by rescueHTTPTimeout and uses no shared HTTP client
// state (rescue runs once per process, then exits).
func postRescue(ctx context.Context, site, apiKey string, report *healthplatform.HealthReport) error {
	if apiKey == "" {
		return errors.New("rescue: api_key is empty")
	}

	url := buildIntakeURL(site)

	payload, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal rescue payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build rescue request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)
	req.Header.Set("User-Agent", "datadog-agent-lite")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send rescue request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("rescue intake returned status %d", resp.StatusCode)
	}
	return nil
}

// mustStruct converts a map to a structpb.Struct. It never fails for our
// inputs (all values are strings, ints, or bools), but we still defend
// against a future refactor producing an unmarshalable value.
func mustStruct(m map[string]any) *structpb.Struct {
	s, err := structpb.NewStruct(m)
	if err != nil {
		// Best-effort: drop the field. The customer still gets the issue.
		return &structpb.Struct{}
	}
	return s
}

// truncate clips s to at most n bytes, appending an ellipsis when it cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// hostnameOrEmpty returns os.Hostname() but never errors. Lite mode is
// best-effort; a missing hostname becomes an empty string and the backend
// resolves it from the request envelope.
func hostnameOrEmpty() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}

// runtimeGOOS is a tiny wrapper so the platform default test can be unit-
// tested without depending on the test runner's GOOS. Production callers
// never need to override.
var runtimeGOOS = func() string { return goosBuildtime() }
