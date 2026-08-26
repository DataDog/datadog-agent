// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package agentimpl

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditorimpl "github.com/DataDog/datadog-agent/comp/logs/auditor/impl"
	compressionmock "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

// setRegistryConfigForTest points the registry auditor at runPath and
// restores the previous values via t.Cleanup. Both serverless_registry_test.go
// cases rely on the auditor reading its registry from - and writing it back
// to - this directory. Must be called right before NewServerlessLogsAgent:
// taggerfxmock.SetupFakeTagger's fx app reloads the global Datadog() config
// from scratch, which resets logs_config.run_path to its default - so
// setting it any earlier gets silently undone.
func setRegistryConfigForTest(t *testing.T, runPath string) {
	cfg := pkgconfigsetup.Datadog()
	cfg.Set("logs_config.run_path", runPath, model.SourceAgentRuntime)
	// Non-atomic writer MkdirAll's the registry dir, matching production
	// (cmd/serverless-init/main.go preloadEarly).
	cfg.Set("logs_config.atomic_registry_write", false, model.SourceAgentRuntime)
	t.Cleanup(func() {
		cfg.UnsetForSource("logs_config.run_path", model.SourceAgentRuntime)
		cfg.UnsetForSource("logs_config.atomic_registry_write", model.SourceAgentRuntime)
	})
}

// newTestServerlessLogsAgent builds and starts a serverless logs agent wired
// to intake, mirroring the setup in serverless_drain_test.go. The registry
// auditor is pointed at runPath immediately before construction (see
// setRegistryConfigForTest).
func newTestServerlessLogsAgent(t *testing.T, intake *capturingIntake, runPath string) *logAgent {
	endpoints := config.NewEndpoints(intake.endpoint(t), nil, false, true)

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeCompression := compressionmock.NewMockCompressor()
	hostnameService := hostnameimpl.NewHostnameService()

	setRegistryConfigForTest(t, runPath)
	serverlessLogsAgent := NewServerlessLogsAgent(fakeTagger, fakeCompression, hostnameService, true)
	logsAgent, ok := serverlessLogsAgent.(*logAgent)
	require.True(t, ok, "Expected NewServerlessLogsAgent to return *logAgent type")

	logsAgent.endpoints = endpoints
	require.NoError(t, logsAgent.setupAgent())
	logsAgent.startPipeline()
	t.Cleanup(func() { _ = logsAgent.stop(context.Background()) })
	return logsAgent
}

// addFileSourceAndWaitForTailer registers a file source for path with the
// given tailing mode and waits for the file launcher to attach a tailer to
// it, mirroring TestDrainTailersDeliversUnreadLine's synchronization.
func addFileSourceAndWaitForTailer(t *testing.T, logsAgent *logAgent, path string, tailingMode string) {
	logConfig := config.LogsConfig{
		Type:        config.FileType,
		Path:        path,
		Identifier:  "registry-test",
		TailingMode: tailingMode,
	}
	source := sources.NewLogSource("registry-test", &logConfig)
	logsAgent.sources.AddSource(source)

	testutil.AssertTrueBeforeTimeout(t, 5*time.Millisecond, 2*time.Second, func() bool {
		return len(logsAgent.tracker.All()) == 1
	})
}

// TestBeginningCapturesPreexistingLine reproduces the cold-start gap: the
// customer app writes a line before the tailer ever attaches (a fresh
// instance, so there's no registry.json yet). With tailingMode "beginning"
// and a real registry auditor, the tailer must read from offset 0 and
// capture that line instead of skipping straight to EOF.
func TestBeginningCapturesPreexistingLine(t *testing.T) {
	dir := t.TempDir()

	logPath := filepath.Join(dir, "app.log")
	require.NoError(t, os.WriteFile(logPath, []byte("startup line\n"), 0o644))

	intake := newCapturingIntake(t)
	logsAgent := newTestServerlessLogsAgent(t, intake, dir)

	addFileSourceAndWaitForTailer(t, logsAgent, logPath, "beginning")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	logsAgent.DrainTailers(ctx)
	logsAgent.Flush(ctx)

	// pipelineProvider.Flush blocks on the sender's WaitGroup, but under CPU
	// contention the message can still be mid-flight between the decoder and
	// the sender when Flush returns (a pre-existing timing gap, also visible
	// in serverless_drain_test.go). Poll briefly instead of asserting once.
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 2*time.Second, func() bool {
		return intake.count("startup line") == 1
	})
}

// TestRegistryResumeNoDuplicate reproduces a restart within the same
// instance: the registry already has a persisted offset past "startup
// line". Per pkg/logs/launchers/file/position.go, a persisted offset wins
// over the tailing mode, so the tailer must resume from it and must not
// re-read "startup line" even though tailingMode is "beginning".
func TestRegistryResumeNoDuplicate(t *testing.T) {
	dir := t.TempDir()

	logPath := filepath.Join(dir, "app.log")
	const preexisting = "startup line\n"
	require.NoError(t, os.WriteFile(logPath, []byte(preexisting+"new line\n"), 0o644))
	seedRegistry(t, dir, "file:"+logPath, int64(len(preexisting)))

	intake := newCapturingIntake(t)
	logsAgent := newTestServerlessLogsAgent(t, intake, dir)

	addFileSourceAndWaitForTailer(t, logsAgent, logPath, "beginning")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	logsAgent.DrainTailers(ctx)
	logsAgent.Flush(ctx)

	// See TestBeginningCapturesPreexistingLine for why this polls rather than
	// asserting once immediately after Flush.
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 2*time.Second, func() bool {
		return intake.count("new line") == 1
	})
	require.Equal(t, 0, intake.count("startup line"))
}

// seedRegistry writes a registry.json into dir with a single entry for
// identifier at offset, simulating a persisted offset from a prior run of
// the same instance.
func seedRegistry(t *testing.T, dir string, identifier string, offset int64) {
	registry := auditorimpl.JSONRegistry{
		Version: 2,
		Registry: map[string]auditorimpl.RegistryEntry{
			identifier: {
				LastUpdated: time.Now().UTC(),
				Offset:      strconv.FormatInt(offset, 10),
				TailingMode: "beginning",
			},
		},
	}
	data, err := json.Marshal(registry)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "registry.json"), data, 0o644))
}
