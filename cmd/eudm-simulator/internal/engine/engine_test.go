// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/accesspoint"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/report"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/bazelbuild/rules_go/go/runfiles"
)

const fixtureCommit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type advancingClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *advancingClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *advancingClock) WaitUntil(ctx context.Context, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	if at.After(c.now) {
		c.now = at
	}
	c.mu.Unlock()
	return nil
}

func testFile(t *testing.T, relative string) string {
	t.Helper()
	if os.Getenv("TEST_SRCDIR") == "" {
		return filepath.Join("..", "..", filepath.FromSlash(relative))
	}
	r, err := runfiles.New()
	if err != nil {
		t.Fatal(err)
	}
	repository := runfiles.CallerRepository()
	if repository == "" {
		repository = "_main"
	}
	path, err := r.Rlocation(repository + "/cmd/eudm-simulator/" + relative)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func capturedFixture(t *testing.T, platform string) *bundle.Loaded {
	t.Helper()
	prefix := "testdata/bundles/" + platform + "/"
	data, err := os.ReadFile(testFile(t, prefix+"manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest bundle.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	// Materialize regular files: the bundle loader correctly refuses symlinks,
	// whereas Bazel uses symlinks for its immutable input runfiles.
	directory := t.TempDir()
	names := []string{"manifest.json", "COMPLETE"}
	for name := range manifest.Files {
		names = append(names, name)
	}
	for _, name := range names {
		data, err := os.ReadFile(testFile(t, prefix+name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := bundle.Load(directory, fixtureCommit)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func requestFor(t *testing.T, scenario *schema.Scenario, digest string, capture *bundle.Loaded) Request {
	t.Helper()
	plan, err := schema.NewPlan(scenario, digest, fixtureCommit, 481516, time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC), capture.Ref())
	if err != nil {
		t.Fatal(err)
	}
	plan.RunID = strings.Repeat("7", 32)
	return Request{Scenario: scenario, Plan: plan, Bundle: capture}
}

func shippedRequest(t *testing.T, name string) Request {
	t.Helper()
	captures := map[string]*bundle.Loaded{"macos": capturedFixture(t, "macos"), "windows": capturedFixture(t, "windows")}
	data, err := os.ReadFile(testFile(t, "scenarios/"+name+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// Operator adaptation is explicit: use a known captured VPN path. The
	// shipped template must never guess which private endpoint represents VPN.
	data = []byte(strings.ReplaceAll(string(data), "REPLACE_WITH_CAPTURED_VPN_CONNECTION_SELECTOR", captures["windows"].Manifest.Profile.ConnectionSelectors[0]))
	var scenario schema.Scenario
	if err := schema.DecodeStrict(data, &scenario); err != nil {
		t.Fatal(err)
	}
	return requestFor(t, &scenario, schema.Digest(data), captures[scenario.Fleet[0].OS])
}

func sharedBaselineRequest(t *testing.T) Request {
	t.Helper()
	scenario := &schema.Scenario{Version: schema.Version, Meta: schema.ScenarioMeta{Name: "shared-baseline-fixture"}, Expectation: schema.Expectation{Conclusion: schema.Healthy}, Fleet: []schema.GroupDef{{Group: "primary", OS: "windows", Count: 2}, {Group: "comparison", OS: "windows", Count: 3}}, Phases: []schema.Phase{{Name: "healthy", Duration: schema.Duration{Duration: 2 * time.Minute}}}}
	return requestFor(t, scenario, schema.Digest([]byte("shared-baseline-fixture-v1")), capturedFixture(t, "windows"))
}

type deliveryRecord struct {
	Host    string
	Stream  schema.Stream
	Offset  time.Duration
	Chunks  int
	Payload string
}
type recordingDelivery struct {
	mu                   sync.Mutex
	start                time.Time
	records              []deliveryRecord
	retainPayload        bool
	failAt, calls, waits int
	waitError            error
	gate                 <-chan struct{}
	entered              chan struct{}
	once                 sync.Once
}

func normalizedSample(sample *telemetry.Sample, stream schema.Stream, start time.Time) ([]byte, error) {
	switch stream {
	case schema.Metrics:
		value, err := telemetry.NewMetricSample(sample.Metrics)
		if err != nil {
			return nil, err
		}
		for i := range value.Series {
			for j := range value.Series[i].Points {
				value.Series[i].Points[j].Timestamp -= float64(start.UnixNano()) / 1e9
			}
		}
		return json.Marshal(value)
	case schema.Processes:
		data, err := json.Marshal(sample.Processes)
		if err != nil {
			return nil, err
		}
		var value model.CollectorProc
		if err := json.Unmarshal(data, &value); err != nil {
			return nil, err
		}
		for _, process := range value.Processes {
			process.CreateTime -= start.UnixMilli()
		}
		return json.Marshal(&value)
	case schema.Connections:
		return json.Marshal(sample.Connections)
	case schema.HostMetadata:
		return json.Marshal(sample.HostMetadata)
	case schema.AgentInventory, schema.HostInventory, schema.HostSystemInfo:
		value := *sample.Inventory
		value.Timestamp -= start.UnixNano()
		if value.Agent != nil {
			agent := *value.Agent
			agent.AgentStartupTimeMS -= start.UnixMilli()
			value.Agent = &agent
		}
		return json.Marshal(&value)
	case schema.Software:
		return json.Marshal(sample.Software)
	default:
		return nil, fmt.Errorf("unsupported test stream %s", stream)
	}
}

func sampleHost(sample *telemetry.Sample, stream schema.Stream) string {
	switch stream {
	case schema.Metrics:
		return sample.Metrics[0].Host
	case schema.Processes:
		return sample.Processes.HostName
	case schema.Connections:
		return sample.Connections.HostName
	case schema.HostMetadata:
		return sample.HostMetadata.Hostname
	case schema.AgentInventory, schema.HostInventory, schema.HostSystemInfo:
		return sample.Inventory.Hostname
	case schema.Software:
		return sample.Software.Hostname
	default:
		return ""
	}
}

func (d *recordingDelivery) record(ctx context.Context, record deliveryRecord) error {
	if d.entered != nil {
		d.once.Do(func() { close(d.entered) })
	}
	if d.gate != nil {
		select {
		case <-d.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	if d.calls == d.failAt {
		return errors.New("injected exhausted delivery")
	}
	d.records = append(d.records, record)
	return nil
}

func (d *recordingDelivery) Send(ctx context.Context, at time.Time, stream schema.Stream, samples []*telemetry.Sample) error {
	if len(samples) == 0 {
		return errors.New("empty collection cycle")
	}
	host := sampleHost(samples[0], stream)
	if !strings.HasPrefix(host, "eudm-") {
		return errors.New("unrewritten device identity")
	}
	var payloads []json.RawMessage
	for _, sample := range samples {
		if sampleHost(sample, stream) != host {
			return errors.New("mixed hosts within one delivery cycle")
		}
		if stream == schema.Processes && sample.Processes.GroupSize != int32(len(samples)) {
			return errors.New("incorrect process chunk count")
		}
		if stream == schema.Connections && sample.Connections.GroupSize != int32(len(samples)) {
			return errors.New("incorrect connection chunk count")
		}
		if d.retainPayload {
			body, err := normalizedSample(sample, stream, d.start)
			if err != nil {
				return err
			}
			payloads = append(payloads, body)
		}
	}
	body, err := json.Marshal(payloads)
	if err != nil {
		return err
	}
	return d.record(ctx, deliveryRecord{Host: host, Stream: stream, Offset: at.Sub(d.start), Chunks: len(samples), Payload: string(body)})
}
func (d *recordingDelivery) NetworkMetrics(ctx context.Context, series []*metrics.Serie) error {
	if len(series) == 0 || len(series[0].Points) == 0 {
		return errors.New("empty AP metrics")
	}
	at := time.Unix(0, int64(series[0].Points[0].Ts*1e9))
	return d.record(ctx, deliveryRecord{Stream: APMetricStream, Offset: at.Sub(d.start), Chunks: 1})
}
func (d *recordingDelivery) NetworkMetadata(ctx context.Context, payloads []metadata.NetworkDevicesMetadata) error {
	if len(payloads) == 0 {
		return errors.New("empty NDM metadata")
	}
	return d.record(ctx, deliveryRecord{Stream: NDMStream, Offset: time.Unix(payloads[0].CollectTimestamp, 0).Sub(d.start), Chunks: len(payloads)})
}
func (d *recordingDelivery) Wait(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.waits++
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return d.waitError
}

func testDuration(s *schema.Scenario) time.Duration {
	var total time.Duration
	for _, phase := range s.Phases {
		total += phase.Duration.Duration
	}
	return total
}

func assertCompleteCadences(t *testing.T, request Request, result *report.Report, sink *recordingDelivery) {
	t.Helper()
	if result == nil || !result.Complete() || result.Status != "succeeded" {
		t.Fatalf("incomplete run: %+v", result)
	}
	if result.ReplayOS != runtime.GOOS || !reflect.DeepEqual(result.Expectation, request.Scenario.Expectation) {
		t.Fatal("report lost replay OS or local expectation")
	}
	if result.BundleDigest != request.Bundle.Digest {
		t.Fatal("report lost the shared baseline identity")
	}
	for name, digest := range request.Bundle.Manifest.Files {
		if schema.Digest(request.Bundle.Files[name]) != digest {
			t.Fatal("scenario overlays modified the shared baseline capture")
		}
	}
	duration := testDuration(request.Scenario)
	if result.End.Sub(result.Start) != duration {
		t.Fatal("fake clock did not traverse entire declared duration")
	}
	seen := map[string]map[schema.Stream][]time.Duration{}
	for _, record := range sink.records {
		if seen[record.Host] == nil {
			seen[record.Host] = map[schema.Stream][]time.Duration{}
		}
		seen[record.Host][record.Stream] = append(seen[record.Host][record.Stream], record.Offset)
	}
	for _, device := range result.Ledger {
		capture := request.Bundle
		if device.BundleDigest != capture.Digest {
			t.Fatal("device did not use the shared baseline")
		}
		for stream, cadence := range capture.Manifest.Cadences {
			// Fixtures use uniform non-metric cadences; hardware arrives at 5s.
			// Assert actual timestamps, not just counters reported by the engine.
			var expected []time.Duration
			first := time.Duration(0)
			if stream == schema.HostSystemInfo {
				first = 5 * time.Second
			}
			for at := first; at < duration; at += cadence {
				expected = append(expected, at)
			}
			if stream == schema.Metrics {
				expected = expectedMetricOffsets(t, capture, duration)
			}
			actual := seen[device.Hostname][stream]
			slices.Sort(actual)
			if !slices.Equal(actual, expected) {
				t.Fatalf("%s %s cadence differs: got %d cycles, want %d", device.Hostname, stream, len(actual), len(expected))
			}
			counts := device.Streams[stream]
			if counts == nil || counts.Expected != uint64(len(expected)) || counts.Delivered != uint64(len(expected)) || counts.Failed != 0 {
				t.Fatal("ledger differs from actual full-fleet delivery")
			}
		}
	}
	for stream, cadence := range map[schema.Stream]time.Duration{APMetricStream: accesspoint.MetricsCadence, NDMStream: accesspoint.MetadataCadence} {
		if counts := result.NetworkStreams[stream]; counts != nil {
			expected := uint64(1 + (duration-1)/cadence)
			if counts.Expected != expected || uint64(len(seen[""][stream])) != expected {
				t.Fatal("AP cadence or network ledger is incomplete")
			}
		}
	}
}

// Derive expectations directly from the typed evidence, independently of the
// scheduler. Each original producer/cycle contributes once to each family.
func expectedMetricOffsets(t *testing.T, b *bundle.Loaded, duration time.Duration) []time.Duration {
	t.Helper()
	families := map[string]map[string]time.Duration{}
	for _, ref := range b.Manifest.Samples {
		if ref.Stream != schema.Metrics {
			continue
		}
		sample, err := telemetry.Decode(ref.Stream, b.Files[ref.File])
		if err != nil {
			t.Fatal(err)
		}
		for _, serie := range sample.Metrics {
			family := telemetrycapture.MetricFamily(serie.Name)
			if families[family] == nil {
				families[family] = map[string]time.Duration{}
			}
			families[family][fmt.Sprintf("%s/%d", ref.ProducerID, ref.CycleID)] = ref.Offset
		}
	}
	var expected []time.Duration
	for family, cycles := range families {
		var offsets []time.Duration
		for _, offset := range cycles {
			offsets = append(offsets, offset)
		}
		slices.Sort(offsets)
		cadence := b.Manifest.MetricCadences[family]
		if cadence <= 0 {
			t.Fatal("fixture lacks a metric family cadence")
		}
		period := offsets[len(offsets)-1] - offsets[0] + cadence
		for _, offset := range offsets {
			for at := offset; at < duration; at += period {
				expected = append(expected, at)
			}
		}
	}
	slices.Sort(expected)
	return expected
}

func TestEveryShippedFleetRunsAtNativeCadence(t *testing.T) {
	for _, name := range []string{"healthy-macos", "healthy-windows", "application-update-regression-macos", "windows-security-agent-regression", "vpn-degradation-windows", "wifi-degradation-macos"} {
		t.Run(name, func(t *testing.T) {
			request := shippedRequest(t, name)
			sink := &recordingDelivery{start: request.Plan.Start}
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			result, err := Run(ctx, request, Options{Workers: 4, QueueCapacity: 1, Clock: &advancingClock{now: request.Plan.Start}, Delivery: sink})
			if err != nil {
				t.Fatal(err)
			}
			assertCompleteCadences(t, request, result, sink)
			var count int
			for _, cohort := range request.Scenario.Fleet {
				count += cohort.Count
			}
			if len(result.Ledger) != count || result.DeclaredDevices != count {
				t.Fatal("full declared fleet was truncated")
			}
			if name == "wifi-degradation-macos" && count != 60 {
				t.Fatal("update this full-fleet bound when the shipped Wi-Fi fleet changes")
			}
		})
	}
}

func TestSharedBaselineReplayDeterministicAcrossWorkersAndMapOrder(t *testing.T) {
	request := sharedBaselineRequest(t)
	var first []deliveryRecord
	for run, workers := range []int{1, 4} {
		if run != 0 {
			request.Plan.Start = request.Plan.Start.Add(7 * 24 * time.Hour)
			capture := request.Bundle
			files := map[string][]byte{}
			var names []string
			for name := range capture.Files {
				names = append(names, name)
			}
			slices.Sort(names)
			slices.Reverse(names)
			for _, name := range names {
				files[name] = capture.Files[name]
			}
			capture.Files = files
		}
		sink := &recordingDelivery{start: request.Plan.Start, retainPayload: true}
		result, err := Run(context.Background(), request, Options{Workers: workers, QueueCapacity: 1, Clock: &advancingClock{now: request.Plan.Start}, Delivery: sink})
		if err != nil {
			t.Fatal(err)
		}
		assertCompleteCadences(t, request, result, sink)
		slices.SortFunc(sink.records, func(a, b deliveryRecord) int {
			aJSON, _ := json.Marshal(a)
			bJSON, _ := json.Marshal(b)
			return strings.Compare(string(aJSON), string(bJSON))
		})
		if run == 0 {
			first = sink.records
		} else if !reflect.DeepEqual(first, sink.records) {
			t.Fatal("worker order, map insertion order, or absolute start changed normalized replay")
		}
	}
}

func TestBackpressureDoesNotTruncateFleet(t *testing.T) {
	request := sharedBaselineRequest(t)
	gate := make(chan struct{})
	entered := make(chan struct{})
	sink := &recordingDelivery{start: request.Plan.Start, gate: gate, entered: entered}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	type completion struct {
		report *report.Report
		err    error
	}
	done := make(chan completion, 1)
	go func() {
		result, err := Run(ctx, request, Options{Workers: 1, QueueCapacity: 1, Clock: &advancingClock{now: request.Plan.Start}, Delivery: sink})
		done <- completion{result, err}
	}()
	select {
	case <-entered:
	case result := <-done:
		t.Fatalf("run ended before blocked delivery: %v", result.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-done:
		t.Fatal("reported success while first delivery was blocked")
	default:
	}
	close(gate)
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatal(result.err)
		}
		assertCompleteCadences(t, request, result.report, sink)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestMidRunAndDrainFailuresKeepIncompleteLedger(t *testing.T) {
	for _, atDrain := range []bool{false, true} {
		request := sharedBaselineRequest(t)
		sink := &recordingDelivery{start: request.Plan.Start, failAt: 7}
		if atDrain {
			sink.failAt = 0
			sink.waitError = errors.New("injected final retry exhaustion")
		}
		result, err := Run(context.Background(), request, Options{Workers: 1, QueueCapacity: 1, Clock: &advancingClock{now: request.Plan.Start}, Delivery: sink})
		if err == nil || result == nil || result.Status != "failed" || result.Complete() || len(result.Errors) == 0 {
			t.Fatal("partial fleet or failed drain was reported successful")
		}
		if len(result.Ledger) != 5 {
			t.Fatal("failure removed unsent devices from ledger")
		}
		var expected, delivered, failed uint64
		for _, device := range result.Ledger {
			for _, counts := range device.Streams {
				expected += counts.Expected
				delivered += counts.Delivered
				failed += counts.Failed
			}
		}
		if atDrain {
			if delivered != expected || sink.waits != 1 {
				t.Fatal("final drain failure lost accepted cycles")
			}
		} else if delivered != 6 || failed != 1 || expected <= delivered+failed || sink.waits != 0 {
			t.Fatal("mid-run failure counts conceal unsent work")
		}
	}
}

func TestCorruptedBundleRejectedBeforeAnyDelivery(t *testing.T) {
	request := sharedBaselineRequest(t)
	for name := range request.Bundle.Files {
		request.Bundle.Files[name] = []byte("corrupted after loading")
		break
	}
	sink := &recordingDelivery{start: request.Plan.Start}
	result, err := Run(context.Background(), request, Options{Workers: 4, QueueCapacity: 1, Clock: &advancingClock{now: request.Plan.Start}, Delivery: sink})
	if err == nil || result != nil || sink.calls != 0 || sink.waits != 0 {
		t.Fatal("corrupt capture reached delivery")
	}
}

func TestExecuteDoesNotRequireAFuturePlannedStart(t *testing.T) {
	request := sharedBaselineRequest(t)
	request.Plan.Start = time.Now().UTC().Add(-time.Minute)
	reportPath := filepath.Join(t.TempDir(), "report.json")
	err := Execute(context.Background(), request, ExecutionOptions{
		ReportPath: reportPath,
	})
	// Stop before constructing live forwarders. An ordinary immediate run must
	// reach the credential check instead of rejecting its provisional start time.
	if err == nil || !strings.Contains(err.Error(), "DD_API_KEY") {
		t.Fatalf("expected the missing-key error, got %v", err)
	}
	if _, err := os.Stat(reportPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing credentials created a report: %v", err)
	}
}

func rewriteFixtureSamples(t *testing.T, request Request, stream schema.Stream, mutate func(*telemetry.Sample, int)) {
	t.Helper()
	capture := request.Bundle
	index := 0
	for _, ref := range capture.Manifest.Samples {
		if ref.Stream != stream {
			continue
		}
		sample, err := telemetry.Decode(stream, capture.Files[ref.File])
		if err != nil {
			t.Fatal(err)
		}
		mutate(sample, index)
		index++
		data, err := normalizedSample(sample, stream, time.Unix(0, 0))
		if err != nil {
			t.Fatal(err)
		}
		capture.Files[ref.File] = data
		capture.Manifest.Files[ref.File] = schema.Digest(data)
	}
}

func TestSemanticPreflightRejectsUnproducibleFleetBeforeDelivery(t *testing.T) {
	for _, tc := range []struct {
		name, scenario, want string
		mutate               func(*testing.T, Request)
	}{
		{"missing-cpu-topology", "application-update-regression-macos", "CPU topology", func(t *testing.T, r Request) {
			rewriteFixtureSamples(t, r, schema.Processes, func(s *telemetry.Sample, _ int) { s.Processes.Info.Cpus = nil })
		}},
		{"regression-already-installed", "application-update-regression-macos", "already equals regression version", func(t *testing.T, r Request) {
			version := r.Scenario.Phases[1].Software["rollout"][0].Version
			rewriteFixtureSamples(t, r, schema.Software, func(s *telemetry.Sample, _ int) {
				for i := range s.Software.Metadata.Software {
					if s.Software.Metadata.Software[i].DisplayName == "Google Chrome" {
						s.Software.Metadata.Software[i].Version = version
					}
				}
			})
		}},
		{"later-phase-memory-overflow", "application-update-regression-macos", "resource capacity", func(_ *testing.T, r Request) {
			r.Scenario.Phases[2].Processes["rollout"][0].Memory = schema.Pattern{Steady: &schema.SteadyPattern{Value: 65536}}
		}},
		{"missing-initial-ap-metrics", "wifi-degradation-macos", "initial network_metrics", func(_ *testing.T, r Request) {
			delete(r.Scenario.Phases[0].NetworkMetrics, r.Scenario.NetworkDevices.AccessPoints[0].Name)
		}},
		{"invalid-nested-host-metadata", "healthy-macos", "invalid gohai", func(t *testing.T, r Request) {
			rewriteFixtureSamples(t, r, schema.HostMetadata, func(s *telemetry.Sample, _ int) { s.HostMetadata.Gohai = `{"cpu":"value"}` })
		}},
		{"missing-wireless-identity", "wifi-degradation-macos", "wireless identity tags", func(t *testing.T, r Request) {
			rewriteFixtureSamples(t, r, schema.Metrics, func(s *telemetry.Sample, _ int) {
				for _, metric := range s.Metrics {
					metric.Tags = tagset.CompositeTags{}
				}
			})
		}},
		{"selector-missing-in-one-cycle", "vpn-degradation-windows", "lacks required", func(t *testing.T, r Request) {
			rewriteFixtureSamples(t, r, schema.Connections, func(s *telemetry.Sample, index int) {
				if index == 1 {
					s.Connections.Connections[0].Raddr.Port++
				}
			})
		}},
		{"metric-missing-in-one-cycle", "wifi-degradation-macos", "lacks required", func(t *testing.T, r Request) {
			rewriteFixtureSamples(t, r, schema.Metrics, func(s *telemetry.Sample, index int) {
				if index == 1 {
					s.Metrics = slices.DeleteFunc(s.Metrics, func(serie *metrics.Serie) bool { return serie.Name == "system.wlan.rssi" })
				}
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := shippedRequest(t, tc.scenario)
			tc.mutate(t, request)
			sink := &recordingDelivery{start: request.Plan.Start}
			result, err := Run(context.Background(), request, Options{Workers: 4, QueueCapacity: 1, Clock: &advancingClock{now: request.Plan.Start}, Delivery: sink})
			if err == nil || !strings.Contains(err.Error(), tc.want) || result != nil || sink.calls != 0 || sink.waits != 0 {
				t.Fatalf("semantic preflight failed to reject %s before delivery: %v", tc.name, err)
			}
		})
	}
}
