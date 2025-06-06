// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package testprogs

import (
	"encoding/json"
	"os"
	"path"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/dyninst/config"
)

type probeYaml struct {
	Binary string           `yaml:"binary"`
	Probes []map[string]any `yaml:"probes"`
}

// GetProbeCfgs returns the probe configurations for binary of a given name.
func GetProbeCfgs(t *testing.T, name string) []config.Probe {
	state, err := getState()
	if err != nil {
		t.Fatalf("testprogs: %v", err)
	}
	yamlData, err := os.ReadFile(path.Join(state.probesCfgsDir, name+".yaml"))
	if err != nil {
		t.Fatalf("testprogs: %v", err)
	}
	var probeYaml probeYaml
	err = yaml.Unmarshal(yamlData, &probeYaml)
	if err != nil {
		t.Fatalf("testprogs: %v", err)
	}
	var probesCfgs []config.Probe
	for _, probe := range probeYaml.Probes {
		probeBytes, err := json.Marshal(probe)
		if err != nil {
			t.Fatalf("testprogs: %v", err)
		}
		probeCfg, err := config.UnmarshalProbe(probeBytes)
		if err != nil {
			t.Fatalf("testprogs: %v", err)
		}
		probesCfgs = append(probesCfgs, probeCfg)
	}
	return probesCfgs
}
