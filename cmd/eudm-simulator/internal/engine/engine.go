// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package engine replays portable captures without starting native collectors.
package engine

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/accesspoint"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/identity"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/overlay"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/report"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

const (
	APMetricStream schema.Stream = "access_point_metrics"
	NDMStream      schema.Stream = "ndm_metadata"
	ndmBatchSize                 = 100
)

var streamOrder = []schema.Stream{schema.Metrics, schema.HostMetadata, schema.Processes, schema.Connections, schema.Software}

type Request struct {
	Scenario *schema.Scenario
	Plan     *schema.RunPlan
	Bundles  map[string]*bundle.Loaded
}

// Delivery returns only after every chunk in its collection cycle is accepted.
// Wait drains remaining tracked delivery. The command uses AgentDelivery.
type Delivery interface {
	Send(context.Context, time.Time, schema.Stream, []*telemetry.Sample) error
	NetworkMetrics(context.Context, []*metrics.Serie) error
	NetworkMetadata(context.Context, []metadata.NetworkDevicesMetadata) error
	Wait(context.Context) error
}

type Options struct {
	Workers, QueueCapacity int
	Clock                  Clock
	Delivery               Delivery
}

type device struct {
	group     schema.GroupDef
	ordinal   int
	id        *identity.Map
	capture   *bundle.Loaded
	timelines map[schema.Stream]*timeline
	wireless  *identity.Wireless
}
type prepared struct {
	request      Request
	duration     time.Duration
	devices      []*device
	accessPoints *accesspoint.Model
	report       *report.Report
}

// Validate performs full fleet preparation without starting delivery or a clock.
func Validate(request Request) error { _, err := prepare(request); return err }

func prepare(request Request) (*prepared, error) {
	if request.Scenario == nil || request.Plan == nil {
		return nil, errors.New("scenario and run plan are required")
	}
	if err := request.Plan.Validate(request.Scenario, request.Plan.ScenarioDigest, request.Plan.AgentCommit); err != nil {
		return nil, err
	}
	if len(request.Bundles) != len(request.Plan.Bundles) {
		return nil, errors.New("supply exactly the run plan's verified bundles")
	}
	for _, ref := range request.Plan.Bundles {
		b := request.Bundles[ref.Digest]
		if b == nil || !reflect.DeepEqual(b.Ref(), ref) {
			return nil, errors.New("assigned bundle differs from run plan")
		}
		for name, digest := range b.Manifest.Files {
			if schema.Digest(b.Files[name]) != digest {
				return nil, errors.New("verified bundle bytes changed before replay")
			}
		}
		for _, ref := range b.Manifest.Samples {
			if _, err := telemetry.Decode(ref.Stream, b.Files[ref.File]); err != nil {
				return nil, fmt.Errorf("invalid captured %s sample: %w", ref.Stream, err)
			}
		}
	}
	aps, err := accesspoint.New(request.Scenario, request.Plan.RunID, request.Plan.Seed)
	if err != nil {
		return nil, err
	}
	p := &prepared{request: request, duration: durationOf(request.Scenario), accessPoints: aps, report: report.New(request.Plan, request.Scenario, runtime.GOOS)}
	for i, assignment := range request.Plan.Assignments {
		group := request.Scenario.Fleet[i]
		b := request.Bundles[assignment.BundleDigest]
		timelines := map[schema.Stream]*timeline{}
		var streams []schema.Stream
		for _, stream := range streamOrder {
			if slices.Contains(b.Manifest.Profile.Streams, stream) {
				timeline, err := makeTimeline(b, stream, p.duration)
				if err != nil {
					return nil, fmt.Errorf("cohort %q: %w", group.Group, err)
				}
				timelines[stream] = timeline
				streams = append(streams, stream)
			}
		}
		if timelines[schema.Processes] == nil {
			return nil, fmt.Errorf("cohort %q lacks a process baseline", group.Group)
		}
		if err := validateRegressionVersions(request.Scenario, group, b); err != nil {
			return nil, err
		}
		if err := validateOverlays(request.Scenario, group, b, timelines); err != nil {
			return nil, err
		}
		wireless, err := aps.Wireless(group)
		if err != nil {
			return nil, err
		}
		for j := 0; j < assignment.Count; j++ {
			ordinal := assignment.FirstOrdinal + j
			id := identity.New(request.Plan.RunID, request.Plan.Seed, group.Group, ordinal)
			d := &device{group: group, ordinal: ordinal, id: id, capture: b, timelines: timelines, wireless: wireless}
			p.devices = append(p.devices, d)
			p.report.AddDevice(ordinal, id.Hostname, group.Group, b.Digest, streams)
			for _, stream := range streams {
				p.report.Ledger[ordinal].Streams[stream].Expected = uint64(timelines[stream].count)
			}
		}
	}
	if len(request.Scenario.NetworkDevices.AccessPoints) > 0 {
		initial, err := aps.Metrics(0, 0, 0, request.Plan.Start)
		if err != nil {
			return nil, fmt.Errorf("access-point metric preflight: %w", err)
		}
		metricHosts := map[string]bool{}
		for _, serie := range initial {
			metricHosts[serie.Host] = true
		}
		p.report.NetworkStreams[APMetricStream] = &report.Counts{Expected: uint64(1 + (p.duration-1)/accesspoint.MetricsCadence)}
		p.report.NetworkStreams[NDMStream] = &report.Counts{Expected: uint64(1 + (p.duration-1)/accesspoint.MetadataCadence)}
		for _, payload := range aps.Metadata(request.Plan.Start, ndmBatchSize) {
			for _, d := range payload.Devices {
				if !metricHosts[d.Name] {
					return nil, errors.New("every declared access point requires initial network_metrics before replay")
				}
				p.report.NetworkDevices = append(p.report.NetworkDevices, d.ID)
			}
		}
	}
	return p, nil
}

