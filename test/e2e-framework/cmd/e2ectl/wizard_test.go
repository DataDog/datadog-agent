// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestRunSuffix(t *testing.T) {
	assert.Equal(t, "", testRunSuffix(""))
	assert.Equal(t, " -run TestFoo", testRunSuffix("TestFoo"))
}

func TestConfirmDestroy(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"exact match", "kind-nopulumi\n", true},
		{"mismatch", "wrong-name\n", false},
		{"whitespace trimmed", "  kind-nopulumi  \n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tc.input))
			got, err := confirmDestroy(scanner, "kind-nopulumi")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestConfirmDestroyNoInput(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader(""))
	got, err := confirmDestroy(scanner, "kind-nopulumi")
	require.NoError(t, err)
	assert.False(t, got)
}

// TestRunEnvLoopGuardsUnprovisionedInstallAndTest covers the fix for
// actions "2" (install/update agent) and "3" (run test) being reachable
// before "1" (provision infra) ever succeeded, which used to fall through
// into doInstall/doTest and fail deep inside with a confusing raw error.
// Picking "2" then "3" on a never-provisioned environment must print a
// short guard message and loop back to the menu without ever invoking
// doInstall/doTest (which would shell out to real installers/`go test`).
func TestRunEnvLoopGuardsUnprovisionedInstallAndTest(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "guard.state.json")
	def := TestDefinition{
		Name:  "guard-test",
		Agent: agentConfig{Installer: "helm-k8s"},
		Test:  testConfig{Package: "./..."},
	}

	r, w, err := os.Pipe()
	require.NoError(t, err)
	origStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	scanner := bufio.NewScanner(strings.NewReader("2\n3\nq\n"))
	outcome, loopErr := runEnvLoop(context.Background(), def, "unused-config.yaml", statePath, false, scanner)

	require.NoError(t, w.Close())
	os.Stdout = origStdout
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	output := buf.String()

	require.NoError(t, loopErr)
	assert.Equal(t, loopQuit, outcome)
	assert.Equal(t, 2, strings.Count(output, "infra not provisioned yet — pick 1 first"))
}
