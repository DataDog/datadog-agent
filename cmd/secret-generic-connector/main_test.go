// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
)

const runMainEnv = "SGC_TEST_RUN_MAIN"

// TestMain lets the tests re-exec the test binary as the connector itself.
func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runConnector runs the connector in a subprocess with the given stdin and arguments.
func runConnector(t *testing.T, stdin []byte, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), runMainEnv+"=1")
	cmd.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	require.NoError(t, err, "connector failed, stderr: %s", stderr.String())
	return stdout
}

func TestFileBackends(t *testing.T) {
	tests := []struct {
		backendType string
		fileName    string
		content     string
	}{
		{
			backendType: "file.yaml",
			fileName:    "secrets.yaml",
			content:     "fake_yaml_key: fake_yaml_value\n",
		},
		{
			backendType: "file.json",
			fileName:    "secrets.json",
			content:     `{"fake_yaml_key": "fake_yaml_value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.backendType, func(t *testing.T) {
			secretsPath := filepath.Join(t.TempDir(), tt.fileName)
			require.NoError(t, os.WriteFile(secretsPath, []byte(tt.content), 0600))

			input, err := json.Marshal(secret.Input{
				Version: "1.0",
				Type:    tt.backendType,
				Config:  map[string]interface{}{"file_path": secretsPath},
				Secrets: []string{"fake_yaml_key", "missing_key"},
			})
			require.NoError(t, err)

			var outputs map[string]secret.Output
			require.NoError(t, json.Unmarshal(runConnector(t, input), &outputs))
			require.Len(t, outputs, 2)

			found := outputs["fake_yaml_key"]
			require.NotNil(t, found.Value)
			assert.Equal(t, "fake_yaml_value", *found.Value)
			assert.Nil(t, found.Error)

			missing := outputs["missing_key"]
			assert.Nil(t, missing.Value)
			require.NotNil(t, missing.Error)
			assert.Equal(t, secret.ErrKeyNotFound.Error(), *missing.Error)
		})
	}
}

func TestVersionFlag(t *testing.T) {
	program, err := os.Executable()
	require.NoError(t, err)

	stdout := runConnector(t, nil, "-version")
	assert.Equal(t, filepath.Base(program)+" dev\n", string(stdout))
}
