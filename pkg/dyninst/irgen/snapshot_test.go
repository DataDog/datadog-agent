// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen_test

import (
	"encoding/json"
	"flag"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/dyninst/config"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irprinter"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

var rewriteFromEnv = func() bool {
	rewrite, _ := strconv.ParseBool(os.Getenv("REWRITE"))
	return rewrite
}()
var rewrite = flag.Bool("rewrite", rewriteFromEnv, "rewrite the test files")

const snapshotDir = "testdata/snapshot"

func TestSnapshotTesting(t *testing.T) {
	files, err := os.ReadDir(snapshotDir)
	require.NoError(t, err)
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if !strings.HasSuffix(file.Name(), ".yaml") {
			continue
		}
		testFile := path.Join(snapshotDir, file.Name())
		t.Run(file.Name(), func(t *testing.T) {
			runFile(t, testFile)
		})
	}
}

func runFile(t *testing.T, path string) {
	outDir := strings.TrimSuffix(path, ".yaml")
	os.MkdirAll(outDir, 0755)
	yamlFile, err := os.ReadFile(path)
	require.NoError(t, err)
	testFile, err := deserializeTestFile(yamlFile)
	require.NoError(t, err)
	cfgs := testprogs.GetCommonConfigs(t)
	assert.NotEmpty(t, cfgs)
	for _, cfg := range cfgs {
		t.Run(cfg.String(), func(t *testing.T) {
			runTest(t, cfg, outDir, testFile)
		})
	}
}

func runTest(
	t *testing.T,
	cfg testprogs.Config,
	outDir string,
	testFile *testFile,
) {
	binPath := testprogs.GetBinary(t, testFile.binary, cfg)
	elfFile, err := safeelf.Open(binPath)
	require.NoError(t, err)
	obj, err := object.NewElfObject(elfFile)
	require.NoError(t, err)
	defer func() { require.NoError(t, elfFile.Close()) }()
	ir, err := irgen.GenerateIR(1, obj, testFile.probes)
	require.NoError(t, err)

	marshaled, err := irprinter.PrintYAML(ir)
	require.NoError(t, err)

	outputFile := path.Join(outDir, cfg.String()+".yaml")
	if *rewrite {
		tmpFile, err := os.CreateTemp(outDir, "ir.yaml")
		require.NoError(t, err)
		name := tmpFile.Name()
		defer func() { _ = os.Remove(name) }()
		_, err = tmpFile.Write(marshaled)
		require.NoError(t, err)
		require.NoError(t, tmpFile.Close())
		require.NoError(t, os.Rename(name, outputFile))
	} else {
		expected, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		require.Equal(t, string(expected), string(marshaled))
	}
}

type probeYaml struct {
	Binary string           `yaml:"binary"`
	Probes []map[string]any `yaml:"probes"`
}

type testFile struct {
	binary string
	probes []config.Probe
}

func deserializeTestFile(input []byte) (*testFile, error) {
	var probeYaml probeYaml
	err := yaml.Unmarshal(input, &probeYaml)
	if err != nil {
		return nil, err
	}
	var probes []config.Probe
	for _, probe := range probeYaml.Probes {
		probeBytes, err := json.Marshal(probe)
		if err != nil {
			return nil, err
		}
		probe, err := config.UnmarshalProbe(probeBytes)
		if err != nil {
			return nil, err
		}
		probes = append(probes, probe)
	}
	return &testFile{
		binary: probeYaml.Binary,
		probes: probes,
	}, nil
}
