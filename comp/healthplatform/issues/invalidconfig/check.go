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
	Rule          string   `json:"rule"`
	ActualType    string   `json:"actual_type"`
	ExpectedTypes []string `json:"expected_types"`
	Required      bool     `json:"required"`
	DefaultStatus string   `json:"default_status"`
	DefaultValue  string   `json:"default_value,omitempty"`
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
	for i, violation := range violations {
		ctx[contextErrorKey(i)] = violation.Message
	}
	payloads, complete := buildViolationPayloads(cfg, violations)
	if !complete {
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

func buildViolationPayloads(cfg config.Component, violations []schema.Violation) ([]violationPayload, bool) {
	if len(violations) == 0 {
		return nil, false
	}

	payloads := make([]violationPayload, 0, len(violations))
	for _, violation := range violations {
		if violation.Rule != "type" || !supportedActualType(violation.ActualType) || !supportedExpectedTypes(violation.ExpectedTypes) {
			return nil, false
		}
		defaultStatus, defaultValue := resolveDefault(cfg, violation.Path)
		payloads = append(payloads, violationPayload{
			Path:          violation.Path,
			Rule:          violation.Rule,
			ActualType:    violation.ActualType,
			ExpectedTypes: violation.ExpectedTypes,
			Required:      violation.Required,
			DefaultStatus: defaultStatus,
			DefaultValue:  defaultValue,
		})
	}
	return payloads, true
}

func supportedActualType(value string) bool {
	switch value {
	case "null", "boolean", "integer", "number", "string", "array", "object", "unknown":
		return true
	default:
		return false
	}
}

func supportedExpectedTypes(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if value == "unknown" || !supportedActualType(value) {
			return false
		}
	}
	return true
}

func resolveDefault(cfg config.Component, pointer string) (string, string) {
	key, ok := configKeyFromJSONPointer(pointer)
	if !ok || !isKnownSetting(cfg, key) {
		return "unknown", ""
	}

	for _, valueWithSource := range cfg.GetAllSources(key) {
		if valueWithSource.Source != model.SourceDefault {
			continue
		}
		if valueWithSource.Value == nil {
			return "none", ""
		}
		if duration, ok := valueWithSource.Value.(time.Duration); ok {
			encoded, err := json.Marshal(duration.String())
			if err != nil {
				return "unknown", ""
			}
			return "known", string(encoded)
		}
		encoded, err := json.Marshal(valueWithSource.Value)
		if err != nil {
			return "unknown", ""
		}
		return "known", string(encoded)
	}
	return "none", ""
}

func isKnownSetting(cfg config.Component, key string) bool {
	if !cfg.IsKnown(key) {
		return false
	}
	for _, candidate := range cfg.AllKeysLowercased() {
		if candidate == strings.ToLower(key) {
			return true
		}
	}
	return false
}

func configKeyFromJSONPointer(pointer string) (string, bool) {
	if pointer == "" || !strings.HasPrefix(pointer, "/") {
		return "", false
	}

	tokens := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for index, token := range tokens {
		decoded, ok := decodeJSONPointerToken(token)
		if !ok || decoded == "" || strings.Contains(decoded, ".") || isArrayIndex(decoded) {
			return "", false
		}
		tokens[index] = decoded
	}
	return strings.Join(tokens, "."), true
}

func decodeJSONPointerToken(token string) (string, bool) {
	var decoded strings.Builder
	for index := 0; index < len(token); index++ {
		if token[index] != '~' {
			decoded.WriteByte(token[index])
			continue
		}
		if index+1 == len(token) {
			return "", false
		}
		index++
		switch token[index] {
		case '0':
			decoded.WriteByte('~')
		case '1':
			decoded.WriteByte('/')
		default:
			return "", false
		}
	}
	return decoded.String(), true
}

func isArrayIndex(token string) bool {
	if token == "-" || token == "0" {
		return true
	}
	if len(token) == 0 || token[0] == '0' {
		return false
	}
	for _, character := range token {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
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

// normalizeForSchema coerces a Go-native config map into JSON-native types via
// a YAML round-trip. ScrubYaml strips any accidental secret-like values
func normalizeForSchema(in map[string]any) (map[string]any, error) {
	b, err := yaml.Marshal(in)
	if err != nil {
		return nil, err
	}
	scrubbed, err := scrubber.ScrubYaml(b)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := yaml.Unmarshal(scrubbed, &out); err != nil {
		return nil, err
	}
	return out, nil
}
