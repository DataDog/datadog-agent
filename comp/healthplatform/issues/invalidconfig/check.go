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
	"slices"
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
	// Validate effective customer settings, including locally resolved secrets.
	raw := c.cfg.AllSettingsWithoutDefault()
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
	violations = slices.DeleteFunc(violations, func(violation schema.Violation) bool {
		return c.isUnresolvedSecret(normalized, violation)
	})
	if len(violations) == 0 {
		return nil, nil
	}
	reports := make([]runnerdef.IssueReport, 0, len(violations))
	for _, violation := range violations {
		reports = append(reports, runnerdef.IssueReport{
			IssueID:   c.instanceIssueID(violation.Path),
			IssueName: IssueName,
			Source:    "agent",
			Context:   BuildContext(c.cfg, c.cfg.ConfigFileUsed(), violation),
		})
	}
	return reports, nil
}

// BuildContext builds issue details from scrubbed paths, types, and registered defaults.
func BuildContext(cfg model.Reader, configPath string, violation schema.Violation) map[string]string {
	path := scrubViolationPath(violation.Path)
	ctx := map[string]string{
		contextKeyConfigPath:  configPath,
		contextKeySettingPath: path,
	}
	// Raw validator messages can expose configured values or credentials in paths.
	// Send only the scrubbed path, type names, and registered default.
	if violation.ActualType == "" || len(violation.ExpectedTypes) == 0 {
		return ctx
	}
	defaultStatus, defaultValue := resolveDefault(cfg, violation.Path)
	payload := violationPayload{
		Path: path, ActualType: violation.ActualType, ExpectedTypes: violation.ExpectedTypes,
		DefaultStatus: defaultStatus, DefaultValue: defaultValue,
	}
	if encoded, err := json.Marshal(payload); err == nil {
		ctx[contextKeyViolation] = string(encoded)
	}
	return ctx
}

// Skip type errors for placeholders, but validate ENC-looking values returned by the secret backend.
func (c *checker) isUnresolvedSecret(normalized map[string]any, violation schema.Violation) bool {
	if violation.ActualType != "string" {
		return false
	}
	pointer, err := jsonpointer.Parse(violation.Path)
	if err != nil {
		return false
	}
	value, err := pointer.Eval(normalized)
	text, ok := value.(string)
	if err != nil || !ok || !scrubber.IsEnc(text) {
		return false
	}
	// A compound setting carries the source on its enclosing map or list.
	for i := len(pointer); i > 0; i-- {
		key := strings.Join(pointer[:i], ".")
		if c.cfg.IsSetting(key) {
			return c.cfg.GetSource(key) != model.SourceSecret
		}
	}
	return false
}

func scrubViolationPath(path string) string {
	pointer, err := jsonpointer.Parse(path)
	if err != nil {
		return ""
	}
	for i, token := range pointer {
		pointer[i], err = scrubber.ScrubString(token)
		if err != nil {
			return ""
		}
	}
	return pointer.String()
}

func resolveDefault(cfg model.Reader, pointerPath string) (string, any) {
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
		switch value := valueWithSource.Value.(type) {
		case nil:
			return "none", nil
		case time.Duration:
			return "known", value.String()
		default:
			return "known", value
		}
	}
	return "none", nil
}

// Keep a problem's ID stable when its value or wording changes. The DaemonSet
// discriminator groups a shared configuration; otherwise IDs are scoped to the host.
func (c *checker) instanceIssueID(settingPath string) string {
	h := fnv.New64a()
	discriminator := issues.IssueDiscriminator(c.selfIdent, c.hostname.GetSafe(context.Background()))
	fmt.Fprintf(h, "%s\x00%s\x00%s", discriminator, c.cfg.ConfigFileUsed(), settingPath)
	return fmt.Sprintf("%s:%016x", IssueID, h.Sum64())
}

// normalizeForSchema coerces a Go-native config map into JSON-native types.
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
