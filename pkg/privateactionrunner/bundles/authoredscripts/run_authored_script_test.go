// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package com_datadoghq_authoredscripts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authoredscriptssupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

const (
	testActionFQN    = "com.datadoghq.authoredscripts.test"
	testActionDigest = "ea7829a6ebdaa464eb4fbfff4c72e6e63176df58a430a4b0b8dfb66f0e57149c"
)

type runTestCatalog struct {
	descriptor authoredscriptssupport.Descriptor
	err        error
}

func (c runTestCatalog) Lookup(key string) (authoredscriptssupport.Descriptor, error) {
	if c.err != nil {
		return authoredscriptssupport.Descriptor{}, c.err
	}
	if key != testActionFQN {
		return authoredscriptssupport.Descriptor{}, authoredscriptssupport.ErrPackageNotConfigured
	}
	return c.descriptor, nil
}

type runTestMaterializer struct {
	mu           sync.Mutex
	materializes int
}

func (*runTestMaterializer) MaterializationID() string {
	return "test-linux-amd64"
}

func (m *runTestMaterializer) Materialize(_ context.Context, descriptor authoredscriptssupport.Descriptor, destination string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.materializes++

	scriptDir := filepath.Join(destination, "script")
	if err := os.MkdirAll(scriptDir, 0o700); err != nil {
		return err
	}
	manifest := fmt.Sprintf(`{
  "schema-version": "v1",
  "version": %q,
  "fqn": %q,
  "command": {"entrypoint": "run.sh"},
  "parameterEnvMapping": {"message": "PAR_ENV_MESSAGE"}
}
`, descriptor.Version, descriptor.FQN)
	if err := os.WriteFile(filepath.Join(scriptDir, "metadata.json"), []byte(manifest), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(scriptDir, "run.sh"), []byte("#!/bin/sh\nprintf '%s' \"$PAR_ENV_MESSAGE\"\n"), 0o700)
}

func (m *runTestMaterializer) materializationCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.materializes
}

func TestRunAuthoredScriptUsesInjectedArtifactResolver(t *testing.T) {
	descriptor := authoredscriptssupport.Descriptor{
		FQN:     testActionFQN,
		Package: testActionFQN,
		Version: "1.2.3",
		URL:     "oci://registry.example.test/authored-script@sha256:" + testActionDigest,
		SHA256:  testActionDigest,
	}
	materializer := &runTestMaterializer{}
	resolver, err := authoredscriptssupport.NewArtifactResolver(filepath.Join(t.TempDir(), "cache"), materializer)
	require.NoError(t, err)
	handler := &RunAuthoredScriptHandler{
		catalog:          runTestCatalog{descriptor: descriptor},
		artifactResolver: resolver,
	}
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		BundleID: "com.datadoghq.authoredscripts",
		Name:     "test",
		Inputs:   map[string]interface{}{"message": "hello"},
	}

	first, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)
	second, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	require.IsType(t, &RunAuthoredScriptOutputs{}, first)
	require.IsType(t, &RunAuthoredScriptOutputs{}, second)
	firstOutput := first.(*RunAuthoredScriptOutputs)
	secondOutput := second.(*RunAuthoredScriptOutputs)
	assert.Equal(t, 0, firstOutput.ExitCode)
	assert.Equal(t, "hello", firstOutput.Stdout)
	assert.Empty(t, firstOutput.Stderr)
	assert.Equal(t, 0, secondOutput.ExitCode)
	assert.Equal(t, "hello", secondOutput.Stdout)
	assert.Empty(t, secondOutput.Stderr)
	assert.Equal(t, 1, materializer.materializationCount())
}

func TestRunAuthoredScriptValidation(t *testing.T) {
	t.Run("nil handler", func(t *testing.T) {
		var handler *RunAuthoredScriptHandler
		_, err := handler.Run(context.Background(), nil, nil)
		require.ErrorContains(t, err, "handler is not configured")
	})

	t.Run("missing task", func(t *testing.T) {
		handler := &RunAuthoredScriptHandler{catalog: runTestCatalog{}}
		_, err := handler.Run(context.Background(), nil, nil)
		require.ErrorContains(t, err, "task is required")
	})

	t.Run("catalog lookup failure", func(t *testing.T) {
		handler := &RunAuthoredScriptHandler{
			catalog: runTestCatalog{err: authoredscriptssupport.ErrPackageNotConfigured},
		}
		task := &types.Task{}
		task.Data.Attributes = &types.Attributes{BundleID: "com.datadoghq.authoredscripts", Name: "test"}

		_, err := handler.Run(context.Background(), task, nil)
		require.ErrorIs(t, err, authoredscriptssupport.ErrPackageNotConfigured)
		require.ErrorContains(t, err, "could not look up authored-script package")
	})

	t.Run("cache initialization error", func(t *testing.T) {
		expectedErr := errors.New("cache initialization failed")
		handler := &RunAuthoredScriptHandler{
			catalog:                 runTestCatalog{descriptor: authoredscriptssupport.Descriptor{}},
			artifactResolverInitErr: expectedErr,
		}
		task := &types.Task{}
		task.Data.Attributes = &types.Attributes{BundleID: "com.datadoghq.authoredscripts", Name: "test"}

		_, err := handler.Run(context.Background(), task, nil)
		require.ErrorIs(t, err, expectedErr)
	})
}
