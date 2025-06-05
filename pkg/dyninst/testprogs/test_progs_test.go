// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package testprogs

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

const inSubprocessEnvVar = "DD_DYNINST_TESTPROGS_IN_SUBPROCESS"

func isInSubprocess() bool {
	b, _ := strconv.ParseBool(os.Getenv(inSubprocessEnvVar))
	return b
}

type configAndPrograms struct {
	Configs  []Config
	Programs []string
}

func TestInitFromBinaries(t *testing.T) {
	if isInSubprocess() {
		testInitFromBinariesInSubprocess(t)
		return
	}

	var cases = []struct {
		name     string
		layout   map[string][]string
		expected configAndPrograms
	}{
		{
			name: "only overlapping binaries",
			layout: map[string][]string{
				"pkg/dyninst/testprogs/binaries/arch=amd64,toolchain=go1.22.5": {
					"foo",
					"bar",
					".flock",
				},
				"pkg/dyninst/testprogs/binaries/arch=arm64,toolchain=go1.22.5": {
					"foo",
					".flock",
				},
			},
			expected: configAndPrograms{
				Configs: []Config{
					{
						GOARCH:      "amd64",
						GOTOOLCHAIN: "go1.22.5",
					},
					{
						GOARCH:      "arm64",
						GOTOOLCHAIN: "go1.22.5",
					},
				},
				Programs: []string{
					"foo",
				},
			},
		},
	}
	outerTestName := t.Name()
	runCase := func(t *testing.T, expected configAndPrograms, layout map[string][]string) {
		tmpDir, err := os.MkdirTemp("", "test_progs_test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)
		for path, programs := range layout {
			fullPath := filepath.Join(tmpDir, path)
			os.MkdirAll(fullPath, 0755)
			for _, program := range programs {
				os.WriteFile(filepath.Join(fullPath, program), []byte{}, 0644)
			}
		}

		cmd := exec.Command(os.Args[0], "--test.run="+outerTestName)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.Env = append(
			os.Environ(),
			inSubprocessEnvVar+"=true",
		)
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf(
				"failed to run testInitFromBinariesInSubprocess: %v\nstdout:\n%s\nstderr:\n%s",
				err,
				stdout.String(),
				stderr.String(),
			)
		}

		{
			var actual configAndPrograms
			err := json.Unmarshal(stderr.Bytes(), &actual)
			require.NoError(t, err, "stdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
			require.Equal(t, expected, actual)
		}
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			runCase(t, c.expected, c.layout)
		})
	}
}

func testInitFromBinariesInSubprocess(t *testing.T) {
	state, err := getState()
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}
	out, err := json.Marshal(configAndPrograms{
		Configs:  state.commonConfigs,
		Programs: state.programs,
	})
	if err != nil {
		t.Fatalf("failed to marshal config and programs: %v", err)
	}
	_, err = os.Stderr.Write(out)
	require.NoError(t, err)
}
