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
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	authoredscriptssupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

const (
	testActionFQN    = "com.datadoghq.authoredscripts.test"
	testActionDigest = "ea7829a6ebdaa464eb4fbfff4c72e6e63176df58a430a4b0b8dfb66f0e57149c"
)

type runTestCatalog struct {
	descriptor authoredscriptssupport.Descriptor
	key        string
	readyErr   error
	err        error
}

func (c runTestCatalog) WaitForReady(context.Context) error {
	return c.readyErr
}

func (c runTestCatalog) Lookup(key string) (authoredscriptssupport.Descriptor, error) {
	if c.err != nil {
		return authoredscriptssupport.Descriptor{}, c.err
	}
	wantKey := c.key
	if wantKey == "" {
		wantKey = testActionFQN
	}
	if key != wantKey {
		return authoredscriptssupport.Descriptor{}, authoredscriptssupport.ErrPackageNotConfigured
	}
	return c.descriptor, nil
}

type runTestMaterializer struct {
	mu           sync.Mutex
	materializes int
	fail         bool
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
	script := "#!/bin/sh\nprintf '%s' \"$PAR_ENV_MESSAGE\"\n"
	if m.fail {
		script += "\nprintf secret-stderr >&2\nexit 3\n"
	}
	manifest := fmt.Sprintf(`{
  "schema-version": "v1",
  "version": %q,
  "fqn": %q,
  "command": {"entrypoint": "run.sh"},
  "parameterEnvMapping": {"message": "PAR_ENV_MESSAGE"},
  "allowedEnvVars": ["PAR_AUDIT_TEST_SECRET"],
  "parameterSchema": {"type":"object","properties":{"message":{"type":"string"}}}
}
`, descriptor.Version, descriptor.FQN)
	if err := os.WriteFile(filepath.Join(scriptDir, "metadata.json"), []byte(manifest), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(scriptDir, "run.sh"), []byte(script), 0o700)
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

func TestRunAuthoredScriptUsesExecutionFQNForPackageIdentity(t *testing.T) {
	const executionFQN = "com.datadoghq.authoredscripts.helm.addRepo"
	descriptor := authoredscriptssupport.Descriptor{
		FQN:     executionFQN,
		Package: "com.datadoghq.authoredscripts.helm.addrepo",
		Version: "1.2.3",
		URL:     "oci://registry.example.test/authored-script@sha256:" + testActionDigest,
		SHA256:  testActionDigest,
	}
	resolver, err := authoredscriptssupport.NewArtifactResolver(filepath.Join(t.TempDir(), "cache"), &runTestMaterializer{})
	require.NoError(t, err)
	handler := &RunAuthoredScriptHandler{
		catalog:          runTestCatalog{key: executionFQN, descriptor: descriptor},
		artifactResolver: resolver,
	}
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		BundleID:     "com.datadoghq.helm",
		Name:         "addRepo",
		ExecutionFQN: executionFQN,
		Inputs:       map[string]interface{}{"message": "hello"},
	}

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)
	require.IsType(t, &RunAuthoredScriptOutputs{}, output)
	assert.Equal(t, "hello", output.(*RunAuthoredScriptOutputs).Stdout)
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

	t.Run("catalog readiness failure", func(t *testing.T) {
		expectedErr := errors.New("catalog is unavailable")
		handler := &RunAuthoredScriptHandler{
			catalog: runTestCatalog{readyErr: expectedErr},
		}
		task := &types.Task{}
		task.Data.Attributes = &types.Attributes{BundleID: "com.datadoghq.authoredscripts", Name: "test"}

		_, err := handler.Run(context.Background(), task, nil)
		require.ErrorIs(t, err, expectedErr)
		require.ErrorContains(t, err, "waiting for authored-script catalog")
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

type auditLogger struct {
	log.Logger
	entries []map[string]interface{}
}

func (l *auditLogger) With(...log.Field) log.Logger       { return l }
func (l *auditLogger) Info(_ string, fields ...log.Field) { l.record(fields) }
func (l *auditLogger) Warn(_ string, fields ...log.Field) { l.record(fields) }
func (l *auditLogger) record(fields []log.Field) {
	entry := map[string]interface{}{}
	for _, field := range fields {
		entry[field.Key] = field.Value
	}
	l.entries = append(l.entries, entry)
}

func TestRunAuthoredScriptAuditReporting(t *testing.T) {
	t.Setenv("PAR_AUDIT_TEST_SECRET", "secret-environment")
	for _, tc := range []struct {
		name                           string
		catalogErr                     error
		processFailure, cleanupFailure bool
		stage                          string
		exitCode                       int
	}{
		{name: "success", exitCode: 0},
		{name: "catalog failure", catalogErr: errors.New("secret-catalog-error"), stage: "catalog", exitCode: -1},
		{name: "process failure", processFailure: true, stage: "execution", exitCode: 3},
		{name: "cleanup failure", cleanupFailure: true, stage: "session_cleanup", exitCode: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			descriptor := authoredscriptssupport.Descriptor{
				FQN: testActionFQN, Package: testActionFQN, Version: "1.2.3",
				URL: "oci://registry.example.test/script@sha256:" + testActionDigest, SHA256: testActionDigest,
			}
			resolver, err := authoredscriptssupport.NewArtifactResolver(t.TempDir(), &runTestMaterializer{fail: tc.processFailure})
			require.NoError(t, err)
			handler := &RunAuthoredScriptHandler{catalog: runTestCatalog{descriptor: descriptor, readyErr: tc.catalogErr}, artifactResolver: resolver}
			if tc.cleanupFailure {
				handler.newSession = func() (*authoredscriptssupport.Session, error) {
					session, err := authoredscriptssupport.NewSession()
					if err == nil {
						root := session.RootDirectory
						t.Cleanup(func() { _ = os.RemoveAll(root) })
						session.RootDirectory = "\x00" // Keep command directories valid while forcing cleanup to fail.
					}
					return session, err
				}
			}
			task := &types.Task{}
			task.Data.Attributes = &types.Attributes{BundleID: "com.datadoghq.public", Name: "test", ExecutionFQN: descriptor.FQN, Inputs: map[string]interface{}{"message": "secret-input"}}
			logger := &auditLogger{}
			ctx, details := observability.WithExecutionDetails(log.ContextWithLogger(context.Background(), logger))
			start := time.Now()
			output, err := handler.Run(ctx, task, nil)
			if tc.stage == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Nil(t, output)
			}
			require.Empty(t, logger.entries, "the handler only contributes metadata")
			observability.ReportExecutionCompleted(&statsd.NoOpClient{}, task.Data.Attributes.Client, task.GetFQN(), task.Data.ID, start, err, logger, details)
			require.Len(t, logger.entries, 1)
			fields := logger.entries[0]
			require.Equal(t, "authored_script", fields["execution_kind"])
			require.Equal(t, descriptor.FQN, fields["execution_fqn"])
			require.NotContains(t, fmt.Sprint(fields), "secret-")
			if tc.stage == "" {
				require.NotContains(t, fields, "failure_stage")
			} else {
				require.Equal(t, tc.stage, fields["failure_stage"])
			}
			if tc.catalogErr != nil {
				require.NotContains(t, fields, "package_name")
			} else {
				require.Equal(t, descriptor.Package, fields["package_name"])
				require.Equal(t, descriptor.Version, fields["package_version"])
				require.Equal(t, "sha256:"+descriptor.SHA256, fields["package_digest"])
			}
			if tc.exitCode < 0 {
				require.NotContains(t, fields, "exit_code")
			} else {
				require.Equal(t, tc.exitCode, fields["exit_code"])
			}
		})
	}
}
