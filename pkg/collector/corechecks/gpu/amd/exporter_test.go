// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && test

package amd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// fakeExporter serves the /gpumetrics response set by the test.
type fakeExporter struct {
	mu     sync.Mutex
	body   string
	status int
}

func (f *fakeExporter) set(status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body = status, body
}

func (f *fakeExporter) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.WriteHeader(f.status)
	fmt.Fprint(w, f.body)
}

// exporterBody returns a /gpumetrics response for the fake AMD host GPU.
func exporterBody(read0, write0, read1, counter, ppt, thermal uint64) string {
	return fmt.Sprintf(`{"Response": [{"Status": {"PCIeStatus": {"PCIeBusId": "0000:c1:00.0"}},
		"Stats": {"XGMILinkStats": [{"DataRead": %d, "DataWrite": %d}, {"DataRead": %d, "DataWrite": 18446744073709551615}],
		"ViolationStats": {"CurrentAccumulatedCounter": %d, "PPTResidencyAccumulated": %d, "SocketThermalResidencyAccumulated": %d,
		"HBMThermalResidencyAccumulated": 18446744073709551615}}}]}`, read0, write0, read1, counter, ppt, thermal)
}

// addHivePeer puts the fake AMD host GPU in an XGMI hive of two GPUs.
func addHivePeer(t *testing.T, root string) {
	dev := filepath.Join(root, "bus", "pci", "devices", "0000:c1:00.0")
	hive := filepath.Join(dev, "xgmi_hive_info")
	require.NoError(t, os.MkdirAll(hive, 0o755))
	require.NoError(t, os.Symlink(dev, filepath.Join(hive, "node1")))
	require.NoError(t, os.Symlink(dev, filepath.Join(hive, "node2")))
}

func setupExporterCheck(t *testing.T, sysRoot, url string) (*Check, *mocksender.MockSender) {
	t.Helper()
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	check := newNamedCheck(ExporterCheckName, taggerfxmock.SetupFakeTagger(t), testutil.GetTelemetryMock(t), testutil.GetWorkloadMetaMock(t))
	applyAMDTestSettings(t, map[string]any{"gpu.static_metrics_reporting_interval": 0})
	check.containerProvider = newMockContainerProvider(t, map[int]string{})
	check.sysRoot = sysRoot
	instance := []byte("gpumetrics_url: " + url)
	require.NoError(t, check.Configure(senderManager, integration.FakeConfigHash, instance, []byte{}, "test", "provider"))
	t.Cleanup(func() { check.Cancel() })
	s := mocksender.NewMockSenderWithSenderManager(check.ID(), senderManager)
	s.SetupAcceptAll()
	return check, s
}

// emittedByName returns the emitted gauges, keyed by name and sample tags.
func emittedByName(s *mocksender.MockSender) map[string]float64 {
	got := map[string]float64{}
	for _, call := range s.Calls {
		if call.Method != "GaugeWithTimestamp" {
			continue
		}
		var key strings.Builder
		key.WriteString(call.Arguments.String(0))
		for _, tag := range call.Arguments.Get(3).([]string) {
			if strings.HasPrefix(tag, "xgmi_link:") || strings.HasPrefix(tag, "reason:") {
				key.WriteString("{" + tag + "}")
			}
		}
		got[key.String()] = call.Arguments.Get(1).(float64)
	}
	return got
}

func TestExporterCheckXGMIAndThrottling(t *testing.T) {
	root := fakeAMDHost(t)
	addHivePeer(t, root)
	exporter := &fakeExporter{}
	srv := httptest.NewServer(exporter)
	defer srv.Close()
	check, s := setupExporterCheck(t, root, srv.URL+"/gpumetrics")

	exporter.set(http.StatusOK, exporterBody(1000, 500, 0, 1000, 100, 0))
	require.NoError(t, check.Run())
	assert.Empty(t, emittedByName(s), "rates and residencies need a second reading")

	exporter.set(http.StatusOK, exporterBody(1000+30*1024, 500+15*1024, 0, 2000, 350, 1500))
	s.ResetCalls()
	require.NoError(t, check.Run())
	got := emittedByName(s)

	for _, name := range []string{"gpu.gr_engine_active", "gpu.memory.limit", "gpu.errors.amdgpu.total"} {
		assert.NotContains(t, got, name, "the exporter check does not report sysfs metrics")
	}
	assert.Equal(t, float64(25), got["gpu.throttle.residency{reason:ppt}"])
	assert.Equal(t, float64(100), got["gpu.throttle.residency{reason:socket_thermal}"], "residency is capped at 100")
	assert.NotContains(t, got, "gpu.throttle.residency{reason:hbm_thermal}", "unsupported counters are skipped")
	assert.Contains(t, got, "gpu.throttle.residency{reason:vr_thermal}", "omitted counters are 0")

	// The rates depend on the time between runs; check the ratios and presence.
	rx0, tx0 := got["gpu.xgmi.throughput.raw.rx{xgmi_link:0}"], got["gpu.xgmi.throughput.raw.tx{xgmi_link:0}"]
	require.Positive(t, rx0)
	assert.InDelta(t, 2, rx0/tx0, 0.001)
	assert.Equal(t, float64(0), got["gpu.xgmi.throughput.raw.rx{xgmi_link:1}"])
	assert.NotContains(t, got, "gpu.xgmi.throughput.raw.tx{xgmi_link:1}", "unsupported counters are skipped")
	assert.InDelta(t, rx0, got["gpu.xgmi.throughput.raw.rx.total"], 0.001)
	assert.InDelta(t, tx0, got["gpu.xgmi.throughput.raw.tx.total"], 0.001)
}

