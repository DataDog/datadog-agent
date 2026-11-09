// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package com_datadoghq_authoredscripts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authoredscriptssupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

const (
	windowsTestActionFQN    = "com.datadoghq.authoredscripts.windowsTest"
	windowsTestActionDigest = "34c13b5d4f6d70b40df8b702988b6801c76c461ae63aa7867332058265e90809"
)

type windowsRunTestCatalog struct {
	descriptor authoredscriptssupport.Descriptor
}

func (windowsRunTestCatalog) WaitForReady(context.Context) error {
	return nil
}

func (c windowsRunTestCatalog) Lookup(string) (authoredscriptssupport.Descriptor, error) {
	return c.descriptor, nil
}

type windowsRunTestMaterializer struct{}

func (windowsRunTestMaterializer) MaterializationID() string {
	return "test-windows-amd64"
}

func (windowsRunTestMaterializer) Materialize(_ context.Context, descriptor authoredscriptssupport.Descriptor, destination string) error {
	scriptDirectory := filepath.Join(destination, "script")
	if err := os.MkdirAll(scriptDirectory, 0o700); err != nil {
		return err
	}
	manifest := fmt.Sprintf(`{
  "schema-version": "v1",
  "version": %q,
  "fqn": %q,
  "command": {"entrypoint": "run.ps1"},
  "parameterEnvMapping": {"message": "PAR_ENV_MESSAGE"}
}
`, descriptor.Version, descriptor.FQN)
	if err := os.WriteFile(filepath.Join(scriptDirectory, "metadata.json"), []byte(manifest), 0o600); err != nil {
		return err
	}
	return os.WriteFile(
		filepath.Join(scriptDirectory, "run.ps1"),
		[]byte(`[Console]::Out.Write($env:PAR_ENV_MESSAGE)`),
		0o700,
	)
}

func TestRunAuthoredScriptExecutesPowerShellOnWindows(t *testing.T) {
	descriptor := authoredscriptssupport.Descriptor{
		FQN:     windowsTestActionFQN,
		Package: "com.datadoghq.authoredscripts.windowstest",
		Version: "1.2.3",
		URL:     "oci://registry.example.test/authored-script@sha256:" + windowsTestActionDigest,
		SHA256:  windowsTestActionDigest,
	}
	resolver, err := authoredscriptssupport.NewArtifactResolver(t.TempDir(), windowsRunTestMaterializer{})
	require.NoError(t, err)
	handler := &RunAuthoredScriptHandler{
		catalog:          windowsRunTestCatalog{descriptor: descriptor},
		artifactResolver: resolver,
	}
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		BundleID: "com.datadoghq.authoredscripts",
		Name:     "windowsTest",
		Inputs:   map[string]interface{}{"message": "hello from windows"},
	}

	output, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := output.(*RunAuthoredScriptOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello from windows", result.Stdout)
	assert.Empty(t, result.Stderr)
}
