// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !jetson

// Package invalidsysprobeconfig reports system-probe.yaml schema violations through the Agent Health Platform
package invalidsysprobeconfig

import (
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/config/schema"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// checker validates the in-memory system-probe config against the embedded
// system-probe schema. It reads sysprobeconfig.Component
type checker struct {
	cfg sysprobeconfig.Component
}

func newChecker(cfg sysprobeconfig.Component) *checker {
	return &checker{cfg: cfg}
}

func (c *checker) Run() ([]runnerdef.IssueReport, error) {
	return c.validate()
}

func (c *checker) validate() ([]runnerdef.IssueReport, error) {
	if c.cfg == nil {
		// nothing to validate as the bundling command didn't include sysprobeconfig
		return nil, nil
	}
	// AllSettingsWithoutDefaultOrSecrets returns only values the customer actually set.
	raw := c.cfg.AllSettingsWithoutDefaultOrSecrets()
	if len(raw) == 0 {
		return nil, nil
	}
	normalized, err := normalizeForSchema(raw)
	if err != nil {
		return nil, fmt.Errorf("invalidsysprobeconfig: normalize config: %w", err)
	}
	errs, schemaErr := schema.ValidateSystemProbeConfig(normalized)
	if schemaErr != nil {
		pkglog.Warnf("invalidsysprobeconfig: schema validator unavailable; skipping check: %v", schemaErr)
		return nil, schemaErr
	}
	if len(errs) == 0 {
		return nil, nil
	}
	return []runnerdef.IssueReport{
		{
			IssueID:   IssueID,
			IssueName: IssueID,
			Source:    "system-probe",
			Context: map[string]string{
				contextKeyConfigPath: c.cfg.ConfigFileUsed(),
				contextKeyErrorCount: strconv.Itoa(len(errs)),
				contextKeyErrors:     strings.Join(errs, "\n"),
			},
		},
	}, nil
}

// normalizeForSchema coerces the Go-native config map into JSON-native types via
// a YAML round-trip so the JSON-schema validator sees the types it expects.
// ScrubYaml is cheap defense-in-depth against plaintext secret-like values
// (resolved ENC[] secrets are already excluded by AllSettingsWithoutDefaultOrSecrets).
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
