// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package controlconfig

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	parcontrolconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/controlconfig"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestResolveControlConfigCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"resolve-control-config"},
		run,
		func() {})
}

// TestRunEmitsSingleJSONSnapshotOnStdout pins the contract par-control depends
// on: stdout carries exactly one versioned JSON document and nothing else.
func TestRunEmitsSingleJSONSnapshotOnStdout(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "datadog.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte("{}\n"), 0600))
	cfg := configmock.NewFromFile(t, configFile)
	cfg.SetInTest("private_action_runner.urn", "urn:dd:apps:on-prem-runner:us1:42:runner")
	cfg.SetInTest("private_action_runner.private_key", "test-private-key")

	hostnameComp, _ := hostnamemock.NewMock(hostnamemock.MockHostname("test-host"))

	stdout := captureStdout(t, func() {
		require.NoError(t, run(cfg, hostnameComp))
	})

	decoder := json.NewDecoder(stdout)
	var got parcontrolconfig.EffectiveConfig
	require.NoError(t, decoder.Decode(&got))
	require.Equal(t, parcontrolconfig.SchemaVersion, got.SchemaVersion)
	require.Equal(t, "urn:dd:apps:on-prem-runner:us1:42:runner", got.URN)
	require.Equal(t, "test-private-key", got.PrivateKey)

	// Nothing may follow the snapshot: Rust reads stdout as one document.
	var trailing json.RawMessage
	require.ErrorIs(t, decoder.Decode(&trailing), io.EOF)
}

func captureStdout(t *testing.T, fn func()) io.Reader {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	saved := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = saved
	}()

	fn()
	require.NoError(t, writer.Close())

	captured, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	return bytes.NewReader(captured)
}
