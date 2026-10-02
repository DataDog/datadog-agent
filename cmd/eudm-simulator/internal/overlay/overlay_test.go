// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package overlay

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	softwareimpl "github.com/DataDog/datadog-agent/comp/softwareinventory/impl"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func steady(value float64) schema.Pattern {
	return schema.Pattern{Steady: &schema.SteadyPattern{Value: value}}
}
func ramp(from, to float64) schema.Pattern {
	return schema.Pattern{Ramp: &schema.RampPattern{From: from, To: to}}
}

func contextFixture(name, app string) Context {
	group := schema.GroupDef{Group: "rollout", OS: "macos", Count: 2}
	scenario := &schema.Scenario{Fleet: []schema.GroupDef{group}, Phases: []schema.Phase{
		{Name: "healthy", Duration: schema.Duration{Duration: time.Minute}},
		{Name: "onset", Duration: schema.Duration{Duration: time.Minute}, Processes: map[string][]schema.ProcessDef{"rollout": {{Name: name, CPU: ramp(4, 40), Memory: ramp(400, 800)}}}, Software: map[string][]schema.SoftwareItem{"rollout": {{Name: app, Version: "10.2"}}}},
		{Name: "sustained", Duration: schema.Duration{Duration: time.Minute}, Processes: map[string][]schema.ProcessDef{"rollout": {{Name: name, CPU: steady(40), Memory: steady(800)}}}, Software: map[string][]schema.SoftwareItem{"rollout": {{Name: app, Version: "10.2"}}}},
		{Name: "recovery", Duration: schema.Duration{Duration: time.Minute}},
	}}
	info := &model.SystemInfo{Cpus: []*model.CPUInfo{{Cores: 4}}, TotalMemory: 8192 * megabyte}
	process := func(pid int32, cpu float32, memory uint64) *model.Process {
		return &model.Process{Pid: pid, Command: &model.Command{Comm: name, Exe: sentinelDirectory("10.1") + `\SentinelAgent.exe`, Args: []string{"baseline"}}, Cpu: &model.CPUStat{TotalPct: cpu, UserPct: cpu * .75, SystemPct: cpu * .25}, Memory: &model.MemoryStat{Rss: memory * megabyte, Vms: (memory + 100) * megabyte}}
	}
	return Context{Scenario: scenario, Group: group, Seed: 42, DeviceOrdinal: 1, ProcessSampleOrdinal: 7, BaselineProcesses: []*model.CollectorProc{
		{HostName: "capture-host", Info: info, Processes: []*model.Process{process(10, 12, 300), {Pid: 3, Command: &model.Command{Comm: "background"}, Cpu: &model.CPUStat{TotalPct: 2}, Memory: &model.MemoryStat{Rss: megabyte}}}},
		{HostName: "capture-host", Info: info, Processes: []*model.Process{process(20, 4, 100)}},
	}}
}

