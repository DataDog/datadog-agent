// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package testprogs

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
)

type probeYaml struct {
	Binary string           `yaml:"binary"`
	Probes []map[string]any `yaml:"probes"`
}

// MustGetProbeDefinitions calls GetProbeDefinitions and checks for an error.
func MustGetProbeDefinitions(t testing.TB, name string) []ir.ProbeDefinition {
	probes, err := GetProbeDefinitions(name)
	require.NoError(t, err)
	return probes
}

// GetProbeDefinitions returns the probe definitions for binary of a given name.
func GetProbeDefinitions(name string) ([]ir.ProbeDefinition, error) {
	probes, err := getProbeDefinitions(name)
	if err != nil {
		return nil, fmt.Errorf("get probe definitions for %s: %w", name, err)
	}
	return probes, nil
}

func getProbeDefinitions(name string) ([]ir.ProbeDefinition, error) {
	state, err := getState()
	if err != nil {
		return nil, err
	}
	yamlData, err := os.ReadFile(path.Join(state.probesCfgsDir, name+".yaml"))
	if err != nil {
		return nil, err
	}
	var probeYaml probeYaml
	err = yaml.Unmarshal(yamlData, &probeYaml)
	if err != nil {
		return nil, err
	}
	var probes []ir.ProbeDefinition
	for _, probe := range probeYaml.Probes {
		probeBytes, err := json.Marshal(probe)
		if err != nil {
			return nil, err
		}
		probe, err := rcjson.UnmarshalProbe(probeBytes)
		if err != nil {
			return nil, err
		}
		if err := rcjson.Validate(probe); err != nil {
			return nil, fmt.Errorf("validate probe %s: %w", probe.GetID(), err)
		}
		probes = append(probes, probe)
	}
	return probes, nil
}

// IssueTagPrefix is the prefix of the issue tag.
const IssueTagPrefix = "issue:"

// SkipIntegrationTagPrefix is the prefix for skipping probes in integration tests.
// The value after the prefix is a Config string (arch=ARCH,toolchain=VERSION)
// parsed by parseConfig. Matching is exact on both fields.
const SkipIntegrationTagPrefix = "skip_integration:"

// GetIssueTag returns the issue tag for a probe definition that matches
// the given config. An issue tag has the form "issue:KIND" (matches all
// configs) or "issue:KIND@arch=X,toolchain=Y" (matches only that config).
// A probe may have multiple issue tags for different configs.
//
// Returns the issue kind, whether a match was found, and whether the
// matching tag was config-conditional (had an @suffix). When conditional
// is false and ok is true, the issue applies to ALL configs.
func GetIssueTag(p ir.ProbeDefinition, cfg Config) (kind string, ok bool, conditional bool) {
	for _, tag := range p.GetTags() {
		if !strings.HasPrefix(tag, IssueTagPrefix) {
			continue
		}
		value := tag[len(IssueTagPrefix):]
		k, cfgStr, hasAt := strings.Cut(value, "@")
		if !hasAt {
			// Unconditional: applies to all configs.
			return k, true, false
		}
		// Config-specific: only match if the config matches exactly.
		issueCfg, err := ParseConfig(cfgStr)
		if err != nil {
			continue // malformed tag, skip
		}
		if issueCfg == cfg {
			return k, true, true
		}
	}
	return "", false, false
}

// HasIssueTag returns true if the probe definition has an issue tag
// matching the given config.
func HasIssueTag(p ir.ProbeDefinition, cfg Config) bool {
	_, ok, _ := GetIssueTag(p, cfg)
	return ok
}

// IsIntegrationConfigSkipped returns true if the probe should be skipped for
// the given Config. Each skip_integration: tag value is parsed as a Config
// string via parseConfig and compared with exact equality.
func IsIntegrationConfigSkipped(t testing.TB, p ir.ProbeDefinition, cfg Config) bool {
	for _, tag := range p.GetTags() {
		if !strings.HasPrefix(tag, SkipIntegrationTagPrefix) {
			continue
		}
		skipCfg, err := ParseConfig(tag[len(SkipIntegrationTagPrefix):])
		require.NoError(t, err, "invalid skip_integration tag %q on probe %s", tag, p.GetID())
		if skipCfg == cfg {
			return true
		}
	}
	return false
}
