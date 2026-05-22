// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !jetson

// Package invalidconfig reports datadog.yaml schema violations through the Agent Health Platform.
// Excluded from the IoT Agent build to stay under the binary size budget.
package invalidconfig

import (
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/config"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
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

func (c *checker) Run() ([]storedef.IssueReport, error) {
	return c.validate()
}

func (c *checker) validate() ([]storedef.IssueReport, error) {
	// AllSettingsWithoutDefaultOrSecrets returns only values the customer actually set
	raw := c.cfg.AllSettingsWithoutDefaultOrSecrets()
	if len(raw) == 0 {
		return nil, nil
	}
	normalized, err := normalizeForSchema(raw)
	if err != nil {
		return nil, fmt.Errorf("invalidconfig: normalize config: %w", err)
	}
	errs, schemaErr := schema.ValidateCoreConfig(normalized)
	if schemaErr != nil {
		pkglog.Warnf("invalidconfig: schema validator unavailable; skipping check: %v", schemaErr)
		return nil, schemaErr
	}
	if len(errs) == 0 {
		return nil, nil
	}
	return []storedef.IssueReport{
		{
			IssueID:   IssueID,
			IssueType: IssueID,
			Source:    "agent",
			Context: map[string]string{
				contextKeyConfigPath: c.cfg.ConfigFileUsed(),
				contextKeyErrorCount: strconv.Itoa(len(errs)),
				contextKeyErrors:     strings.Join(errs, "\n"),
			},
		},
	}, nil
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