func clone[T any](t *testing.T, value T) T {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func metricFixture() []*metrics.Serie {
	values := []struct {
		name  string
		value float64
	}{
		{"system.cpu.user", 20}, {"system.cpu.system", 5}, {"system.cpu.idle", 75},
		{"system.mem.used", 2048}, {"system.mem.free", 6144}, {"system.mem.usable", 6144}, {"system.mem.pct_usable", .75},
		{"system.wlan.rssi", -55}, {"system.wlan.txrate", 866},
	}
	var result []*metrics.Serie
	for _, value := range values {
		result = append(result, &metrics.Serie{Name: value.name, Host: "capture-host", MType: metrics.APIGaugeType, Points: []metrics.Point{{Value: value.value, Ts: 1}}})
	}
	return result
}

func metricValues(sample *telemetry.Sample) map[string]float64 {
	values := map[string]float64{}
	for _, serie := range sample.Metrics {
		values[serie.Name] = serie.Points[0].Value
	}
	return values
}

func closeEnough(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > .001 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCorrelatedScenarioProgression(t *testing.T) {
	for _, app := range []struct{ process, software string }{{"Google Chrome", "Google Chrome"}, {"SentinelAgent.exe", "SentinelOne"}} {
		t.Run(app.software, func(t *testing.T) {
			base := contextFixture(app.process, app.software)
			for _, point := range []struct {
				phase                   int
				elapsed                 time.Duration
				targetCPU, targetMemory float64
				version                 string
			}{{0, 0, 16, 400, "10.1"}, {1, 0, 16, 400, "10.2"}, {1, 30 * time.Second, 88, 600, "10.2"}, {2, 0, 160, 800, "10.2"}, {3, 0, 16, 400, "10.1"}} {
				ctx := base
				ctx.PhaseIndex, ctx.Elapsed = point.phase, point.elapsed
				var cpu, user, system, memory float64
				for _, chunk := range base.BaselineProcesses {
					sample := &telemetry.Sample{Processes: clone(t, chunk)}
					ctx.Stream = schema.Processes
					if err := Apply(ctx, sample); err != nil {
						t.Fatal(err)
					}
					for _, process := range sample.Processes.Processes {
						if process.Command.Comm != app.process {
							if !reflect.DeepEqual(process, chunk.Processes[1]) {
								t.Fatal("background changed")
							}
							continue
						}
						cpu += float64(process.Cpu.TotalPct)
						user += float64(process.Cpu.UserPct)
						system += float64(process.Cpu.SystemPct)
						memory += float64(process.Memory.Rss) / megabyte
						if app.process == "SentinelAgent.exe" && !strings.Contains(process.Command.Exe, "Sentinel Agent "+point.version+`\SentinelAgent.exe`) {
							t.Fatal("Sentinel process path and software version differ")
						}
					}
				}
				closeEnough(t, cpu, point.targetCPU)
				closeEnough(t, memory, point.targetMemory)
				ctx.Stream, ctx.SampleOrdinal = schema.Metrics, 103
				metricSample := &telemetry.Sample{Metrics: metricFixture()}
				if err := Apply(ctx, metricSample); err != nil {
					t.Fatal(err)
				}
				values := metricValues(metricSample)
				closeEnough(t, values["system.cpu.user"], 20+(user-12)/4)
				closeEnough(t, values["system.cpu.system"], 5+(system-4)/4)
				closeEnough(t, values["system.cpu.user"]+values["system.cpu.system"]+values["system.cpu.idle"], 100)
				closeEnough(t, values["system.mem.used"], 2048+memory-400)
				closeEnough(t, values["system.mem.free"], 6144-memory+400)
				closeEnough(t, values["system.mem.usable"]/8192, values["system.mem.pct_usable"])
				closeEnough(t, values["system.wlan.rssi"], -55)
				ctx.Stream = schema.Software
				inventory := &telemetry.Sample{Software: &softwareimpl.Payload{Hostname: "capture-host", Metadata: softwareimpl.HostSoftware{Software: []software.Entry{{DisplayName: app.software, Version: "10.1", InstallPaths: []string{sentinelDirectory("10.1")}}, {DisplayName: "background", Version: "1"}}}}}
				if err := Apply(ctx, inventory); err != nil {
					t.Fatal(err)
				}
				if len(inventory.Software.Metadata.Software) != 2 || inventory.Software.Metadata.Software[0].Version != point.version || inventory.Software.Metadata.Software[1].Version != "1" {
					t.Fatal("software update duplicated or altered unrelated entries")
				}
			}
		})
	}
}

func TestVariationCorrelatesAcrossStreamsAndChunking(t *testing.T) {
	ctx := contextFixture("Google Chrome", "Google Chrome")
	ctx.PhaseIndex = 2
	ctx.Group.BaselineVariance = .1
	original := clone(t, ctx.BaselineProcesses)
	var cpu float64
	for i, chunk := range ctx.BaselineProcesses {
		ctx.Stream, ctx.SampleOrdinal = schema.Processes, int64(i)
		sample := &telemetry.Sample{Processes: clone(t, chunk)}
		if err := Apply(ctx, sample); err != nil {
			t.Fatal(err)
		}
		for _, process := range sample.Processes.Processes {
			if process.Command.Comm == "Google Chrome" {
				cpu += float64(process.Cpu.TotalPct)
			}
		}
	}
	ctx.Stream, ctx.SampleOrdinal = schema.Metrics, 200
	sample := &telemetry.Sample{Metrics: metricFixture()}
	if err := Apply(ctx, sample); err != nil {
		t.Fatal(err)
	}
	values := metricValues(sample)
	closeEnough(t, 75-values["system.cpu.idle"], (cpu-16)/4)
	if cpu < 144 || cpu > 176 {
		t.Fatal("device variation exceeded configured bounds")
	}
	if !reflect.DeepEqual(original, ctx.BaselineProcesses) {
		t.Fatal("shared baseline was mutated")
	}
	ctx.Group.Group = "comparison"
	unaffected := &telemetry.Sample{Metrics: metricFixture()}
	if err := Apply(ctx, unaffected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(unaffected.Metrics, metricFixture()) {
		t.Fatal("variation changed unaffected cohort")
	}
}

func TestVPNUsesOnlyCapturedTCPPath(t *testing.T) {
	ctx := contextFixture("Google Chrome", "Google Chrome")
	ctx.PhaseIndex = 2
	connection := &model.Connection{Pid: 10, Laddr: &model.Addr{Ip: "192.0.2.10", Port: 2000}, Raddr: &model.Addr{Ip: "192.0.2.20", Port: 443}, Rtt: 30000, TcpFailuresByErrCode: map[uint32]uint32{104: 2}}
	selector := telemetry.ConnectionSelector(connection)
	rtt, variance, retransmits := steady(600), steady(180), steady(30)
	ctx.Scenario.Phases[2].Processes = nil
	ctx.Scenario.Phases[2].Connections = map[string][]schema.ConnectionOverlay{"rollout": {{Selector: selector, RTTMilliseconds: &rtt, RTTVarianceMilliseconds: &variance, Retransmits: &retransmits, TCPFailures: map[uint32]schema.Pattern{110: steady(8)}}}}
	other := clone(t, connection)
	other.Raddr.Ip = "192.0.2.21"
	sample := &telemetry.Sample{Connections: &model.CollectorConnections{Connections: []*model.Connection{clone(t, connection), clone(t, other)}}}
	ctx.Stream = schema.Connections
	if err := Apply(ctx, sample); err != nil {
		t.Fatal(err)
	}
	got := sample.Connections.Connections[0]
	if got.Rtt != 600000 || got.RttVar != 180000 || got.LastRetransmits != 30 || got.TcpFailuresByErrCode[110] != 8 || got.TcpFailuresByErrCode[104] != 2 {
		t.Fatal("VPN units or TCP failure evidence differ")
	}
	if !reflect.DeepEqual(other, sample.Connections.Connections[1]) {
		t.Fatal("non-VPN connection changed")
	}
	ctx.Stream = schema.Metrics
	metricsSample := &telemetry.Sample{Metrics: metricFixture()}
	if err := Apply(ctx, metricsSample); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(metricsSample.Metrics, metricFixture()) {
		t.Fatal("VPN degradation changed host or WLAN metrics")
	}
	ctx.Stream = schema.Connections
	ctx.Scenario.Phases[2].Connections["rollout"][0].TCPFailures[999] = steady(1)
	if err := Apply(ctx, sample); err == nil {
		t.Fatal("nonstandard TCP failure accepted")
	}
}

func TestMissingEvidenceFailsWithoutFabrication(t *testing.T) {
	ctx := contextFixture("Google Chrome", "Google Chrome")
	ctx.PhaseIndex = 2
	ctx.Stream = schema.Processes
	ctx.Scenario.Phases[2].Processes["rollout"][0].Name = "absent"
	sample := &telemetry.Sample{Processes: clone(t, ctx.BaselineProcesses[0])}
	if err := Apply(ctx, sample); err == nil {
		t.Fatal("invented an absent process")
	}
	ctx.Scenario.Phases[2].Processes["rollout"][0].Name = "Google Chrome"
	ctx.BaselineProcesses[0].Info.Cpus = nil
	if err := Apply(ctx, sample); err == nil {
		t.Fatal("invented CPU topology")
	}
}
