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

type auditEntry struct {
	level  string
	fields map[string]interface{}
}

type auditLogger struct {
	log.Logger
	entries *[]auditEntry
	fields  []log.Field
}

func (l auditLogger) With(fields ...log.Field) log.Logger {
	l.fields = append(append([]log.Field{}, l.fields...), fields...)
	return l
}
func (l auditLogger) record(level string, fields []log.Field) {
	entry := auditEntry{level: level, fields: map[string]interface{}{}}
	for _, field := range append(append([]log.Field{}, l.fields...), fields...) {
		entry.fields[field.Key] = field.Value
	}
	*l.entries = append(*l.entries, entry)
}
func (l auditLogger) Info(_ string, fields ...log.Field) { l.record("info", fields) }
func (l auditLogger) Warn(_ string, fields ...log.Field) { l.record("warn", fields) }

func TestRunAuthoredScriptAuditReporting(t *testing.T) {
	t.Setenv("PAR_AUDIT_TEST_SECRET", "secret-environment")
	for _, tc := range []struct {
		name            string
		catalogErr      error
		processFailure  bool
		badInputs       bool
		cleanupFailure  bool
		sessionPanic    bool
		cancelExecution bool
		stage           string
		exitCode        *int
	}{
		{name: "success", exitCode: auditInt(0)},
		{name: "catalog failure", catalogErr: errors.New("secret-catalog-error"), stage: "catalog"},
		{name: "timeout", catalogErr: context.DeadlineExceeded, stage: "catalog"},
		{name: "cancellation", catalogErr: context.Canceled, stage: "catalog"},
		{name: "execution cancellation", cancelExecution: true, stage: "execution"},
		{name: "process failure", processFailure: true, stage: "execution", exitCode: auditInt(3)},
		{name: "invalid inputs", badInputs: true, stage: "command_preparation"},
		{name: "panic", sessionPanic: true},
		{name: "cleanup failure", cleanupFailure: true, stage: "session_cleanup", exitCode: auditInt(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			descriptor := authoredscriptssupport.Descriptor{
				FQN:     "com.datadoghq.authoredscripts.test",
				Package: "com.datadoghq.authoredscripts.test", Version: "1.2.3",
				SHA256: "ea7829a6ebdaa464eb4fbfff4c72e6e63176df58a430a4b0b8dfb66f0e57149c",
			}
			descriptor.URL = "oci://registry.example.test/script@sha256:" + descriptor.SHA256
			resolver, err := authoredscriptssupport.NewArtifactResolver(t.TempDir(), &runTestMaterializer{fail: tc.processFailure})
			require.NoError(t, err)
			handler := &RunAuthoredScriptHandler{catalog: runTestCatalog{descriptor: descriptor, readyErr: tc.catalogErr}, artifactResolver: resolver}
			if tc.cleanupFailure {
				handler.newSession = func() (*authoredscriptssupport.Session, error) {
					session, err := authoredscriptssupport.NewSession()
					if err == nil {
						root := session.RootDirectory
						t.Cleanup(func() { _ = os.RemoveAll(root) })
						// Command directories stay valid; cleanup deterministically fails on every OS.
						session.RootDirectory = "\x00"
					}
					return session, err
				}
			}
			if tc.sessionPanic {
				handler.newSession = func() (*authoredscriptssupport.Session, error) { panic("secret-panic") }
			}
			task := &types.Task{}
			task.Data.ID = "task-123"
			task.Data.Attributes = &types.Attributes{BundleID: "com.datadoghq.public", Name: "test", ExecutionFQN: descriptor.FQN, Inputs: map[string]interface{}{"message": "secret-value"}}
			if tc.badInputs {
				task.Data.Attributes.Inputs = map[string]interface{}{"message": false}
			}
			var entries []auditEntry
			logger := auditLogger{entries: &entries}
			baseCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			loggerWithIdentity := logger.With(
				log.String("runner_id", "runner-123"),
				log.String("task_id", task.Data.ID),
				log.String("action_fqn", task.GetFQN()),
			)
			ctx, details := observability.WithExecutionDetails(log.ContextWithLogger(baseCtx, loggerWithIdentity))
			metrics := &statsd.NoOpClient{}
			start := observability.ReportExecutionStart(metrics, task.Data.Attributes.Client, task.GetFQN(), task.Data.ID, loggerWithIdentity)
			if tc.cancelExecution {
				handler.newSession = func() (*authoredscriptssupport.Session, error) {
					session, err := authoredscriptssupport.NewSession()
					cancel()
					return session, err
				}
			}
			if tc.sessionPanic {
				require.Panics(t, func() { _, _ = handler.Run(ctx, task, nil) })
				require.Len(t, entries, 1, "an unhandled panic must not report successful completion")
				return
			}
			output, err := handler.Run(ctx, task, nil)
			if tc.stage == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Nil(t, output)
			}
			if tc.cancelExecution {
				require.ErrorIs(t, err, context.Canceled)
			}
			require.Len(t, entries, 1, "the handler must not emit lifecycle logs")
			observability.ReportExecutionCompleted(metrics, task.Data.Attributes.Client, task.GetFQN(), task.Data.ID, start, err, loggerWithIdentity, details)
			require.Len(t, entries, 2, "one shared start and one shared completion")
			require.NotContains(t, entries[0].fields, "package_name")
			final := entries[1]
			require.Equal(t, "authored_script", final.fields["execution_kind"])
			require.Equal(t, descriptor.FQN, final.fields["execution_fqn"])
			require.GreaterOrEqual(t, final.fields["duration"].(time.Duration), time.Duration(0))
			require.Equal(t, "execution.outcome", final.fields["event"])
			for _, entry := range entries {
				require.Equal(t, "runner-123", entry.fields["runner_id"])
				require.Equal(t, "task-123", entry.fields["task_id"])
				require.Equal(t, task.GetFQN(), entry.fields["action_fqn"])
				require.NotContains(t, fmt.Sprint(entry.fields), "secret-")
			}
			if tc.stage == "" {
				require.Equal(t, "info", final.level)
				require.Equal(t, "success", final.fields["outcome"])
				require.NotContains(t, final.fields, "failure_stage")
			} else {
				require.Equal(t, "warn", final.level)
				require.Equal(t, "failure", final.fields["outcome"])
				require.Equal(t, tc.stage, final.fields["failure_stage"])
			}
			if tc.catalogErr != nil {
				require.NotContains(t, final.fields, "package_name")
			} else {
				require.Equal(t, descriptor.Package, final.fields["package_name"])
				require.Equal(t, descriptor.Version, final.fields["package_version"])
				require.Equal(t, "sha256:"+descriptor.SHA256, final.fields["package_digest"])
			}
			if tc.exitCode == nil {
				require.NotContains(t, final.fields, "exit_code")
			} else {
				require.Equal(t, *tc.exitCode, final.fields["exit_code"])
			}
		})
	}
}

func auditInt(value int) *int { return &value }