func validateRegressionVersions(s *schema.Scenario, g schema.GroupDef, b *bundle.Loaded) error {
	for _, phase := range s.Phases {
		if phase.Name != "onset" && phase.Name != "sustained" {
			continue
		}
		for _, changed := range phase.Software[g.Group] {
			for _, ref := range b.Manifest.Samples {
				if ref.Stream != schema.Software {
					continue
				}
				sample, err := telemetry.Decode(ref.Stream, b.Files[ref.File])
				if err != nil {
					return err
				}
				for _, entry := range sample.Software.Metadata.Software {
					if entry.DisplayName == changed.Name && entry.Version == changed.Version {
						return fmt.Errorf("cohort %q: captured %s version already equals regression version; choose a healthy comparison capture or change the overlay", g.Group, changed.Name)
					}
				}
			}
		}
	}
	return nil
}

type job struct {
	device  *device
	stream  schema.Stream
	ordinal int64
	offset  time.Duration
	rank    int
}
type schedule []job

func (s schedule) Len() int { return len(s) }
func (s schedule) Less(i, j int) bool {
	if s[i].offset != s[j].offset {
		return s[i].offset < s[j].offset
	}
	if s[i].rank != s[j].rank {
		return s[i].rank < s[j].rank
	}
	return s[i].stream < s[j].stream
}
func (s schedule) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s *schedule) Push(v any)   { *s = append(*s, v.(job)) }
func (s *schedule) Pop() any     { v := (*s)[len(*s)-1]; *s = (*s)[:len(*s)-1]; return v }

