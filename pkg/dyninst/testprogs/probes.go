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
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/config"
)

type probeYaml struct {
	Binary string           `yaml:"binary"`
	Probes []map[string]any `yaml:"probes"`
}

// MustGetProbeCfgs calls GetProbeCfgs and checks for an error.
func MustGetProbeCfgs(t *testing.T, name string) []config.Probe {
	probes, err := GetProbeCfgs(name)
	require.NoError(t, err)
	return probes
}

// GetProbeCfgs returns the probe configurations for binary of a given name.
func GetProbeCfgs(name string) ([]config.Probe, error) {
	probes, err := getProbeCfgs(name)
	if err != nil {
		return nil, fmt.Errorf("testprogs: %w", err)
	}
	return probes, nil
}

func getProbeCfgs(name string) ([]config.Probe, error) {
	state, err := GetState()
	if err != nil {
		return nil, err
	}
	yamlData, err := os.ReadFile(path.Join(state.ProbesCfgsDir, name+".yaml"))
	if err != nil {
		return nil, err
	}
	var probeYaml probeYaml
	err = yaml.Unmarshal(yamlData, &probeYaml)
	if err != nil {
		return nil, err
	}
	var probesCfgs []config.Probe
	for _, probe := range probeYaml.Probes {
		probeBytes, err := json.Marshal(probe)
		if err != nil {
			return nil, err
		}
		probeCfg, err := config.UnmarshalProbe(probeBytes)
		if err != nil {
			return nil, err
		}
		probesCfgs = append(probesCfgs, probeCfg)
	}
	return probesCfgs, nil
}
