// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package invalidconfig reports datadog.yaml schema violations through the Agent Health Platform.
package invalidconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"github.com/qri-io/jsonpointer"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/schema"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

type violationPayload struct {
	Path          string   `json:"path"`
	ActualType    string   `json:"actual_type"`
	ExpectedTypes []string `json:"expected_types"`
	DefaultStatus string   `json:"default_status"`
	DefaultValue  any      `json:"default_value,omitempty"`
}

// checker validates the merged in-memory config against the schema.
type checker struct {
	cfg       config.Component
	hostname  hostnameinterface.Component
	selfIdent *selfident.SelfIdent
}

func newChecker(cfg config.Component, hostname hostnameinterface.Component, selfIdent *selfident.SelfIdent) *checker {
	return &checker{cfg: cfg, hostname: hostname, selfIdent: selfIdent}
}

func (c *checker) Run() ([]runnerdef.IssueReport, error) {
	return c.validate()
}

func (c *checker) validate() ([]runnerdef.IssueReport, error) {
	// AllSettingsWithoutDefaultOrSecrets returns only values the customer actually set
	raw := c.cfg.AllSettingsWithoutDefaultOrSecrets()
	if len(raw) == 0 {
		return nil, nil
	}
	normalized, err := normalizeForSchema(raw)
	if err != nil {
		return nil, fmt.Errorf("invalidconfig: normalize config: %w", err)
	}
	violations, schemaErr := schema.ValidateCoreConfigDetailed(normalized)
	if schemaErr != nil {
		pkglog.Warnf("invalidconfig: schema validator unavailable; skipping check: %v", schemaErr)
		return nil, schemaErr
	}
	retained := violations[:0]
	for _, violation := range violations {
		if !isUnresolvedSecret(normalized, violation) {
			retained = append(retained, violation)
		}
	}
	violations = retained
	if len(violations) == 0 {
		return nil, nil
	}
	return []runnerdef.IssueReport{
		{
			IssueID:   c.instanceIssueID(),
			IssueName: IssueName,
			Source:    "agent",
			Context:   buildIssueReportContext(c.cfg, c.cfg.ConfigFileUsed(), violations),
		},
	}, nil
}

func buildIssueReportContext(cfg config.Component, configPath string, violations []schema.Violation) map[string]string {
	ctx := map[string]string{
		contextKeyConfigPath: configPath,
		contextKeyErrorCount: strconv.Itoa(len(violations)),
	}
	payloads := make([]violationPayload, 0, len(violations))
	for i, violation := range violations {
		path := scrubViolationPath(violation.Path)
		// Never forward raw schema messages: non-type errors can quote values.
		ctx[contextErrorKey(i)] = fmt.Sprintf("at '%s': configuration does not match schema", path)
		if violation.ActualType == "" || len(violation.ExpectedTypes) == 0 {
			continue
		}
		ctx[contextErrorKey(i)] = fmt.Sprintf("at '%s': got %s, want %s", path, violation.ActualType, strings.Join(violation.ExpectedTypes, " or "))
		defaultStatus, defaultValue := resolveDefault(cfg, violation.Path)
		payloads = append(payloads, violationPayload{
			Path:          path,
			ActualType:    violation.ActualType,
			ExpectedTypes: violation.ExpectedTypes,
			DefaultStatus: defaultStatus,
			DefaultValue:  defaultValue,
		})
	}
	if len(payloads) != len(violations) {
		return ctx
	}
	encoded, err := json.Marshal(payloads)
	if err != nil {
		return ctx
	}
	ctx[contextKeyViolationsVersion] = "1"
	ctx[contextKeyViolations] = string(encoded)
	return ctx
}

func scrubViolationPath(path string) string {
	pointer, err := jsonpointer.Parse(path)
	if err != nil {
		return ""
	}
	// Map keys can be URLs with credentials. Scrub before JSON-pointer escaping
	// turns "://" into ":~1~1", which the existing URL scrubber cannot recognize.
	for i, token := range pointer {
		pointer[i], err = scrubber.ScrubString(token)
		if err != nil {
			return ""
		}
	}
	return pointer.String()
}

func isUnresolvedSecret(normalized map[string]any, violation schema.Violation) bool {
	pointer, err := jsonpointer.Parse(violation.Path)
	if err != nil {
		return false
	}
	value, err := pointer.Eval(normalized)
	if err != nil {
		return false
	}
	text, ok := value.(string)
	return ok && scrubber.IsEnc(text) && allExpectedTypesScalar(violation.ExpectedTypes)
}

// The secrets layer is excluded from this check, so we cannot validate resolved
// scalar values. A placeholder cannot supply an array or object.
func allExpectedTypesScalar(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if value == "array" || value == "object" || value == "null" {
			return false
		}
	}
	return true
}

func resolveDefault(cfg config.Component, pointerPath string) (string, any) {
	pointer, err := jsonpointer.Parse(pointerPath)
	if err != nil || pointer.IsEmpty() {
		return "unknown", nil
	}
	for _, token := range pointer {
		if token == "" || strings.Contains(token, ".") {
			return "unknown", nil
		}
	}
	key := strings.Join(pointer, ".")
	if !cfg.IsSetting(key) {
		return "unknown", nil
	}

	for _, valueWithSource := range cfg.GetAllSources(key) {
		if valueWithSource.Source != model.SourceDefault {
			continue
		}
		if valueWithSource.Value == nil {
			return "none", nil
		}
		if duration, ok := valueWithSource.Value.(time.Duration); ok {
			return "known", duration.String()
		}
		return "known", valueWithSource.Value
	}
	return "none", nil
}

// instanceIssueID scopes IssueID to this agent's discriminator and config
// file. Without this, two hosts in the same org validating the same config
// file (or, on one host, the agent and cluster-agent validating their own
// distinct config files) would all report the bare IssueID: downstream
// aggregation keys recommendations on (org, IssueID) alone and would collapse
// them into a single case.
//
// The discriminator is this agent's owning DaemonSet uid when resolvable
// (issues.IssueDiscriminator), so that a config file distributed by that
// DaemonSet to every node agent collapses into one case instead of one per
// host — a deliberate inversion of the default per-host scoping, since the
// underlying cause and fix are shared across the whole DaemonSet. It falls
// back to the hostname on non-Kubernetes agents, preserving today's per-host
// behavior there.
//
// Uses a 64-bit digest rather than 32-bit: at 32 bits, an org with ~10k
// distinct discriminator/config-path pairs would already have a ~1% chance
// of two of them colliding (birthday bound), silently recreating the exact
// aggregation bug this ID scoping exists to fix. At 64 bits that probability
// is ~2.7e-12 at the same fleet size — negligible at any realistic scale.
func (c *checker) instanceIssueID() string {
	h := fnv.New64a()
	discriminator := issues.IssueDiscriminator(c.selfIdent, c.hostname.GetSafe(context.Background()))
	fmt.Fprintf(h, "%s\x00%s", discriminator, c.cfg.ConfigFileUsed())
	return fmt.Sprintf("%s:%016x", IssueID, h.Sum64())
}

// normalizeForSchema coerces a Go-native config map into JSON-native types.
// Values stay local; only value-free diagnostics are included in the issue.
func normalizeForSchema(in map[string]any) (map[string]any, error) {
	b, err := yaml.Marshal(in)
	if err != nil {
		return nil, err
	}
	var normalized map[string]any
	if err := yaml.Unmarshal(b, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}