func TestExporterCheckSkipsXGMIWithoutHivePeers(t *testing.T) {
	exporter := &fakeExporter{}
	srv := httptest.NewServer(exporter)
	defer srv.Close()
	check, s := setupExporterCheck(t, fakeAMDHost(t), srv.URL)

	exporter.set(http.StatusOK, exporterBody(1000, 500, 0, 1000, 100, 0))
	require.NoError(t, check.Run())
	exporter.set(http.StatusOK, exporterBody(2000, 900, 0, 2000, 200, 0))
	s.ResetCalls()
	require.NoError(t, check.Run())
	for name := range emittedByName(s) {
		assert.NotContains(t, name, "xgmi", "a GPU alone in its hive has no XGMI traffic to report")
	}
	assert.Contains(t, emittedByName(s), "gpu.throttle.residency{reason:ppt}")
}

func TestExporterCheckUnavailableExporter(t *testing.T) {
	exporter := &fakeExporter{}
	srv := httptest.NewServer(exporter)
	defer srv.Close()
	check, s := setupExporterCheck(t, fakeAMDHost(t), srv.URL)

	exporter.set(http.StatusServiceUnavailable, "")
	require.NoError(t, check.Run(), "an unavailable exporter does not fail the check")
	assert.Empty(t, emittedByName(s))

	exporter.set(http.StatusOK, `{"some": "other document"}`)
	require.NoError(t, check.Run())
	assert.Empty(t, emittedByName(s))
}

func TestExporterCheckRequiresURL(t *testing.T) {
	check := newNamedCheck(ExporterCheckName, taggerfxmock.SetupFakeTagger(t), testutil.GetTelemetryMock(t), testutil.GetWorkloadMetaMock(t))
	applyAMDTestSettings(t, nil)
	err := check.Configure(mocksender.CreateDefaultDemultiplexer(t), integration.FakeConfigHash, []byte("timeout: 3"), []byte{}, "test", "provider")
	assert.ErrorContains(t, err, "gpumetrics_url")
}

func TestExporterMetricsAndTagsMatchSpec(t *testing.T) {
	metricsSpec, err := gpuspec.LoadMetricsSpec()
	require.NoError(t, err)
	for _, name := range []string{"xgmi.throughput.raw.rx", "xgmi.throughput.raw.tx", "xgmi.throughput.raw.rx.total", "xgmi.throughput.raw.tx.total", "throttle.residency"} {
		assert.Contains(t, metricsSpec.Metrics, name)
	}
	tagsSpec, err := gpuspec.LoadTagsSpec()
	require.NoError(t, err)
	for _, reason := range throttleReasons {
		assert.Regexp(t, tagsSpec.Tags["reason"].Regex, reason.tag)
	}
	assert.Regexp(t, tagsSpec.Tags["xgmi_link"].Regex, "7")
}

func TestExporterCheckInstancesHaveDistinctIDs(t *testing.T) {
	root := fakeAMDHost(t)
	a, _ := setupExporterCheck(t, root, "http://10.0.0.1:5000/gpumetrics")
	b, _ := setupExporterCheck(t, root, "http://10.0.0.2:5000/gpumetrics")
	assert.NotEqual(t, a.ID(), b.ID())
}

// Autodiscovery creates a new instance when the exporter is rescheduled, and
// several endpoints can be configured: instances share one telemetry
// registry, which rejects duplicate registrations.
func TestExporterCheckInstancesShareTelemetry(t *testing.T) {
	tm := testutil.GetTelemetryMock(t)
	tagger := taggerfxmock.SetupFakeTagger(t)
	wmeta := testutil.GetWorkloadMetaMock(t)
	applyAMDTestSettings(t, nil)
	root := fakeAMDHost(t)
	for _, url := range []string{"http://10.0.0.1:5000/gpumetrics", "http://10.0.0.2:5000/gpumetrics"} {
		check := newNamedCheck(ExporterCheckName, tagger, tm, wmeta)
		check.containerProvider = newMockContainerProvider(t, map[int]string{})
		check.sysRoot = root
		require.NotPanics(t, func() {
			require.NoError(t, check.Configure(mocksender.CreateDefaultDemultiplexer(t), integration.FakeConfigHash, []byte("gpumetrics_url: "+url), []byte{}, "test", "provider"))
		})
		check.Cancel()
	}
}

func TestExporterMetricsCarryDeviceTagsOnly(t *testing.T) {
	assert.True(t, deviceMetric("throttle.residency", 1, nil).NoWorkloadTags())
	assert.True(t, rateMetric("xgmi.throughput.raw.rx", 1, nil).NoWorkloadTags())
}
