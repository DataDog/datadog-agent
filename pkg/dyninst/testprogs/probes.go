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
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

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

// GetIssueTag returns the issue tag for a probe definition.
func GetIssueTag(p ir.ProbeDefinition) (string, bool) {
	tags := p.GetTags()
	index := slices.IndexFunc(tags, func(tag string) bool {
		return strings.HasPrefix(tag, IssueTagPrefix)
	})
	if index == -1 {
		return "", false
	}
	return tags[index][len(IssueTagPrefix):], true
}

// HasIssueTag returns true if the probe definition has an issue tag.
func HasIssueTag(p ir.ProbeDefinition) bool {
	_, ok := GetIssueTag(p)
	return ok
}
