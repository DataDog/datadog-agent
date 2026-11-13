// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && test

package tpu

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

const testChipID = "5197990447957134550-0"

// fakeTPUHost returns a sysfs root with one TPU v6e chip.
func fakeTPUHost(t *testing.T) string {
	root := t.TempDir()
	dir := filepath.Join(root, "bus", "pci", "devices", "0000:00:04.0")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor"), []byte("0x1ae0\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "device"), []byte("0x006f\n"), 0o644))
	return root
}

// serveFixture serves a device plugin scrape from testdata.
func serveFixture(t *testing.T, name string) string {
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) }))
	t.Cleanup(srv.Close)
	return srv.URL + "/metrics"
}

func enableTPU(t *testing.T, tpuEnabled bool) {
	gpu.WithGPUConfigEnabled(t)
	previous := pkgconfigsetup.Datadog().Get("gpu.tpu.enabled")
	t.Cleanup(func() { pkgconfigsetup.Datadog().SetInTest("gpu.tpu.enabled", previous) })
	pkgconfigsetup.Datadog().SetInTest("gpu.tpu.enabled", tpuEnabled)
}

// setupTPUCheck configures a TPU check scraping endpoint. The settings are applied
// after the component mocks are created, as creating them resets the global configuration.
func setupTPUCheck(t *testing.T, endpoint string, settings map[string]any) (*Check, *mocksender.MockSender, workloadmetamock.Mock, taggermock.Mock) {
	t.Helper()
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	c, ok := newCheck(fakeTagger, testutil.GetTelemetryMock(t), testutil.GetWorkloadMetaMock(t)).(*Check)
	require.True(t, ok)
	enableTPU(t, true)
	for key, value := range settings {
		previous := pkgconfigsetup.Datadog().Get(key)
		t.Cleanup(func() { pkgconfigsetup.Datadog().SetInTest(key, previous) })
		pkgconfigsetup.Datadog().SetInTest(key, value)
	}
	c.sysRoot = fakeTPUHost(t)
	c.endpoint = endpoint
	require.NoError(t, c.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test", "provider"))
	t.Cleanup(func() { c.Cancel() })
	snd := mocksender.NewMockSenderWithSenderManager(c.ID(), senderManager)
	snd.SetupAcceptAll()
	wmetaMock, ok := c.wmeta.(workloadmetamock.Mock)
	require.True(t, ok)
	return c, snd, wmetaMock, fakeTagger
}

// gauges returns the gauges sent by the check, keyed by metric name.
func gauges(snd *mocksender.MockSender) map[string][]mock.Call {
	out := map[string][]mock.Call{}
	for _, call := range snd.Mock.Calls {
		if call.Method == "GaugeWithTimestamp" {
			out[call.Arguments.String(0)] = append(out[call.Arguments.String(0)], call)
		}
	}
	return out
}

func TestParseDevicePluginMetrics(t *testing.T) {
	data, err := os.ReadFile("testdata/device_plugin_busy.txt")
	require.NoError(t, err)
	chips, err := parseDevicePluginMetrics(data)
	require.NoError(t, err)
	require.Len(t, chips, 1)
	ch := chips[0]
	assert.Equal(t, testChipID, ch.id)
	assert.InDelta(t, 80.397, ch.node["tensorcore_utilization"], 0.01)
	assert.InDelta(t, 92.585, ch.node["duty_cycle"], 0.01)
	assert.InDelta(t, 3.3550237184e+10, ch.node["memory_total"], 1)
	key := workloadKey{namespace: "default", pod: "jax-prof", container: "jax"}
	require.Contains(t, ch.workloads, key)
	assert.InDelta(t, 1.35183872e+08, ch.workloads[key]["memory_used"], 1)
}

func TestDeviceTagsMatchTagSpec(t *testing.T) {
	tagsSpec, err := gpuspec.LoadTagsSpec()
	require.NoError(t, err)
	tags := deviceTags(testChipID, "tpu-v6e")
	assert.Subset(t, tags, []string{"gpu_vendor:google", "gpu_uuid:" + testChipID, "gpu_type:tpu-v6e", "gpu_slicing_mode:none"})
	for _, tag := range tags {
		name, value, found := strings.Cut(tag, ":")
		require.True(t, found, tag)
		spec, ok := tagsSpec.Tags[name]
		require.True(t, ok, "tag %s is not in the spec", name)
		if spec.Regex != nil {
			assert.Regexp(t, spec.Regex, value, "tag %s", name)
		}
	}
}

// nodeSampleValues returns the values of the device-level samples of a chip, by name.
func nodeSampleValues(ch *chip) map[string]float64 {
	out := map[string]float64{}
	for _, s := range nodeSamples(ch, true) {
		m := s.(*nvidia.Metric)
		out[m.Name] = m.Value
	}
	return out
}

func TestEngineActivityFromPartialScrapes(t *testing.T) {
	busy := workloadKey{namespace: "default", pod: "p", container: "c"}
	t.Run("no duty cycle while a workload runs is not idle", func(t *testing.T) {
		ch := &chip{node: map[string]float64{"tensorcore_utilization": 80}, workloads: map[workloadKey]map[string]float64{busy: {}}}
		assert.NotContains(t, nodeSampleValues(ch), "gr_engine_active")
	})
	t.Run("busy TensorCores without duty cycle are not idle", func(t *testing.T) {
		ch := &chip{node: map[string]float64{"tensorcore_utilization": 80}, workloads: map[workloadKey]map[string]float64{}}
		assert.NotContains(t, nodeSampleValues(ch), "gr_engine_active")
	})
	t.Run("duty cycle alone", func(t *testing.T) {
		ch := &chip{node: map[string]float64{"duty_cycle": 42}, workloads: map[workloadKey]map[string]float64{}}
		assert.Equal(t, 42.0, nodeSampleValues(ch)["gr_engine_active"])
	})
}

func TestBusyChipEmitsDeviceAndWorkloadMetrics(t *testing.T) {
	c, snd, wmetaMock, fakeTagger := setupTPUCheck(t, serveFixture(t, "device_plugin_busy.txt"), nil)
	wmetaMock.Set(&workloadmeta.KubernetesPod{
		EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-uid"},
		EntityMeta: workloadmeta.EntityMeta{Name: "jax-prof", Namespace: "default"},
		Containers: []workloadmeta.OrchestratorContainer{{ID: "ctr-jax", Name: "jax"}},
	})
	wmetaMock.Set(&workloadmeta.Container{
		EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "ctr-jax"},
		EntityMeta: workloadmeta.EntityMeta{Name: "jax"},
	})
	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.ContainerID, "ctr-jax"), "fake", []string{"container_id:ctr-jax"}, nil, nil, nil)

	require.NoError(t, c.Run())

	got := gauges(snd)
	for _, name := range []string{"gpu.device.total", "gpu.sm_active", "gpu.gr_engine_active", "gpu.dram_active", "gpu.memory.limit", "gpu.memory.free", "gpu.memory.utilization", "gpu.process.memory.usage", "gpu.process.sm_active", "gpu.process.dram_active"} {
		assert.Contains(t, got, name)
	}
	require.NotEmpty(t, got["gpu.process.memory.usage"])
	tags := got["gpu.process.memory.usage"][0].Arguments.Get(3).([]string)
	assert.Contains(t, tags, "container_id:ctr-jax")
	assert.Contains(t, tags, "gpu_uuid:"+testChipID)
	// Device metrics carry the tags of the container allocated the chip.
	require.NotEmpty(t, got["gpu.sm_active"])
	assert.Contains(t, got["gpu.sm_active"][0].Arguments.Get(3).([]string), "container_id:ctr-jax")
	assert.InDelta(t, 80.397, got["gpu.sm_active"][0].Arguments.Get(1).(float64), 0.01)
}

