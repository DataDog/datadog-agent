// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package offlinereporterimpl

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const testFilePath = "/run/agent_heartbeat"

// mockDemux captures SendSamplesWithoutAggregation calls for assertions.
type mockDemux struct {
	samples []metrics.MetricSampleBatch
}

func (m *mockDemux) SendSamplesWithoutAggregation(batch metrics.MetricSampleBatch) {
	m.samples = append(m.samples, batch)
}

type mockHostname struct{ name string }

func (m *mockHostname) GetSafe(_ context.Context) string { return m.name }

// newHarness builds an offlinereporterImpl directly (bypassing NewComponent and fx)
// so tests can inject an afero.MemMapFs, a minimal sampleSender mock, and a
// hostname mock.
func newHarness(t *testing.T, fs afero.Fs, demux sampleSender, hn hostnameGetter) (*offlinereporterImpl, *compdef.TestLifecycle) {
	t.Helper()
	lc := compdef.NewTestLifecycle(t)
	h := &offlinereporterImpl{
		log:               logmock.New(t),
		fs:                fs,
		filePath:          testFilePath,
		heartbeatInterval: 5 * time.Second,
		demux:             demux,
		hostnameComp:      hn,
		stopChan:          make(chan struct{}),
	}
	lc.Append(compdef.Hook{
		OnStart: func(ctx context.Context) error { return h.onStart(ctx) },
		OnStop:  func(_ context.Context) error { h.stopChan <- struct{}{}; return nil },
	})
	return h, lc
}

// TestFirstRun verifies SendOfflineDuration is a no-op when no previous file exists.
func TestFirstRun(t *testing.T) {
	fs := afero.NewMemMapFs()
	demux := &mockDemux{}

	h, lc := newHarness(t, fs, demux, &mockHostname{"host"})
	ctx := context.Background()
	require.NoError(t, lc.Start(ctx))
	defer lc.Stop(ctx) //nolint:errcheck

	h.SendOfflineDuration("test.offline", nil)
	assert.Empty(t, demux.samples, "expected no samples on first run")
}

// TestSecondRun verifies SendOfflineDuration sends a gauge ≈ seconds since
// the previous heartbeat.
func TestSecondRun(t *testing.T) {
	fs := afero.NewMemMapFs()
	demux := &mockDemux{}

	// Simulate a previous run that finished 10 seconds ago.
	pastTs := time.Now().Add(-10 * time.Second).Unix()
	require.NoError(t, afero.WriteFile(fs, testFilePath, []byte(strconv.FormatInt(pastTs, 10)), 0600))

	h, lc := newHarness(t, fs, demux, &mockHostname{"myhost"})
	ctx := context.Background()
	require.NoError(t, lc.Start(ctx))
	defer lc.Stop(ctx) //nolint:errcheck

	h.SendOfflineDuration("test.offline", []string{"env:test"})

	require.Len(t, demux.samples, 1)
	require.Len(t, demux.samples[0], 1)
	sample := demux.samples[0][0]
	assert.Equal(t, "test.offline", sample.Name)
	assert.Equal(t, "myhost", sample.Host)
	assert.Equal(t, metrics.GaugeType, sample.Mtype)
	assert.InDelta(t, 10.0, sample.Value, 2.0, "offline duration should be ~10s")
}

// TestCorruptFile verifies that a corrupt heartbeat file is treated as a first run.
func TestCorruptFile(t *testing.T) {
	fs := afero.NewMemMapFs()
	demux := &mockDemux{}

	require.NoError(t, afero.WriteFile(fs, testFilePath, []byte("not-a-timestamp"), 0600))

	h, lc := newHarness(t, fs, demux, &mockHostname{"host"})
	ctx := context.Background()
	require.NoError(t, lc.Start(ctx))
	defer lc.Stop(ctx) //nolint:errcheck

	h.SendOfflineDuration("test.offline", nil)
	assert.Empty(t, demux.samples, "corrupt file should be treated as first run")
}

// TestOnStart_WritesFile verifies the heartbeat file is created on startup.
func TestOnStart_WritesFile(t *testing.T) {
	fs := afero.NewMemMapFs()
	demux := &mockDemux{}

	_, lc := newHarness(t, fs, demux, &mockHostname{"host"})
	ctx := context.Background()
	require.NoError(t, lc.Start(ctx))
	defer lc.Stop(ctx) //nolint:errcheck

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

// TestDisabled verifies that when lifecycle hooks are not registered (simulating
// telemetry.offlinereporter.enabled=false), SendOfflineDuration is a no-op even when
// a previous heartbeat file exists.
func TestDisabled(t *testing.T) {
	fs := afero.NewMemMapFs()
	demux := &mockDemux{}

	// Simulate a previous run that finished 10 seconds ago.
	pastTs := time.Now().Add(-10 * time.Second).Unix()
	require.NoError(t, afero.WriteFile(fs, testFilePath, []byte(strconv.FormatInt(pastTs, 10)), 0600))

	// Build the impl directly without registering lifecycle hooks, mirroring
	// what NewComponent does when telemetry.offlinereporter.enabled=false.
	h := &offlinereporterImpl{
		log:               logmock.New(t),
		fs:                fs,
		filePath:          testFilePath,
		heartbeatInterval: 5 * time.Second,
		demux:             demux,
		hostnameComp:      &mockHostname{"host"},
		stopChan:          make(chan struct{}),
	}

	h.SendOfflineDuration("test.offline", nil)
	assert.Empty(t, demux.samples, "disabled heartbeat should not send samples")
}

// TestOnStop_StopsLoop verifies the background goroutine exits cleanly.
// If OnStop blocks forever the test runner will time out.
func TestOnStop_StopsLoop(t *testing.T) {
	fs := afero.NewMemMapFs()
	demux := &mockDemux{}

	_, lc := newHarness(t, fs, demux, &mockHostname{"host"})
	ctx := context.Background()
	require.NoError(t, lc.Start(ctx))
	require.NoError(t, lc.Stop(ctx))
}
