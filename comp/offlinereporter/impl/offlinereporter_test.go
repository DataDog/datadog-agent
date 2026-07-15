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

	"github.com/benbjohnson/clock"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	demultiplexer "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	demultiplexerimpl "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/impl"
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

func newTestOptions(t *testing.T, enabled bool) (afero.Fs, *clock.Mock, fx.Option) {
	fs := afero.NewMemMapFs()
	clk := clock.NewMock()
	return fs, clk, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			return config.NewMockWithOverrides(t, map[string]interface{}{
				"telemetry.offlinereporter.enabled":            enabled,
				"telemetry.offlinereporter.heartbeat_interval": "5s",
				"run_path": testRunPath,
			})
		}),
		fx.Supply(Params{Fs: fs, Clk: clk}),
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
	_, _, opts := newTestOptions(t, true)
	deps := fxutil.Test[testDeps](t, opts)

	deps.Reporter.SendOfflineDuration("test.offline", nil)

	_, timed := deps.Demux.WaitForSamples(50 * time.Millisecond)
	assert.Empty(t, timed, "expected no samples on first run")
}

// TestSecondRun verifies SendOfflineDuration sends a gauge equal to
// the seconds since the previous heartbeat.
func TestSecondRun(t *testing.T) {
	fs, clk, opts := newTestOptions(t, true)
	pastTs := clk.Now().Add(-10 * time.Second).Unix()
	require.NoError(t, afero.WriteFile(fs, testFilePath, []byte(strconv.FormatInt(pastTs, 10)), 0600))

	deps := fxutil.Test[testDeps](t, opts)
	deps.Reporter.SendOfflineDuration("test.offline", []string{"env:test"})

	_, timed := deps.Demux.WaitForSamples(1 * time.Second)
	require.Len(t, timed, 1)
	sample := timed[0]
	assert.Equal(t, "test.offline", sample.Name)
	assert.Equal(t, "my-hostname", sample.Host)
	assert.Equal(t, metrics.GaugeType, sample.Mtype)
	assert.Equal(t, 10.0, sample.Value)
}

// TestCorruptFile verifies that a corrupt heartbeat file is treated as a first run.
func TestCorruptFile(t *testing.T) {
	fs, _, opts := newTestOptions(t, true)
	require.NoError(t, afero.WriteFile(fs, testFilePath, []byte("not-a-timestamp"), 0600))

	deps := fxutil.Test[testDeps](t, opts)

	deps.Reporter.SendOfflineDuration("test.offline", nil)

	_, timed := deps.Demux.WaitForSamples(50 * time.Millisecond)
	assert.Empty(t, timed, "corrupt file should be treated as first run")
}

// TestOnStart_WritesFile verifies the heartbeat file is created on startup.
func TestOnStart_WritesFile(t *testing.T) {
	fs, clk, opts := newTestOptions(t, true)
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
	assert.Equal(t, clk.Now().Unix(), secs)
}

// TestDisabled verifies that when telemetry.offlinereporter.enabled=false,
// SendOfflineDuration is a no-op even when a previous heartbeat file exists.
// NewComponent skips registering lifecycle hooks, so onStart (and readLastHeartbeat)
// are never called.
func TestDisabled(t *testing.T) {
	fs, clk, opts := newTestOptions(t, false)
	pastTs := clk.Now().Add(-10 * time.Second).Unix()
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
	_, _, opts := newTestOptions(t, true)
	fxutil.Test[testDeps](t, opts)
}

// TestNegativeInterval verifies that when heartbeat_interval is negative, the
// reporter loop does not start even when the component is enabled. NewComponent
// should emit a warning and skip registering the lifecycle hooks.
func TestNegativeInterval(t *testing.T) {
	fs := afero.NewMemMapFs()
	opts := fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			return config.NewMockWithOverrides(t, map[string]interface{}{
				"telemetry.offlinereporter.enabled":            true,
				"telemetry.offlinereporter.heartbeat_interval": "-1s",
				"run_path": testRunPath,
			})
		}),
		fx.Supply(Params{Fs: fs, Clk: clock.NewMock()}),
		fxutil.ProvideComponentConstructor(NewComponent),
		hostnameimpl.MockModule(),
		demultiplexerimpl.FakeSamplerMockModule(),
		logscompressionmock.MockModule(),
		metricscompressionmock.MockModule(),
		fx.Provide(func(m demultiplexer.FakeSamplerMock) demultiplexer.Component {
			return m.(demultiplexer.Component)
		}),
	)
	fxutil.Test[testDeps](t, opts)

	// The lifecycle OnStart hook must not have been registered, so the loop
	// goroutine was never launched and the heartbeat file must not exist.
	_, err := afero.ReadFile(fs, testFilePath)
	assert.Error(t, err, "heartbeat file should not be written when interval is negative")
}