func TestIdleChipReportsZeroActivity(t *testing.T) {
	c, snd, _, _ := setupTPUCheck(t, serveFixture(t, "device_plugin_idle.txt"), nil)
	require.NoError(t, c.Run())

	got := gauges(snd)
	require.Len(t, got["gpu.device.total"], 1)
	require.Len(t, got["gpu.gr_engine_active"], 1)
	assert.Zero(t, got["gpu.gr_engine_active"][0].Arguments.Get(1).(float64))
	assert.Contains(t, got, "gpu.sm_active")
	assert.NotContains(t, got, "gpu.memory.limit")
	assert.NotContains(t, got, "gpu.process.memory.usage")
}

func TestOneMemoryLimitForAllWorkloadsOfAChip(t *testing.T) {
	const fixture = `memory_total_node{accelerator_id="1-0",model="tpu-v6e-slice"} 100
memory_used_node{accelerator_id="1-0",model="tpu-v6e-slice"} 30
memory_used{accelerator_id="1-0",model="tpu-v6e-slice",namespace="ns",pod="a",container="c"} 10
memory_total{accelerator_id="1-0",model="tpu-v6e-slice",namespace="ns",pod="a",container="c"} 100
memory_used{accelerator_id="1-0",model="tpu-v6e-slice",namespace="ns",pod="b",container="c"} 20
memory_total{accelerator_id="1-0",model="tpu-v6e-slice",namespace="ns",pod="b",container="c"} 100
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(fixture)) }))
	t.Cleanup(srv.Close)
	c, snd, wmetaMock, fakeTagger := setupTPUCheck(t, srv.URL+"/metrics", nil)
	for _, pod := range []string{"a", "b"} {
		wmetaMock.Set(&workloadmeta.KubernetesPod{
			EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "uid-" + pod},
			EntityMeta: workloadmeta.EntityMeta{Name: pod, Namespace: "ns"},
			Containers: []workloadmeta.OrchestratorContainer{{ID: "ctr-" + pod, Name: "c"}},
		})
		wmetaMock.Set(&workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "ctr-" + pod}, EntityMeta: workloadmeta.EntityMeta{Name: "c"}})
		fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.ContainerID, "ctr-"+pod), "fake", []string{"container_id:ctr-" + pod}, nil, nil, nil)
	}
	require.NoError(t, c.Run())

	limits := gauges(snd)["gpu.memory.limit"]
	require.Len(t, limits, 1)
	assert.Equal(t, 100.0, limits[0].Arguments.Get(1).(float64))
	assert.Subset(t, limits[0].Arguments.Get(3).([]string), []string{"container_id:ctr-a", "container_id:ctr-b"})
	assert.Len(t, gauges(snd)["gpu.process.memory.usage"], 2)
}

func TestExcludedChipIsNotReported(t *testing.T) {
	c, snd, _, _ := setupTPUCheck(t, serveFixture(t, "device_plugin_idle.txt"), map[string]any{"gpu.excluded_devices": []string{testChipID}})
	require.NoError(t, c.Run())
	assert.Empty(t, gauges(snd))
}

func TestScrapeErrorFailsRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	t.Cleanup(srv.Close)
	c, _, _, _ := setupTPUCheck(t, srv.URL+"/metrics", nil)
	assert.ErrorContains(t, c.Run(), "503")
}

func TestCheckSkippedWithoutConfigOrTPU(t *testing.T) {
	configure := func(c *Check) error {
		return c.Configure(mocksender.CreateDefaultDemultiplexer(t), integration.FakeConfigHash, []byte{}, []byte{}, "test", "provider")
	}
	newTestCheck := func() *Check {
		c, ok := newCheck(taggerfxmock.SetupFakeTagger(t), testutil.GetTelemetryMock(t), testutil.GetWorkloadMetaMock(t)).(*Check)
		require.True(t, ok)
		return c
	}

	t.Run("disabled", func(t *testing.T) {
		c := newTestCheck()
		enableTPU(t, false)
		c.sysRoot = fakeTPUHost(t)
		assert.ErrorIs(t, configure(c), check.ErrSkipCheckInstance)
	})
	t.Run("endpoint from the kubelet host", func(t *testing.T) {
		for host, endpoint := range map[string]string{"10.0.0.5": "http://10.0.0.5:2112/metrics", "fd00::5": "http://[fd00::5]:2112/metrics"} {
			c := newTestCheck()
			enableTPU(t, true)
			previous := pkgconfigsetup.Datadog().Get("kubernetes_kubelet_host")
			t.Cleanup(func() { pkgconfigsetup.Datadog().SetInTest("kubernetes_kubelet_host", previous) })
			pkgconfigsetup.Datadog().SetInTest("kubernetes_kubelet_host", host)
			c.sysRoot = fakeTPUHost(t)
			require.NoError(t, configure(c))
			assert.Equal(t, endpoint, c.endpoint)
		}
	})
	t.Run("no TPU", func(t *testing.T) {
		c := newTestCheck()
		enableTPU(t, true)
		c.sysRoot = t.TempDir()
		assert.ErrorIs(t, configure(c), check.ErrSkipCheckInstance)
	})
}

// VBAR GetHostMetrics captured on a TPU v6e VM with the chip idle.
const vbarIdleHex = "0a89010a0b08ba97dad70610e0c2e31a1211080215000000001d0000000025000000001211080315000000001d0000000025000000001211080415000000001d0000000025000000001211080115000000001d0000000025000000001a2e415a50515539357149697568614237614443755365324d4e4b6468433059435852646c5a563154566a594f785777"

func TestNoIdleInferenceFromCachedVBAR(t *testing.T) {
	c, snd, _ := setupHostModeCheck(t, false, vbarIdleHex)
	require.NoError(t, c.Run())
	got := gauges(snd)
	require.NotEmpty(t, got["gpu.sm_active"])
	assert.Zero(t, got["gpu.sm_active"][0].Arguments.Get(1).(float64))
	assert.NotContains(t, got, "gpu.gr_engine_active", "without runtime metrics, an idle VBAR sample does not make the chip idle")
}

func TestMultiHostRuntimeIndicesMapToHostChips(t *testing.T) {
	// Host series number the chips of the node from 0; runtime series of multi-host
	// slices number them across the slice (ray-project/ray#63998).
	var fixture strings.Builder
	for i, global := range []int{10, 11, 14, 15} {
		fmt.Fprintf(&fixture, "tensorcore_utilization_node{accelerator_id=\"7-%d\"} %d\n", i, 10*i)
		fmt.Fprintf(&fixture, "duty_cycle_node{accelerator_id=\"7-%d\"} %d\n", global, 50+i)
	}
	chips, err := parseDevicePluginMetrics([]byte(fixture.String()))
	require.NoError(t, err)
	require.Len(t, chips, 4)
	for _, ch := range chips {
		i, err := strconv.Atoi(strings.TrimPrefix(ch.id, "7-"))
		require.NoError(t, err)
		assert.Equal(t, float64(10*i), ch.node["tensorcore_utilization"], ch.id)
		assert.Equal(t, float64(50+i), ch.node["duty_cycle"], ch.id)
	}
	// Unmatched runtime chips are dropped rather than reported as more chips.
	chips, err = parseDevicePluginMetrics([]byte("tensorcore_utilization_node{accelerator_id=\"7-0\"} 1\nduty_cycle_node{accelerator_id=\"7-10\"} 2\nduty_cycle_node{accelerator_id=\"7-11\"} 3\n"))
	require.NoError(t, err)
	require.Len(t, chips, 1)
	assert.NotContains(t, chips[0].node, "duty_cycle")
}

// Responses captured on a GKE TPU v6e node: VBAR GetHostMetrics with the chip busy
// (TensorCore 79.2%, bandwidth 58.1%), and libtpu runtime metrics of the workload.
const vbarBusyHex = "0a8a010a0c08bbcbdad70610a991f9e3011211080215774168421dbdea2d3f25000000001211080315000000001d0000000025000000001211080415000000001d00000000250000000012110801156b529e421d1d447c3f250000803f1a2e415a5051553935795f773365434d66713749494730704b79365a7165644b6c436c3936667547725565796d333077"

var runtimeHex = map[string]string{
	"tpu.runtime.hbm.memory.total.bytes":       "0a4d0a227470752e72756e74696d652e68626d2e6d656d6f72792e746f74616c2e62797465731a270a0f0a096465766963652d696412021800120c0885cadad70610f2a2b3bd011a061080fcfffd7c1801",
	"tpu.runtime.hbm.memory.usage.bytes":       "0a4c0a227470752e72756e74696d652e68626d2e6d656d6f72792e75736167652e62797465731a260a0f0a096465766963652d696412021800120c0885cadad70610a69fb3bd011a051080fcba401801",
	"tpu.runtime.tensorcore.dutycycle.percent": "0a560a287470752e72756e74696d652e74656e736f72636f72652e647574796379636c652e70657263656e741a2a0a0f0a096465766963652d696412021800120c0885cadad70610e29796bd011a0909e4c237da0b0657401801",
}

func mustHex(t *testing.T, s string) []byte {
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

// serveTPUHost serves the VBAR and libtpu services on one address. Without
// runtime, the libtpu service is unavailable, as when no workload runs.
func serveTPUHost(t *testing.T, runtime bool, vbarHex string) (addr string, vbarCalls *int) {
	vbarCalls = new(int)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer(grpc.ForceServerCodec(rawCodec{}), grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		method, _ := grpc.MethodFromServerStream(stream)
		var req []byte
		if err := stream.RecvMsg(&req); err != nil {
			return err
		}
		if strings.HasSuffix(method, "/GetHostMetrics") {
			*vbarCalls++
			if vbarHex == "" {
				return status.Error(codes.Unavailable, "device reset")
			}
			return stream.SendMsg(mustHex(t, vbarHex))
		}
		if !runtime {
			return status.Error(codes.Unavailable, "no workload")
		}
		return stream.SendMsg(mustHex(t, runtimeHex[string(req[2:])]))
	}))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String(), vbarCalls
}

func TestHostSourceParsesCapturedResponses(t *testing.T) {
	chips := parseHostMetrics(mustHex(t, vbarBusyHex))
	require.Len(t, chips, 1)
	assert.InDelta(t, 79.16, chips[0]["tensorcore_utilization"], 0.01)
	assert.InDelta(t, 58.06, chips[0]["memory_bandwidth_utilization"], 0.01)
	assert.Equal(t, map[int]float64{0: 33550237184}, parseRuntimeMetric(mustHex(t, runtimeHex["tpu.runtime.hbm.memory.total.bytes"])))
	assert.InDelta(t, 92.09, parseRuntimeMetric(mustHex(t, runtimeHex["tpu.runtime.tensorcore.dutycycle.percent"]))[0], 0.01)
	assert.Empty(t, parseHostMetrics([]byte{0xff}))
	assert.Empty(t, parseHostMetrics([]byte{0x0a, 0x01, 0xff}), "a malformed chip entry is not a chip")
	assert.Len(t, parseHostMetrics([]byte{0x0a, 0x03, 0x1a, 0x01, 'x'}), 1, "a chip without metrics keeps its index")
}

// setupHostModeCheck configures a TPU check reading fake TPU VM host services.
func setupHostModeCheck(t *testing.T, workload bool, vbarHex string) (*Check, *mocksender.MockSender, *int) {
	addr, vbarCalls := serveTPUHost(t, workload, vbarHex)
	metadata := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Google", r.Header.Get("Metadata-Flavor"))
		_, _ = w.Write([]byte("5518188025174767349\n"))
	}))
	t.Cleanup(metadata.Close)
	host, err := newHostSource(metadata.URL, addr, addr)
	require.NoError(t, err)
	c, ok := newCheck(taggerfxmock.SetupFakeTagger(t), testutil.GetTelemetryMock(t), testutil.GetWorkloadMetaMock(t)).(*Check)
	require.True(t, ok)
	enableTPU(t, true)
	c.sysRoot, c.host = fakeTPUHost(t), host
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	require.NoError(t, c.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test", "provider"))
	t.Cleanup(c.Cancel)
	snd := mocksender.NewMockSenderWithSenderManager(c.ID(), senderManager)
	snd.SetupAcceptAll()
	return c, snd, vbarCalls
}

func TestFailedVBARPollIsNotRetriedEveryRun(t *testing.T) {
	c, _, vbarCalls := setupHostModeCheck(t, true, "")
	require.Error(t, c.Run())
	require.Error(t, c.Run(), "the poll error is reported until the next poll")
	assert.Equal(t, 1, *vbarCalls)
}

func TestHostModeOnTPUVM(t *testing.T) {
	for _, workload := range []bool{true, false} {
		c, snd, vbarCalls := setupHostModeCheck(t, workload, vbarBusyHex)

		require.NoError(t, c.Run())
		require.NoError(t, c.Run())
		assert.Equal(t, 1, *vbarCalls, "VBAR is polled at most every %s", vbarPollInterval)
		got := gauges(snd)
		require.NotEmpty(t, got["gpu.sm_active"])
		assert.Contains(t, got["gpu.sm_active"][0].Arguments.Get(3).([]string), "gpu_uuid:5518188025174767349-0")
		assert.Contains(t, got["gpu.sm_active"][0].Arguments.Get(3).([]string), "gpu_type:tpu-v6e")
		if workload {
			assert.InDelta(t, 92.09, got["gpu.gr_engine_active"][0].Arguments.Get(1).(float64), 0.01)
			assert.Contains(t, got, "gpu.memory.limit")
		} else {
			// Busy TensorCores without runtime metrics: the duty cycle is unknown, not 0.
			assert.NotContains(t, got, "gpu.gr_engine_active")
			assert.NotContains(t, got, "gpu.memory.limit")
		}
	}
}