// Run schedules the complete fleet. Queues bound concurrency and memory only;
// no device or cycle is skipped to reduce work under backpressure.
func Run(ctx context.Context, request Request, options Options) (*report.Report, error) {
	p, err := prepare(request)
	if err != nil {
		return nil, err
	}
	return p.run(ctx, options)
}
func (p *prepared) run(parent context.Context, options Options) (*report.Report, error) {
	r := p.report
	if options.Workers <= 0 || options.QueueCapacity <= 0 || options.Delivery == nil {
		return r, errors.New("positive worker/queue limits and delivery are required")
	}
	if options.Clock == nil {
		options.Clock = WallClock{}
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	r.Status = "running"
	pending := schedule{}
	for _, d := range p.devices {
		for _, stream := range streamOrder {
			if t := d.timelines[stream]; t != nil {
				_, offset := t.at(0)
				heap.Push(&pending, job{device: d, stream: stream, offset: offset, rank: d.ordinal})
			}
		}
	}
	if len(r.NetworkStreams) > 0 {
		heap.Push(&pending, job{stream: APMetricStream, rank: len(p.devices)})
		heap.Push(&pending, job{stream: NDMStream, rank: len(p.devices) + 1})
	}
	jobs := make(chan job, options.QueueCapacity)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstError error
	fail := func(err error) {
		mu.Lock()
		if firstError == nil {
			firstError = err
			r.Errors = append(r.Errors, err.Error())
			cancel()
		}
		mu.Unlock()
	}
	for i := 0; i < options.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for work := range jobs {
				if ctx.Err() != nil {
					continue
				}
				err := p.deliver(ctx, options.Delivery, work)
				mu.Lock()
				counts := r.NetworkStreams[work.stream]
				if work.device != nil {
					counts = r.Ledger[work.device.ordinal].Streams[work.stream]
				}
				if err == nil {
					counts.Delivered++
				} else {
					counts.Failed++
				}
				mu.Unlock()
				if err != nil {
					fail(fmt.Errorf("%s cycle %d delivery: %w", work.stream, work.ordinal, err))
				}
			}
		}()
	}
	scheduling := true
	for len(pending) > 0 && scheduling {
		work := heap.Pop(&pending).(job)
		if err := options.Clock.WaitUntil(ctx, p.request.Plan.Start.Add(work.offset)); err != nil {
			fail(err)
			break
		}
		select {
		case jobs <- work:
		case <-ctx.Done():
			scheduling = false
			continue
		}
		next := work
		next.ordinal++
		if work.device != nil {
			t := work.device.timelines[work.stream]
			if next.ordinal < t.count {
				_, next.offset = t.at(next.ordinal)
				heap.Push(&pending, next)
			}
		} else {
			cadence := accesspoint.MetricsCadence
			if work.stream == NDMStream {
				cadence = accesspoint.MetadataCadence
			}
			if next.ordinal < int64(r.NetworkStreams[work.stream].Expected) {
				next.offset = time.Duration(next.ordinal) * cadence
				heap.Push(&pending, next)
			}
		}
	}
	close(jobs)
	wg.Wait()
	if ctx.Err() == nil {
		if err := options.Clock.WaitUntil(ctx, p.request.Plan.Start.Add(p.duration)); err != nil {
			fail(err)
		}
	}
	if ctx.Err() == nil {
		if err := options.Delivery.Wait(ctx); err != nil {
			fail(err)
		}
	}
	if firstError == nil && parent.Err() != nil {
		fail(parent.Err())
	}
	r.End = options.Clock.Now()
	if firstError == nil && !r.Complete() {
		fail(errors.New("fleet delivery is incomplete"))
	}
	if firstError != nil {
		r.Status = "failed"
		return r, firstError
	}
	r.Status = "succeeded"
	return r, nil
}

func (p *prepared) deliver(ctx context.Context, out Delivery, work job) error {
	at := p.request.Plan.Start.Add(work.offset)
	phase, elapsed := phaseAt(p.request.Scenario, work.offset)
	if work.device == nil {
		if work.stream == NDMStream {
			return out.NetworkMetadata(ctx, p.accessPoints.Metadata(at, ndmBatchSize))
		}
		series, err := p.accessPoints.Metrics(phase, elapsed, work.ordinal, at)
		if err != nil {
			return err
		}
		return out.NetworkMetrics(ctx, series)
	}
	d := work.device
	c, _ := d.timelines[work.stream].at(work.ordinal)
	processCycle, processOrdinal := d.timelines[schema.Processes].nearest(work.offset)
	var baseline []*model.CollectorProc
	for _, ref := range processCycle.refs {
		decoded, err := telemetry.Decode(schema.Processes, d.capture.Files[ref.File])
		if err != nil {
			return err
		}
		baseline = append(baseline, decoded.Processes)
	}
	context := overlay.Context{Scenario: p.request.Scenario, Group: d.group, Seed: p.request.Plan.Seed, DeviceOrdinal: d.ordinal, PhaseIndex: phase, Elapsed: elapsed, Stream: work.stream, SampleOrdinal: work.ordinal, ProcessSampleOrdinal: processOrdinal, BaselineProcesses: baseline}
	samples := make([]*telemetry.Sample, 0, len(c.refs))
	for _, ref := range c.refs {
		sample, err := telemetry.Decode(ref.Stream, d.capture.Files[ref.File])
		if err != nil {
			return err
		}
		if err := overlay.Apply(context, sample); err != nil {
			return err
		}
		rebase(sample, p.request.Plan.Start, work.offset-ref.Offset, work.ordinal, len(c.refs))
		if err := d.id.Apply(sample, d.group, d.wireless); err != nil {
			return err
		}
		samples = append(samples, sample)
	}
	return out.Send(ctx, at, work.stream, samples)
}

func rebase(sample *telemetry.Sample, start time.Time, shift time.Duration, cycle int64, chunks int) {
	for _, series := range sample.Metrics {
		for i := range series.Points {
			series.Points[i].Ts += float64(start.UnixNano())/1e9 + shift.Seconds()
		}
	}
	if process := sample.Processes; process != nil {
		process.GroupId = int32(cycle%2147483647 + 1)
		process.GroupSize = int32(chunks)
		for _, proc := range process.Processes {
			proc.CreateTime += start.UnixMilli()
		}
	}
	if connections := sample.Connections; connections != nil {
		connections.GroupId = int32(cycle%2147483647 + 1)
		connections.GroupSize = int32(chunks)
	}
}
