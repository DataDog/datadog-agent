// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && zlib

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/gogen"
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/accesspoint"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/engine"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/identity"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/output"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/overlay"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/impl"
	softwareimpl "github.com/DataDog/datadog-agent/comp/softwareinventory/impl"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/bazelbuild/rules_go/go/runfiles"
)

// Only the scheduler clock is accelerated. Delivery still traverses real Agent
// codecs, weighted queues, batching, forwarders and tracked transaction completion.
type replayClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *replayClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *replayClock) WaitUntil(ctx context.Context, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if at.After(c.now) {
		c.now = at
	}
	return nil
}

func replayFixture(t *testing.T, platform string) *bundle.Loaded {
	t.Helper()
	read := func(name string) []byte {
		t.Helper()
		relative := "testdata/bundles/" + platform + "/" + name
		path := filepath.Join("..", filepath.FromSlash(relative))
		if os.Getenv("TEST_SRCDIR") != "" {
			r, err := runfiles.New()
			if err != nil {
				t.Fatal(err)
			}
			repository := runfiles.CallerRepository()
			if repository == "" {
				repository = "_main"
			}
			path, err = r.Rlocation(repository + "/cmd/eudm-simulator/" + relative)
			if err != nil {
				t.Fatal(err)
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	var manifest bundle.Manifest
	if err := json.Unmarshal(read("manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	// Bazel input runfiles are symlinks. Materialize them as regular files so
	// the actual bundle loader's no-symlink invariant remains exercised.
	directory := t.TempDir()
	names := []string{"manifest.json", "COMPLETE"}
	for name := range manifest.Files {
		names = append(names, name)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(directory, name), read(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := bundle.Load(directory, fixtureCommit)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func TestSharedBaselineReplayThroughAgentPayloadDelivery(t *testing.T) {
	for _, platform := range []string{"macos", "windows"} {
		t.Run(platform, func(t *testing.T) { testSharedBaselineReplay(t, platform) })
	}
}

// Long replay exercises inventory scheduling from the checked-in bundle with a
// recording delivery boundary. The test above separately checks Agent encoding
// and forwarding; this clock can advance without waiting through real minutes.
type inventoryCycleRecorder struct {
	mu          sync.Mutex
	samples     map[schema.Stream][]*tc.Inventory
	metricTimes map[string][]float64
}

func (d *inventoryCycleRecorder) Send(_ context.Context, _ time.Time, stream schema.Stream, samples []*telemetry.Sample) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, sample := range samples {
		if sample.Inventory != nil {
			d.samples[stream] = append(d.samples[stream], tc.CloneInventory(sample.Inventory))
		}
		for _, serie := range sample.Metrics {
			for _, point := range serie.Points {
				d.metricTimes[serie.Name] = append(d.metricTimes[serie.Name], point.Ts)
			}
		}
	}
	return nil
}

func (*inventoryCycleRecorder) NetworkMetrics(context.Context, []*metrics.Serie) error { return nil }
func (*inventoryCycleRecorder) NetworkMetadata(context.Context, []metadata.NetworkDevicesMetadata) error {
	return nil
}
func (*inventoryCycleRecorder) Wait(context.Context) error { return nil }

func TestRepeatedInventoryAndMetricFamiliesKeepNativeCadences(t *testing.T) {
	for _, platform := range []string{"macos", "windows"} {
		t.Run(platform, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			captured := replayFixture(t, platform)
			start := time.Date(2026, 10, 1, 12, 0, 0, 375000000, time.UTC)
			const duration = 65 * time.Minute
			scenario := &schema.Scenario{
				Version: schema.Version, Meta: schema.ScenarioMeta{Name: "inventory-repeat-validation"},
				Expectation: schema.Expectation{Conclusion: schema.Healthy},
				Fleet:       []schema.GroupDef{{Group: "inventory-repeat-device", OS: platform, Count: 1}},
				Phases:      []schema.Phase{{Name: "healthy", Duration: schema.Duration{Duration: duration}}},
			}
			plan, err := schema.NewPlan(scenario, schema.Digest([]byte("repeated-inventory")), fixtureCommit, 7, start, captured.Ref())
			if err != nil {
				t.Fatal(err)
			}
			recorder := &inventoryCycleRecorder{samples: map[schema.Stream][]*tc.Inventory{}, metricTimes: map[string][]float64{}}
			result, err := engine.Run(ctx, engine.Request{Scenario: scenario, Plan: plan, Bundle: captured}, engine.Options{Workers: 1, QueueCapacity: 2, Clock: &replayClock{now: start}, Delivery: recorder})
			if err != nil || !result.Complete() {
				t.Fatal("repeated inventory replay did not complete", err)
			}
			id := identity.New(plan.RunID, plan.Seed, scenario.Fleet[0].Group, 0)
			for _, stream := range []schema.Stream{schema.AgentInventory, schema.HostInventory, schema.HostSystemInfo} {
				cadence, firstOffset, count := 10*time.Minute, 250*time.Millisecond, 7
				if stream == schema.HostInventory {
					firstOffset = 500 * time.Millisecond
				} else if stream == schema.HostSystemInfo {
					cadence, firstOffset, count = time.Hour, 5250*time.Millisecond, 2
				}
				cycles := recorder.samples[stream]
				if len(cycles) != count {
					t.Fatalf("%s must repeat at its captured %s cadence: got %d cycles", stream, cadence, len(cycles))
				}
				for i, inventory := range cycles {
					if inventory.Hostname != id.Hostname || inventory.UUID != id.UUID || inventory.Timestamp != start.Add(time.Duration(i)*cadence+firstOffset).UnixNano() {
						t.Fatal("repeated inventory identity or collection time diverged")
					}
					if inventory.Agent != nil && inventory.Agent.AgentStartupTimeMS != start.Add(-time.Minute).UnixMilli() {
						t.Fatal("repeating inventory falsely restarted the simulated Agent")
					}
					if info := inventory.SystemInfo; info != nil {
						if info.SerialNumber == "" || info.SerialNumber == "serial_number-"+strings.Repeat("5", 32) || info.SerialNumber != cycles[0].SystemInfo.SerialNumber {
							t.Fatal("repeated system information lost its stable device serial")
						}
					}
				}
			}
			for _, name := range captured.Manifest.Profile.MetricNames {
				cadence := 15 * time.Second
				if tc.MetricFamily(name) == "battery" {
					cadence = 5 * time.Minute
				}
				if captured.Manifest.MetricCadences[tc.MetricFamily(name)] != cadence {
					t.Fatalf("synthetic metric %s lost its declared family cadence", name)
				}
				var expected []float64
				for offset := time.Duration(0); offset < duration; offset += cadence {
					expected = append(expected, float64(start.Unix())+float64(start.Nanosecond())/1e9+offset.Seconds())
				}
				actual := recorder.metricTimes[name]
				slices.Sort(actual)
				if !slices.Equal(actual, expected) {
					t.Fatalf("metric %s did not replay at its native %s cadence: got %d points, wanted %d", name, cadence, len(actual), len(expected))
				}
			}
		})
	}
}

func testSharedBaselineReplay(t *testing.T, platform string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	scenario := &schema.Scenario{
		Version: schema.Version, Meta: schema.ScenarioMeta{Name: "private-shared-baseline-validation"},
		Expectation: schema.Expectation{Conclusion: schema.Healthy},
		Fleet:       []schema.GroupDef{{Group: "private-primary-cohort", OS: platform, Count: 30}, {Group: "private-comparison-cohort", OS: platform, Count: 30}},
		Phases:      []schema.Phase{{Name: "healthy", Duration: schema.Duration{Duration: 31 * time.Second}}},
	}
	capture := replayFixture(t, platform)
	start := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	plan, err := schema.NewPlan(scenario, schema.Digest([]byte("shared-baseline-agent-wire-integration")), fixtureCommit, 7, start, capture.Ref())
	if err != nil {
		t.Fatal(err)
	}
	plan.RunID = "0123456789abcdef0123456789abcdef"
	recorder := output.NewRecorder()
	pipeline, err := output.New(ctx, nil, "synthetic-integration-no-credential", recorder, output.Options{QueueCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer pipeline.Close()
	// Select the supported v2 Agent protocol to inspect its public protobuf
	// fields, including origin metadata, instead of relying on packed v3 bytes.
	pipeline.Config.Set("use_v3_api.series.enabled", "false", configmodel.SourceAgentRuntime)
	result, err := engine.Run(ctx, engine.Request{Scenario: scenario, Plan: plan, Bundle: capture}, engine.Options{Workers: 6, QueueCapacity: 1, Clock: &replayClock{now: start.Add(-time.Second)}, Delivery: engine.AgentDelivery{Pipeline: pipeline}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete() || result.Status != "succeeded" || len(result.Ledger) != 60 || !result.End.Equal(start.Add(31*time.Second)) {
		t.Fatalf("full declared fleet did not complete: %+v", result)
	}
	references, err := recorder.Wait(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	type deviceEvidence struct {
		id                            *identity.Map
		os                            string
		metricNames                   []string
		metrics                       map[string][]int64
		processes                     []int64
		connections                   []int64
		processPIDs                   map[int32]bool
		connectionPID                 []int32
		hosts, software               int
		agentInventory, hostInventory int
		hostSystemInfo                int
	}
	devices := map[string]*deviceEvidence{}
	ordinal := 0
	for _, group := range scenario.Fleet {
		for range group.Count {
			id := identity.New(plan.RunID, plan.Seed, group.Group, ordinal)
			devices[id.Hostname] = &deviceEvidence{id: id, os: group.OS, metricNames: capture.Manifest.Profile.MetricNames, metrics: map[string][]int64{}, processPIDs: map[int32]bool{}}
			ordinal++
		}
	}
	device := func(host string) *deviceEvidence {
		t.Helper()
		got := devices[host]
		if got == nil {
			t.Fatalf("unexpected or replay-host identity on intake: %q", host)
		}
		return got
	}
	requestHosts := map[string]string{}
	systemInfoSerials := map[string]bool{}
	var systemInfoModel string
	for _, ref := range references {
		var inspected any
		switch ref.Path {
		case "/api/v1/collector", "/api/v1/connections":
			message, err := model.DecodeMessage(ref.Body)
			if err != nil {
				t.Fatal(err)
			}
			at, err := strconv.ParseInt(ref.Headers.Get(headers.TimestampHeader), 10, 64)
			if err != nil {
				t.Fatal("missing Agent collection timestamp header", err)
			}
			var host string
			switch body := message.Body.(type) {
			case *model.CollectorProc:
				host = body.HostName
				d := device(host)
				d.processes = append(d.processes, at)
				wantOS, wantChrome := "darwin", "Google Chrome"
				if d.os == "windows" {
					wantOS, wantChrome = "windows", "chrome.exe"
				}
				if body.Info == nil || body.Info.Os == nil || body.Info.Os.Name != wantOS || body.Info.Uuid != d.id.UUID || body.NetworkId != d.id.NetworkID || body.GroupSize != 1 {
					t.Fatal("process OS, UUID, network or chunk identity changed")
				}
				foundChrome := false
				for _, process := range body.Processes {
					d.processPIDs[process.Pid] = true
					if process.CreateTime != start.Add(-time.Minute).UnixMilli() || !slices.Contains(process.Tags, d.id.RunTag) {
						t.Fatal("process time or opaque run identity changed")
					}
					if process.Command.Comm == wantChrome {
						foundChrome = true
						if process.Cpu.TotalPct != 8 || process.Memory.Rss != 300<<20 {
							t.Fatal("healthy captured process workload changed")
						}
					}
				}
				if !foundChrome {
					t.Fatal("captured platform process missing from wire payload")
				}
			case *model.CollectorConnections:
				host = body.HostName
				d := device(host)
				d.connections = append(d.connections, at)
				if body.NetworkId != d.id.NetworkID || body.GroupSize != 1 || len(body.Connections) != 1 {
					t.Fatal("portable connection identity or evidence changed")
				}
				if config := body.AgentConfiguration; config == nil || !config.EudmEnabled || config.NpmEnabled != (d.os == "windows") {
					t.Fatal("connection product-routing flags changed between the captured sample and Agent delivery")
				}
				connection := body.Connections[0]
				d.connectionPID = append(d.connectionPID, connection.Pid)
				if connection.Rtt != 20000 || connection.LastBytesSent != 1000 || connection.Laddr.HostName != host {
					t.Fatal("captured connection baseline or local host relationship changed")
				}
			default:
				t.Fatalf("unexpected process message %T", message.Body)
			}
			if ref.Headers.Get(headers.HostHeader) != host || ref.Headers.Get(headers.ContentTypeHeader) != headers.ProtobufContentType || ref.Headers.Get(headers.ProcessVersionHeader) == "" {
				t.Fatal("Agent process headers do not match the encoded device")
			}
			requestID := ref.Headers.Get(headers.RequestIDHeader)
			if requestID == "" || (requestHosts[requestID] != "" && requestHosts[requestID] != host) {
				t.Fatal("missing or cross-host-colliding Agent request ID")
			}
			requestHosts[requestID] = host
			inspected = message.Body
		case "/api/v2/series":
			decoded, err := pipeline.Serializer.Strategy.Decompress(ref.Body)
			if err != nil {
				t.Fatal(err)
			}
			var payload gogen.MetricPayload
			if err := payload.Unmarshal(decoded); err != nil {
				t.Fatal(err)
			}
			if len(payload.Series) == 0 || ref.Headers.Get("DD-Agent-Payload") == "" {
				t.Fatal("missing Agent metric series or payload version header")
			}
			for _, serie := range payload.Series {
				var host string
				for _, resource := range serie.Resources {
					if resource.Type == "host" {
						host = resource.Name
					}
				}
				d := device(host)
				if !slices.Contains(serie.Tags, d.id.RunTag) || serie.Metadata.GetOrigin().GetOriginService() == 0 {
					t.Fatal("metric run tag or captured Agent origin lost")
				}
				metricType := metrics.APIGaugeType
				if tc.MetricFamily(serie.Metric) == "network" {
					metricType = metrics.APIRateType
				}
				if int32(serie.Type) != metricType.SeriesAPIV2Enum() || serie.Interval != int64(capture.Manifest.MetricCadences[tc.MetricFamily(serie.Metric)]/time.Second) {
					t.Fatal("native metric type or check interval changed during delivery")
				}
				for _, point := range serie.Points {
					d.metrics[serie.Metric] = append(d.metrics[serie.Metric], point.Timestamp)
					if (serie.Metric == "system.cpu.user" && point.Value != 5) || (serie.Metric == "system.wlan.rssi" && point.Value != -55) {
						t.Fatal("healthy host or WLAN baseline changed")
					}
				}
			}
			inspected = &payload
		case "/intake/", "/api/v2/host_metadata":
			decoded, err := pipeline.Serializer.Strategy.Decompress(ref.Body)
			if err != nil {
				t.Fatal(err)
			}
			var payload telemetry.HostMetadata
			if err := json.Unmarshal(decoded, &payload); err != nil {
				t.Fatal(err)
			}
			d := device(payload.Hostname)
			d.hosts++
			wantOS := "darwin"
			if d.os == "windows" {
				wantOS = "win32"
			}
			if payload.OS != wantOS || payload.UUID != d.id.UUID || !slices.Contains(payload.HostTags["system"], d.id.RunTag) {
				t.Fatal("host enrichment metadata does not represent the simulated device")
			}
			inspected = &payload
		case "/api/v1/metadata":
			decoded, err := pipeline.Serializer.Strategy.Decompress(ref.Body)
			if err != nil {
				t.Fatal(err)
			}
			var payload tc.Inventory
			if err := json.Unmarshal(decoded, &payload); err != nil {
				t.Fatal(err)
			}
			d := device(payload.Hostname)
			envelopes := 0
			for _, present := range []bool{payload.Agent != nil, payload.Host != nil, payload.SystemInfo != nil} {
				if present {
					envelopes++
				}
			}
			if payload.UUID != d.id.UUID || envelopes != 1 {
				t.Fatal("inventory identity or envelope does not represent the simulated device")
			}
			if agent := payload.Agent; agent != nil {
				d.agentInventory++
				if agent.InfrastructureMode != "end_user_device" || agent.AgentVersion != "7.85.0-fixture" || agent.Flavor != "agent" || !agent.FeatureProcessEnabled || !agent.FeatureNetworksEnabled {
					t.Fatal("Agent inventory discovery fields or producing version changed")
				}
				if payload.Timestamp != start.Add(250*time.Millisecond).UnixNano() || agent.AgentStartupTimeMS != start.Add(-time.Minute).UnixMilli() {
					t.Fatal("Agent inventory collection or startup time was not rebased from the captured offsets")
				}
			} else if payload.Host != nil {
				d.hostInventory++
				host := payload.Host
				arch, osname := "arm64", "darwin"
				if d.os == "windows" {
					arch, osname = "amd64", "Microsoft Windows 11 Pro"
				}
				if host.AgentVersion != "7.85.0-fixture" || host.CPUArchitecture != arch || host.OS != osname || host.CPULogicalProcessors != 4 || host.MemoryTotalKb != 16<<20 {
					t.Fatal("host inventory hardware, platform or producing version changed")
				}
				if payload.Timestamp != start.Add(500*time.Millisecond).UnixNano() || host.IPAddress == "10.0.0.1" || host.MacAddress == "02:00:00:00:00:02" {
					t.Fatal("host inventory timestamp or network identity was not rewritten")
				}
			} else {
				d.hostSystemInfo++
				info := payload.SystemInfo
				manufacturer := "Apple Inc."
				if d.os == "windows" {
					manufacturer = "Lenovo"
				}
				if payload.Timestamp != start.Add(5250*time.Millisecond).UnixNano() || info.Manufacturer != manufacturer || info.ChassisType != "Laptop" {
					t.Fatal("host system information lost its observed timing or hardware fields")
				}
				if info.SerialNumber == "" || info.SerialNumber == "serial_number-"+strings.Repeat("5", 32) || systemInfoSerials[info.SerialNumber] {
					t.Fatal("host system information serial was not isolated per simulated device")
				}
				systemInfoSerials[info.SerialNumber] = true
				if info.ModelName == "" || info.ModelName == "device_model-"+strings.Repeat("6", 32) || (systemInfoModel != "" && info.ModelName != systemInfoModel) {
					t.Fatal("host system information lost its shared run-scoped hardware model")
				}
				systemInfoModel = info.ModelName
				if d.os == "macos" && (info.ModelNumber != "Mac16,5" || info.Identifier != "Mac16,5") {
					t.Fatal("safe observed Mac model identifiers changed")
				}
			}
			for _, forbidden := range []string{"api_key", "apiKey", "auth_token", "agent_config", "full_configuration", "synthetic-integration-no-credential"} {
				if strings.Contains(string(decoded), forbidden) {
					t.Fatal("credential or configuration data appeared in inventory delivery")
				}
			}
			inspected = &payload
		case "/api/v2/softinv":
			decoded := ref.Body
			if encoding := ref.Headers.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
				kind, ok := map[string]string{"zstd": "zstd", "gzip": "gzip", "deflate": "zlib"}[encoding]
				if !ok {
					t.Fatalf("unexpected software encoding %q", encoding)
				}
				decoded, err = logscompression.NewComponent().NewCompressor(kind, 1).Decompress(ref.Body)
				if err != nil {
					t.Fatal(err)
				}
			}
			var payloads []softwareimpl.Payload
			if err := json.Unmarshal(decoded, &payloads); err != nil {
				t.Fatal(err)
			}
			for _, payload := range payloads {
				d := device(payload.Hostname)
				d.software++
				if len(payload.Metadata.Software) < 2 || payload.Metadata.Software[0].DisplayName != "Google Chrome" || payload.Metadata.Software[0].Version != "137.0.7151.69" {
					t.Fatal("complete software snapshot or captured Chrome version lost")
				}
			}
			inspected = payloads
		default:
			t.Fatalf("unexpected stream escaped the portable delivery graph: %s", ref.Path)
		}
		encoded, err := json.Marshal(inspected)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"capture-host", "synthetic-integration-no-credential", scenario.Meta.Name, "private-primary-cohort", "private-comparison-cohort", "expectation", "affected_cohorts"} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatalf("local or captured identity leaked into %s: %s", ref.Path, forbidden)
			}
		}
	}
	wantProcesses := []int64{start.Unix(), start.Add(10 * time.Second).Unix(), start.Add(20 * time.Second).Unix(), start.Add(30 * time.Second).Unix()}
	for host, evidence := range devices {
		if evidence.hosts != 1 || evidence.agentInventory != 1 || evidence.hostInventory != 1 || evidence.hostSystemInfo != 1 || evidence.software != 1 || len(evidence.metrics) != len(evidence.metricNames) {
			t.Fatalf("partial declared device %s: metadata=%d agent inventory=%d host inventory=%d system information=%d software=%d metric names=%d", host, evidence.hosts, evidence.agentInventory, evidence.hostInventory, evidence.hostSystemInfo, evidence.software, len(evidence.metrics))
		}
		for _, metric := range evidence.metricNames {
			wantMetrics := []int64{start.Unix(), start.Add(15 * time.Second).Unix(), start.Add(30 * time.Second).Unix()}
			if tc.MetricFamily(metric) == "battery" {
				wantMetrics = []int64{start.Unix()}
			}
			times := evidence.metrics[metric]
			slices.Sort(times)
			if !slices.Equal(times, wantMetrics) {
				t.Fatalf("metric cadence/coverage mismatch for %s %s: %v", host, metric, times)
			}
		}
		slices.Sort(evidence.processes)
		slices.Sort(evidence.connections)
		if !slices.Equal(evidence.processes, wantProcesses) || !slices.Equal(evidence.connections, wantProcesses) {
			t.Fatalf("process/connection cadence or platform coverage mismatch for %s", host)
		}
		for _, pid := range evidence.connectionPID {
			if !evidence.processPIDs[pid] {
				t.Fatal("connection PID does not resolve to a process on the same device")
			}
		}
	}
}

func TestWirelessEvidenceThroughAgentNDMBatchesAndMetricDelivery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	steady := func(value float64) schema.Pattern { return schema.Pattern{Steady: &schema.SteadyPattern{Value: value}} }
	s := &schema.Scenario{Version: schema.Version, Meta: schema.ScenarioMeta{Name: "private-wireless-wire-validation"}, Expectation: schema.Expectation{Conclusion: schema.WirelessAccessPoints, AffectedCohorts: []string{"clients-0"}}, MonitorWindow: schema.Duration{Duration: time.Minute}}
	for _, name := range []string{"healthy", "onset", "sustained", "recovery"} {
		s.Phases = append(s.Phases, schema.Phase{Name: name, Duration: schema.Duration{Duration: 2 * time.Minute}, Metrics: map[string]map[string]schema.Pattern{}, NetworkMetrics: map[string]schema.APPhaseMetrics{}})
	}
	for i := range 25 {
		name := fmt.Sprintf("private-ap-%d", i)
		s.NetworkDevices.AccessPoints = append(s.NetworkDevices.AccessPoints, schema.AccessPointDef{Name: name, IPAddress: fmt.Sprintf("192.0.2.%d", i+1), Vendor: "aruba", Model: "AP-535", Interfaces: []schema.NetworkInterfaceDef{{Name: "ethernet", Index: 1, Kind: "ethernet"}, {Name: "radio", Index: 2, Kind: "radio", Band: "5GHz", SSID: "private-network"}}})
		s.Fleet = append(s.Fleet, schema.GroupDef{Group: fmt.Sprintf("clients-%d", i), Count: 2, OS: "macos", AccessPoint: name, Radio: "radio"})
		s.Phases[0].NetworkMetrics[name] = schema.APPhaseMetrics{Device: map[string]schema.Pattern{"snmp.device.reachable": steady(1)}, Interfaces: map[string]map[string]schema.Pattern{"radio": {"snmp.apChannelNoise": steady(-95), "snmp.apChannelBwRate": steady(10), "snmp.ifInErrors": steady(0)}}}
	}
	s.Phases[2].NetworkMetrics["private-ap-0"] = schema.APPhaseMetrics{Interfaces: map[string]map[string]schema.Pattern{"radio": {"snmp.apChannelNoise": steady(-65), "snmp.apChannelBwRate": steady(90), "snmp.ifInErrors": steady(50)}}}
	s.Phases[2].Metrics["clients-0"] = map[string]schema.Pattern{"system.wlan.rssi": steady(-83), "system.wlan.noise": steady(-65), "system.wlan.txrate": steady(15)}
	runID := "11111111111111111111111111111111"
	model, err := accesspoint.New(s, runID, 42)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 29, 13, 0, 0, 0, time.UTC)
	recorder := output.NewRecorder()
	pipeline, err := output.New(ctx, nil, "synthetic-integration-no-credential", recorder, output.Options{QueueCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer pipeline.Close()
	pipeline.Config.Set("use_v3_api.series.enabled", "false", configmodel.SourceAgentRuntime)
	delivery := engine.AgentDelivery{Pipeline: pipeline}
	if err := delivery.NetworkMetadata(ctx, model.Metadata(at, 100)); err != nil {
		t.Fatal(err)
	}
	apMetrics, err := model.Metrics(2, time.Minute, 1, at)
	if err != nil {
		t.Fatal(err)
	}
	if err := delivery.NetworkMetrics(ctx, apMetrics); err != nil {
		t.Fatal(err)
	}
	captured := replayFixture(t, "macos")
	var metricData []byte
	for _, ref := range captured.Manifest.Samples {
		if ref.Stream == schema.Metrics {
			metricData = captured.Files[ref.File]
			break
		}
	}
	expectedRadio := map[string]*identity.Wireless{}
	affectedRadio, err := model.Wireless(s.Fleet[0])
	if err != nil {
		t.Fatal(err)
	}
	var clients []*telemetry.Sample
	for groupIndex, group := range s.Fleet {
		wireless, err := model.Wireless(group)
		if err != nil {
			t.Fatal(err)
		}
		for i := range group.Count {
			ordinal := groupIndex*2 + i
			sample, err := telemetry.Decode(schema.Metrics, metricData)
			if err != nil {
				t.Fatal(err)
			}
			if err := overlay.Apply(overlay.Context{Scenario: s, Group: group, Seed: 42, DeviceOrdinal: ordinal, PhaseIndex: 2, Elapsed: time.Minute, Stream: schema.Metrics, SampleOrdinal: 1}, sample); err != nil {
				t.Fatal(err)
			}
			id := identity.New(runID, 42, group.Group, ordinal)
			if err := id.Apply(sample, group, wireless); err != nil {
				t.Fatal(err)
			}
			for _, serie := range sample.Metrics {
				for j := range serie.Points {
					serie.Points[j].Ts = float64(at.Unix())
				}
			}
			expectedRadio[id.Hostname] = wireless
			clients = append(clients, sample)
		}
	}
	if err := delivery.Send(ctx, at, schema.Metrics, clients); err != nil {
		t.Fatal(err)
	}
	refs, err := recorder.Wait(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	devices := map[string]metadata.DeviceMetadata{}
	interfaces := map[string]metadata.InterfaceMetadata{}
	radios := map[string]metadata.WirelessInterfaceMetadata{}
	var addresses []metadata.IPAddressMetadata
	var series []*gogen.MetricPayload_MetricSeries
	batches := 0
	for _, ref := range refs {
		switch ref.Path {
		case "/api/v2/ndm":
			data := eventBody(t, ref)
			var payloads []metadata.NetworkDevicesMetadata
			if err := json.Unmarshal(data, &payloads); err != nil {
				t.Fatal(err)
			}
			for _, payload := range payloads {
				batches++
				count := len(payload.Devices) + len(payload.Interfaces) + len(payload.WirelessInterfaces) + len(payload.IPAddresses)
				if count > 100 || payload.Namespace != identity.Namespace(runID) || payload.CollectTimestamp != at.Unix() {
					t.Fatal("serialized NDM batch lost its resource bound, namespace or cadence")
				}
				for _, d := range payload.Devices {
					devices[d.ID] = d
				}
				for _, iface := range payload.Interfaces {
					interfaces[metadata.InterfaceID(iface.DeviceID, iface.Index)] = iface
				}
				for _, radio := range payload.WirelessInterfaces {
					radios[radio.BSSID] = radio
				}
				addresses = append(addresses, payload.IPAddresses...)
			}
			if strings.Contains(string(data), "private-") || strings.Contains(string(data), "wireless_access_points") {
				t.Fatal("local AP declaration or expectation leaked into NDM evidence")
			}
		case "/api/v2/series":
			data, err := pipeline.Serializer.Strategy.Decompress(ref.Body)
			if err != nil {
				t.Fatal(err)
			}
			var payload gogen.MetricPayload
			if err := payload.Unmarshal(data); err != nil {
				t.Fatal(err)
			}
			series = append(series, payload.Series...)
		default:
			t.Fatalf("unexpected wireless delivery route %s", ref.Path)
		}
	}
	if batches != 2 || len(devices) != 25 || len(interfaces) != 50 || len(radios) != 25 || len(addresses) != 25 {
		t.Fatal("Agent event-platform batching truncated or duplicated NDM resources")
	}
	for _, address := range addresses {
		if _, ok := interfaces[address.InterfaceID]; !ok {
			t.Fatal("NDM address lost its physical interface relationship")
		}
	}
	tagValue := func(tags []string, key string) string {
		for _, tag := range tags {
			if value, ok := strings.CutPrefix(tag, key+":"); ok {
				return value
			}
		}
		return ""
	}
	clientMACs := map[string]bool{}
	clientSignals, clientWorkloads, radioNoise, reachability := 0, 0, 0, 0
	for _, serie := range series {
		if !slices.Contains(serie.Tags, "eudm_run_id:"+runID) || len(serie.Points) != 1 || serie.Points[0].Timestamp != at.Unix() {
			t.Fatal("wireless metric lost its opaque run identity or absolute timestamp")
		}
		value := serie.Points[0].Value
		if serie.Metric == "snmp.device.reachable" {
			reachability++
			if value != 1 {
				t.Fatal("degraded AP became unreachable")
			}
		}
		if serie.Metric == "snmp.apChannelNoise" {
			radioNoise++
			radio, ok := radios[tagValue(serie.Tags, "bssid")]
			var hasDevice, hasInterface bool
			for _, resource := range serie.Resources {
				hasDevice = hasDevice || (resource.Type == "ndm_device" && resource.Name == radio.DeviceByIntegrationID)
				hasInterface = hasInterface || (resource.Type == "ndm_interface" && resource.Name == radio.InterfaceByIntegrationID)
			}
			if !ok || !hasDevice || !hasInterface {
				t.Fatal("AP metric resource tags do not resolve to the emitted radio")
			}
			if (radio.BSSID == affectedRadio.BSSID && value < -70) || (radio.BSSID != affectedRadio.BSSID && value > -90) {
				t.Fatal("degraded and comparison AP noise do not agree with the scenario")
			}
		}
		if serie.Metric != "system.wlan.rssi" && serie.Metric != "system.cpu.user" {
			continue
		}
		var host string
		for _, resource := range serie.Resources {
			if resource.Type == "host" {
				host = resource.Name
			}
		}
		wireless := expectedRadio[host]
		if wireless == nil {
			t.Fatal("unexpected wireless client host")
		}
		if serie.Metric == "system.cpu.user" {
			clientWorkloads++
			if value != 5 {
				t.Fatal("wireless scenario changed endpoint workloads")
			}
			continue
		}
		clientSignals++
		radio, ok := radios[wireless.BSSID]
		if !ok || tagValue(serie.Tags, "bssid") != radio.BSSID || tagValue(serie.Tags, "ssid") != radio.SSID || interfaces[radio.InterfaceByIntegrationID].MacAddress != radio.BSSID || devices[radio.DeviceByIntegrationID].Status != metadata.DeviceStatusReachable {
			t.Fatal("WLAN client metric does not resolve to its reachable NDM radio")
		}
		mac := tagValue(serie.Tags, "mac_address")
		if mac == "" || clientMACs[mac] {
			t.Fatal("WLAN clients share or omit a client MAC")
		}
		clientMACs[mac] = true
		want := -55.0
		if radio.BSSID == affectedRadio.BSSID {
			want = -83
		}
		if value != want {
			t.Fatal("client degradation does not agree with its AP cohort")
		}
	}
	if clientSignals != 50 || clientWorkloads != 50 || radioNoise != 25 || reachability != 25 {
		t.Fatal("serialized wireless fleet lacks client, AP or healthy comparison evidence")
	}
}

func eventBody(t *testing.T, ref output.RecordedRequest) []byte {
	t.Helper()
	encoding := ref.Headers.Get("Content-Encoding")
	if encoding == "" || encoding == "identity" {
		return ref.Body
	}
	kind, ok := map[string]string{"zstd": "zstd", "gzip": "gzip", "deflate": "zlib"}[encoding]
	if !ok {
		t.Fatalf("unexpected event-platform encoding %q", encoding)
	}
	decoded, err := logscompression.NewComponent().NewCompressor(kind, 1).Decompress(ref.Body)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}
