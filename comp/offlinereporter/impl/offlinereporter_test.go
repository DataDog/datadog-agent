// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package offlinereporterimpl

import (
	"strconv"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	offlinereporter "github.com/DataDog/datadog-agent/comp/offlinereporter/def"
	logscompressionmock "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompressionmock "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const testRunPath = "/run"
const testFilePath = "/run/agent_heartbeat"

type testDeps struct {
	fx.In
	Reporter offlinereporter.Component
	Demux    demultiplexer.FakeSamplerMock
}

// newTestOptions creates an in-memory filesystem and the fx options to wire a
// real offlinereporter component against mock dependencies. The returned fs can
// be used by callers to pre-seed or inspect the heartbeat file. Pass
// enabled=false to simulate telemetry.offlinereporter.enabled=false.
func newTestOptions(t *testing.T, enabled bool) (afero.Fs, fx.Option) {
	fs := afero.NewMemMapFs()
	return fs, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			return config.NewMockWithOverrides(t, map[string]interface{}{
				"telemetry.offlinereporter.enabled":            enabled,
				"telemetry.offlinereporter.heartbeat_interval": "5s",
				"run_path": testRunPath,
			})
		}),
		fx.Supply(Params{Fs: fs}),
		fxutil.ProvideComponentConstructor(NewComponent),
		hostnameimpl.MockModule(),
		demultiplexerimpl.FakeSamplerMockModule(),
		logscompressionmock.MockModule(),
		metricscompressionmock.MockModule(),
		// FakeSamplerMock's concrete type (*fakeSamplerMock) embeds *AgentDemultiplexer
		// and therefore implements demultiplexer.Component. Provide an explicit adapter
		// so fx can satisfy Requires.Demultiplexer.
		fx.Provide(func(m demultiplexer.FakeSamplerMock) demultiplexer.Component {
			return m.(demultiplexer.Component)
		}),
	)
}

// TestFirstRun verifies SendOfflineDuration is a no-op when no previous file exists.
func TestFirstRun(t *testing.T) {
	_, opts := newTestOptions(t, true)
	deps := fxutil.Test[testDeps](t, opts)

	deps.Reporter.SendOfflineDuration("test.offline", nil)

	_, timed := deps.Demux.WaitForSamples(50 * time.Millisecond)
	assert.Empty(t, timed, "expected no samples on first run")
}

// TestSecondRun verifies SendOfflineDuration sends a gauge ≈ seconds since
// the previous heartbeat.
func TestSecondRun(t *testing.T) {
	fs, opts := newTestOptions(t, true)
	pastTs := time.Now().Add(-10 * time.Second).Unix()
	require.NoError(t, afero.WriteFile(fs, testFilePath, []byte(strconv.FormatInt(pastTs, 10)), 0600))

	deps := fxutil.Test[testDeps](t, opts)

	deps.Reporter.SendOfflineDuration("test.offline", []string{"env:test"})

	_, timed := deps.Demux.WaitForSamples(1 * time.Second)
	require.Len(t, timed, 1)
	sample := timed[0]
	assert.Equal(t, "test.offline", sample.Name)
	assert.Equal(t, "my-hostname", sample.Host)
	assert.Equal(t, metrics.GaugeType, sample.Mtype)
	assert.InDelta(t, 10.0, sample.Value, 2.0, "offline duration should be ~10s")
}

// TestCorruptFile verifies that a corrupt heartbeat file is treated as a first run.
func TestCorruptFile(t *testing.T) {
	fs, opts := newTestOptions(t, true)
	require.NoError(t, afero.WriteFile(fs, testFilePath, []byte("not-a-timestamp"), 0600))

	deps := fxutil.Test[testDeps](t, opts)

	deps.Reporter.SendOfflineDuration("test.offline", nil)

	_, timed := deps.Demux.WaitForSamples(50 * time.Millisecond)
	assert.Empty(t, timed, "corrupt file should be treated as first run")
}

// TestOnStart_WritesFile verifies the heartbeat file is created on startup.
func TestOnStart_WritesFile(t *testing.T) {
	fs, opts := newTestOptions(t, true)
	fxutil.Test[testDeps](t, opts)

	var secs int64
	require.Eventually(t, func() bool {
		data, err := afero.ReadFile(fs, testFilePath)
		if err != nil {
			return false
		}
		secs, err = strconv.ParseInt(string(data), 10, 64)
		return err == nil
	}, 100*time.Millisecond, 5*time.Millisecond)
	assert.InDelta(t, time.Now().Unix(), secs, 2.0)
}

// TestDisabled verifies that when telemetry.offlinereporter.enabled=false,
// SendOfflineDuration is a no-op even when a previous heartbeat file exists.
// NewComponent skips registering lifecycle hooks, so onStart (and readLastHeartbeat)
// are never called.
func TestDisabled(t *testing.T) {
	fs, opts := newTestOptions(t, false)
	pastTs := time.Now().Add(-10 * time.Second).Unix()
	require.NoError(t, afero.WriteFile(fs, testFilePath, []byte(strconv.FormatInt(pastTs, 10)), 0600))

	deps := fxutil.Test[testDeps](t, opts)

	deps.Reporter.SendOfflineDuration("test.offline", nil)

	_, timed := deps.Demux.WaitForSamples(50 * time.Millisecond)
	assert.Empty(t, timed, "disabled component should not send samples")
}

// TestOnStop_StopsLoop verifies the background goroutine exits cleanly on shutdown.
// fxutil.Test registers app.RequireStop() as a cleanup function; if OnStop blocks,
// the test runner will time out.
func TestOnStop_StopsLoop(t *testing.T) {
	_, opts := newTestOptions(t, true)
	fxutil.Test[testDeps](t, opts)
}
