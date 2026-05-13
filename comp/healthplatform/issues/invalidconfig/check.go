// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package invalidconfig reports datadog.yaml problems through the Agent Health Platform.
package invalidconfig

import (
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/config"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/config/lite"
	"github.com/DataDog/datadog-agent/pkg/config/schema"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// checker validates the merged in-memory config against the schema.
type checker struct {
	cfg config.Component
}

func newChecker(cfg config.Component) *checker {
	return &checker{cfg: cfg}
}

func (c *checker) Run() (*healthplatform.IssueReport, error) {
	return c.validate(), nil
}

func (c *checker) validate() *healthplatform.IssueReport {
	// AllSettingsWithoutDefaultOrSecrets returns only values the customer actually set
	raw := c.cfg.AllSettingsWithoutDefaultOrSecrets()
	if len(raw) == 0 {
		return nil
	}
	normalized, err := normalizeForSchema(raw)
	if err != nil {
		return nil
	}
	errs, schemaErr := schema.ValidateCoreConfig(normalized)
	if schemaErr != nil {
		pkglog.Warnf("[AGENTLITECONFIG] invalidconfig: schema validator unavailable; skipping check")
		return nil
	}
	if len(errs) == 0 {
		return nil
	}
	info := lite.IssueInfo{
		Kind:       lite.ErrorKindSchemaValidation,
		ConfigPath: c.cfg.ConfigFileUsed(),
		Errors:     strings.Join(errs, "\n"),
		ErrorCount: len(errs),
	}
	// Context-specific tags only; kind-based tags come from the lite template.
	var tags []string
	if env := c.cfg.GetString("env"); env != "" {
		tags = append(tags, "env:"+env)
	}
	return &healthplatform.IssueReport{
		IssueId: healthplatformdef.InvalidConfigIssueID,
		Context: info.ToContext(),
		Tags:    tags,
	}
}

// normalizeForSchema coerces a Go-native config map into JSON-native types
// via a YAML round-trip
// ScrubYaml strips any accidental secret-like values
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
