// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package live

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	processapi "github.com/DataDog/datadog-agent/pkg/process/util/api"
	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

type coordinatorOptions struct {
	requestTimeout time.Duration
	readinessPoll  time.Duration
	recordPoll     time.Duration
	heartbeat      time.Duration
	cleanupTimeout time.Duration
}

var defaultOptions = coordinatorOptions{
	requestTimeout: 5 * time.Second,
	readinessPoll:  time.Second,
	recordPoll:     100 * time.Millisecond,
	heartbeat:      tc.HeartbeatInterval,
	cleanupTimeout: 10 * time.Second,
}

var (
	fullCommit   = regexp.MustCompile(`^[0-9a-f]{40}$`)
	buildVersion = regexp.MustCompile(`^[0-9][a-zA-Z0-9.+_-]{0,127}$`)
	processID    = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,64}$`)
)

type producer struct {
	client     Client
	identity   tc.Identity
	streams    []tc.Stream
	activation tc.Status
	schedules  []tc.Capability
	activated  atomic.Pointer[tc.Status]
	stop       tc.Status
	attempted  bool
	cursor     uint64
	cycles     map[uint64]bool
}

// Run captures existing output without triggering collection or modifying
// delivery. The caller supplies the overall capture timeout, normally 35 minutes.
func Run(ctx context.Context, platform string, clients []Client, sink Sink) error {
	return run(ctx, platform, clients, sink, defaultOptions)
}

func run(ctx context.Context, platform string, clients []Client, sink Sink, options coordinatorOptions) (result error) {
	if (platform != "macos" && platform != "windows") || sink == nil {
		return errors.New("live capture requires macOS or Windows and an evidence writer")
	}
	selected, err := discover(ctx, platform, clients, options)
	if err != nil {
		return err
	}
	control := tc.Control{ProtocolVersion: tc.ProtocolVersion, SessionID: rand.Text()}
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	var stopHeartbeats func() error
	defer func() {
		if recover() != nil {
			result = errors.New("capture coordinator worker failed")
		}
		if stopHeartbeats != nil {
			_ = stopHeartbeats()
		}
		if result != nil {
			// The caller's context may already be cancelled. Cleanup gets its own
			// bounded opportunity to disarm and acknowledge retained records.
			cleanup, done := context.WithTimeout(context.Background(), options.cleanupTimeout)
			defer done()
			if !cleanupProducers(cleanup, selected, control, options) {
				result = fmt.Errorf("%w; producer cleanup was not fully acknowledged", result)
			}
		}
	}()
	statuses := parallelControl(work, selected, options, func(ctx context.Context, p *producer) (tc.Status, error) {
		p.attempted = true
		return p.client.Prepare(ctx, tc.PrepareRequest{Control: control, Streams: slices.Clone(p.streams)})
	})
	for i, response := range statuses {
		if response.err != nil || validateStatus(response.status, selected[i], control) != nil || response.status.State != tc.Prepared || response.status.FinalSequence != 0 || response.status.Acknowledged != 0 {
			return errors.New("capture producer preparation was not acknowledged")
		}
	}
	stopHeartbeats, heartbeatError := keepAlive(work, cancel, selected, control, options)
	statuses = parallelControl(work, selected, options, func(ctx context.Context, p *producer) (tc.Status, error) {
		return p.client.Activate(ctx, control)
	})
	session := Session{ID: control.SessionID}
	var latest time.Time
	for i, response := range statuses {
		p := selected[i]
		if response.err != nil || validateStatus(response.status, p, control) != nil || response.status.State != tc.Active || response.status.ActivatedAt.IsZero() {
			return errors.New("capture producer activation was not acknowledged")
		}
		p.activation = response.status
		p.activated.Store(&response.status)
		if session.Origin.IsZero() || response.status.ActivatedAt.Before(session.Origin) {
			session.Origin = response.status.ActivatedAt
		}
		if response.status.ActivatedAt.After(latest) {
			latest = response.status.ActivatedAt
		}
		session.Participants = append(session.Participants, Participant{Status: response.status, Streams: slices.Clone(p.streams)})
	}
	if latest.Sub(session.Origin) > 5*time.Second {
		return errors.New("capture activation spread exceeds five seconds")
	}
	if err := sink.Start(work, session); err != nil {
		return errors.New("cannot initialize normalized capture evidence")
	}
	for {
		if err := contextFailure(work, heartbeatError, sink, session); err != nil {
			return err
		}
		if _, err := readRound(work, selected, control, sink, false, options); err != nil {
			if contextErr := contextFailure(work, heartbeatError, sink, session); contextErr != nil {
				return contextErr
			}
			return err
		}
		if complete, _ := sink.Coverage(); complete {
			break
		}
		if err := pause(work, options.recordPoll); err != nil {
			return contextFailure(work, heartbeatError, sink, session)
		}
	}
	statuses = parallelControl(work, selected, options, func(ctx context.Context, p *producer) (tc.Status, error) {
		return p.client.Stop(ctx, control)
	})
	for i, response := range statuses {
		p := selected[i]
		if response.err != nil || validateStatus(response.status, p, control) != nil || !slices.Contains([]tc.State{tc.Stopping, tc.Stopped}, response.status.State) || response.status.StoppedAt.Before(p.activation.ActivatedAt) || response.status.FinalSequence < p.cursor {
			return errors.New("capture producer stop boundary was not acknowledged")
		}
		p.stop = response.status
	}
	for {
		if err := contextFailure(work, heartbeatError, sink, session); err != nil {
			return err
		}
		drained, err := readRound(work, selected, control, sink, true, options)
		if err != nil {
			if contextErr := contextFailure(work, heartbeatError, sink, session); contextErr != nil {
				return contextErr
			}
			return err
		}
		if drained {
			break
		}
		if err := pause(work, options.recordPoll); err != nil {
			return contextFailure(work, heartbeatError, sink, session)
		}
	}
	statuses = parallelControl(work, selected, options, func(ctx context.Context, p *producer) (tc.Status, error) {
		return p.client.Stop(ctx, control)
	})
	stopped := make([]tc.Status, 0, len(selected))
	for i, response := range statuses {
		p := selected[i]
		if response.err != nil || validateStatus(response.status, p, control) != nil || response.status.State != tc.Stopped || response.status.FinalSequence != p.cursor || response.status.Acknowledged != p.cursor || !response.status.StoppedAt.Equal(p.stop.StoppedAt) {
			return errors.New("capture producer final stopped acknowledgement is missing")
		}
		stopped = append(stopped, response.status)
	}
	if err := stopHeartbeats(); err != nil {
		return err
	}
	if err := contextFailure(work, heartbeatError, sink, session); err != nil {
		return err
	}
	if err := sink.Finish(work, stopped, time.Now()); err != nil {
		return errors.New("cannot finalize normalized capture evidence")
	}
	return nil
}

type controlResult struct {
	status tc.Status
	err    error
}

func recoverControl(result *controlResult) {
	if recover() != nil {
		*result = controlResult{err: errors.New("capture producer control worker failed")}
	}
}

func readProducer(ctx context.Context, client Client, request tc.ReadRequest) (batch Batch, err error) {
	defer func() {
		if recover() != nil {
			batch, err = Batch{}, errors.New("capture producer reader failed")
		}
	}()
	return client.Records(ctx, request)
}

func parallelControl(ctx context.Context, producers []*producer, options coordinatorOptions, operation func(context.Context, *producer) (tc.Status, error)) []controlResult {
	results := make([]controlResult, len(producers))
	var workers sync.WaitGroup
	for i, p := range producers {
		workers.Go(func() {
			defer recoverControl(&results[i])
			request, cancel := context.WithTimeout(ctx, options.requestTimeout)
			defer cancel()
			results[i].status, results[i].err = operation(request, p)
		})
	}
	workers.Wait()
	return results
}

func keepAlive(ctx context.Context, fail context.CancelFunc, producers []*producer, control tc.Control, options coordinatorOptions) (func() error, <-chan error) {
	ctx, cancel := context.WithCancel(ctx)
	failures := make(chan error, 1)
	var workers sync.WaitGroup
	for _, p := range producers {
		workers.Go(func() {
			defer func() {
				if recover() != nil {
					select {
					case failures <- errors.New("capture producer heartbeat worker failed"):
					default:
					}
					fail()
				}
			}()
			ticker := time.NewTicker(options.heartbeat)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					request, done := context.WithTimeout(ctx, options.requestTimeout)
					status, err := p.client.Heartbeat(request, control)
					done()
					if ctx.Err() != nil {
						return
					}
					if err != nil || validateStatus(status, p, control) != nil {
						select {
						case failures <- errors.New("capture producer heartbeat failed"):
						default:
						}
						fail()
						return
					}
				}
			}
		})
	}
	return func() error {
		cancel()
		workers.Wait()
		select {
		case err := <-failures:
			return err
		default:
			return nil
		}
	}, failures
}

func readRound(ctx context.Context, producers []*producer, control tc.Control, sink Sink, draining bool, options coordinatorOptions) (bool, error) {
	allDrained := draining
	for _, p := range producers {
		request, done := context.WithTimeout(ctx, options.requestTimeout)
		batch, err := readProducer(request, p.client, tc.ReadRequest{Control: control, Cursor: p.cursor})
		done()
		if err != nil || validateStatus(batch.Status, p, control) != nil || batch.Status.Acknowledged != p.cursor || len(batch.Records) > tc.MaxBatchRecords {
			return false, errors.New("capture producer records could not be read or acknowledged")
		}
		if (!draining && batch.Status.State != tc.Active) || (draining && (!slices.Contains([]tc.State{tc.Stopping, tc.Stopped}, batch.Status.State) || batch.Status.FinalSequence != p.stop.FinalSequence || !batch.Status.StoppedAt.Equal(p.stop.StoppedAt))) {
			return false, errors.New("capture producer changed state outside acknowledged session boundaries")
		}
		var bytes int64
		for i := range batch.Records {
			record := &batch.Records[i]
			size := tc.PayloadSize(record.Payload) + tc.RecordOverhead
			if size < tc.RecordOverhead || size > tc.MaxItemBytes || bytes > tc.MaxBytes-size {
				return false, errors.New("capture producer returned an oversized record batch")
			}
			bytes += size
			if len(p.cycles) >= maxSessionCycles {
				return false, errors.New("capture producer exceeded the session cycle limit")
			}
			if record.ProtocolVersion != tc.ProtocolVersion || record.SessionID != control.SessionID || record.Producer != p.identity || record.Sequence != p.cursor+1 || record.Sequence > batch.Status.FinalSequence || record.CycleID == 0 || p.cycles[record.CycleID] || !slices.Contains(p.streams, record.Stream) || record.Cadence <= 0 || record.CollectedAt.IsZero() || record.ObservedAt.IsZero() || record.CollectedAt.After(record.ObservedAt) || record.ObservedAt.Before(p.activation.ActivatedAt) || record.ObservedAt.After(time.Now().Add(5*time.Second)) || (draining && record.ObservedAt.After(p.stop.StoppedAt)) {
				return false, errors.New("capture record has invalid identity, sequence, cycle, or timing evidence")
			}
			// Queue polling can observe a result whose collection began before
			// activation. Consume and acknowledge it, but never count or persist it.
			if record.CollectedAt.Before(p.activation.ActivatedAt) {
				// Admission still promises a complete group even when the
				// collection boundary excludes it from persisted coverage.
				if record.Stream == tc.Processes || record.Stream == tc.Connections {
					if _, err := processapi.DecodeCaptureGroup(record); err != nil {
						return false, errors.New("capture producer returned an incomplete process or connection group")
					}
				}
			} else {
				if err := sink.Accept(ctx, *record); err != nil {
					return false, errors.New("cannot normalize or persist captured output")
				}
			}
			p.cursor = record.Sequence
			p.cycles[record.CycleID] = true
			*record = tc.Record{} // Release each raw item as soon as it is consumed.
		}
		if draining && (p.cursor != p.stop.FinalSequence || batch.Status.Acknowledged != p.cursor) {
			allDrained = false
		}
	}
	return allDrained, nil
}

func validateStatus(status tc.Status, p *producer, control tc.Control) error {
	if status.ProtocolVersion != tc.ProtocolVersion || status.Producer != p.identity || status.SessionID != control.SessionID || status.Failures != 0 || status.Drops != 0 || status.Acknowledged > status.FinalSequence || !slices.Contains([]tc.State{tc.Prepared, tc.Active, tc.Stopping, tc.Stopped}, status.State) {
		return errors.New("invalid capture producer acknowledgement")
	}
	if activated := p.activated.Load(); activated != nil && status.State != tc.Prepared && !activated.ActivatedAt.Equal(status.ActivatedAt) {
		return errors.New("capture producer changed its activation acknowledgement")
	}
	if status.State != tc.Prepared && (status.ActivatedAt.IsZero() || status.ActivatedAt.After(time.Now().Add(5*time.Second))) {
		// A prepared producer may be stopped during cleanup before activation;
		// that case is handled separately by cleanupProducers.
		return errors.New("invalid capture producer activation boundary")
	}
	if (status.State == tc.Stopping || status.State == tc.Stopped) && (status.StoppedAt.IsZero() || status.StoppedAt.Before(status.ActivatedAt) || status.StoppedAt.After(time.Now().Add(5*time.Second))) {
		return errors.New("invalid capture producer stop boundary")
	}
	for _, stream := range p.streams {
		if !slices.ContainsFunc(status.Capabilities, func(c tc.Capability) bool {
			if c.Stream != stream || c.Cadence <= 0 || !validOwner(p.identity.Role, c) || !validMetricSchedules(c) {
				return false
			}
			if stream == tc.Metrics {
				for _, initial := range p.schedules {
					if initial.Stream == stream && !slices.Equal(initial.MetricSchedules, c.MetricSchedules) {
						return false
					}
				}
			}
			return true
		}) {
			return errors.New("capture producer lost a selected capability")
		}
	}
	return nil
}

func discover(ctx context.Context, platform string, clients []Client, options coordinatorOptions) ([]*producer, error) {
	roles := map[string]bool{}
	for _, client := range clients {
		if client == nil || !slices.Contains([]string{"core-agent", "process-agent", "system-probe"}, client.Role()) || roles[client.Role()] {
			return nil, errors.New("capture requires unique installed producer clients")
		}
		roles[client.Role()] = true
	}
	if !roles["core-agent"] || !roles["process-agent"] {
		return nil, errors.New("capture requires core-agent and process-agent APIs")
	}
	if platform == "macos" {
		clients = slices.DeleteFunc(slices.Clone(clients), func(client Client) bool { return client.Role() == "system-probe" })
	}
	var latest []tc.Status
	for {
		states := make([]controlResult, len(clients))
		var workers sync.WaitGroup
		for i, client := range clients {
			workers.Go(func() {
				defer recoverControl(&states[i])
				request, done := context.WithTimeout(ctx, options.requestTimeout)
				defer done()
				states[i].status, states[i].err = client.Capabilities(request)
			})
		}
		workers.Wait()
		if ctx.Err() != nil {
			return nil, fmt.Errorf("capture readiness incomplete (%s): %w", readinessSummary(platform, latest), ctx.Err())
		}
		available := map[string]*producer{}
		var optionalError error
		latest = nil
		for i, result := range states {
			optional := clients[i].Role() == "system-probe"
			if result.err != nil {
				if errors.Is(result.err, ErrUnavailable) {
					continue
				}
				if errors.Is(result.err, ErrAuthentication) {
					if optional {
						optionalError = ErrAuthentication
						continue
					}
					return nil, ErrAuthentication
				}
				if errors.Is(result.err, ErrIncompatible) {
					if optional {
						optionalError = ErrIncompatible
						continue
					}
					return nil, ErrIncompatible
				}
				if optional {
					optionalError = errors.New("capture producer discovery failed")
					continue
				}
				return nil, errors.New("capture producer discovery failed")
			}
			status := result.status
			if status.ProtocolVersion != tc.ProtocolVersion || status.Producer.Role != clients[i].Role() || !fullCommit.MatchString(status.Producer.Commit) || !buildVersion.MatchString(status.Producer.Version) || !processID.MatchString(status.Producer.InstanceID) || len(status.Capabilities) > 6 {
				if optional {
					optionalError = ErrIncompatible
					continue
				}
				return nil, ErrIncompatible
			}
			seen := map[tc.Stream]bool{}
			invalid := false
			for _, capability := range status.Capabilities {
				if capability.Cadence <= 0 || seen[capability.Stream] || !validOwner(status.Producer.Role, capability) || !validMetricSchedules(capability) {
					invalid = true
					break
				}
				seen[capability.Stream] = true
			}
			if invalid {
				if optional {
					optionalError = ErrIncompatible
					continue
				}
				return nil, ErrIncompatible
			}
			latest = append(latest, status)
			available[clients[i].Role()] = &producer{client: clients[i], identity: status.Producer, activation: status, schedules: status.Capabilities, cycles: map[uint64]bool{}}
		}
		selected, ready := selectProducers(platform, available)
		if ready {
			instances := map[string]bool{}
			for _, p := range selected {
				if instances[p.identity.InstanceID] {
					return nil, ErrIncompatible
				}
				instances[p.identity.InstanceID] = true
			}
			return selected, nil
		}
		if optionalError != nil {
			fallback := available["process-agent"]
			if fallback == nil || !slices.ContainsFunc(fallback.activation.Capabilities, func(c tc.Capability) bool { return c.Stream == tc.Connections }) {
				return nil, optionalError
			}
		}
		if err := pause(ctx, options.readinessPoll); err != nil {
			return nil, fmt.Errorf("capture readiness incomplete (%s): %w", readinessSummary(platform, latest), err)
		}
	}
}

func validOwner(role string, capability tc.Capability) bool {
	switch role {
	case "core-agent":
		return (capability.Stream == tc.Metrics || capability.Stream == tc.Metadata || capability.Stream == tc.AgentInventory || capability.Stream == tc.HostInventory || capability.Stream == tc.HostSystemInfo || capability.Stream == tc.Software) && capability.ConnectionOwner == ""
	case "process-agent":
		return (capability.Stream == tc.Processes && capability.ConnectionOwner == "") || (capability.Stream == tc.Connections && capability.ConnectionOwner == "process")
	case "system-probe":
		return capability.Stream == tc.Connections && capability.ConnectionOwner == "direct"
	default:
		return false
	}
}

// Metric schedules distinguish slow checks from the serializer flush interval.
func validMetricSchedules(capability tc.Capability) bool {
	if capability.Stream != tc.Metrics {
		return len(capability.MetricSchedules) == 0
	}
	if len(capability.MetricSchedules) == 0 || len(capability.MetricSchedules) > 7 {
		return false
	}
	seen := map[string]bool{}
	for _, schedule := range capability.MetricSchedules {
		if tc.MetricCheckFamily(schedule.Family) == "" || schedule.Cadence <= 0 || seen[schedule.Family] {
			return false
		}
		seen[schedule.Family] = true
	}
	return true
}

func selectProducers(platform string, available map[string]*producer) ([]*producer, bool) {
	owner := func(role string, stream tc.Stream) bool {
		p := available[role]
		if p == nil || !slices.ContainsFunc(p.activation.Capabilities, func(c tc.Capability) bool { return c.Stream == stream }) {
			return false
		}
		p.streams = append(p.streams, stream)
		return true
	}
	ready := true
	for _, stream := range []tc.Stream{tc.Metrics, tc.Metadata, tc.AgentInventory, tc.HostInventory, tc.Software} {
		ready = owner("core-agent", stream) && ready
	}
	owner("core-agent", tc.HostSystemInfo)
	ready = owner("process-agent", tc.Processes) && ready
	if platform == "windows" {
		if !owner("system-probe", tc.Connections) {
			ready = owner("process-agent", tc.Connections) && ready
		}
	} else {
		// macOS supports Process Agent connection collection when its tracer is running.
		owner("process-agent", tc.Connections)
	}
	var selected []*producer
	for _, role := range []string{"core-agent", "process-agent", "system-probe"} {
		if p := available[role]; p != nil && len(p.streams) != 0 {
			selected = append(selected, p)
		}
	}
	return selected, ready
}

func readinessSummary(platform string, statuses []tc.Status) string {
	available := map[tc.Stream]time.Duration{}
	for _, status := range statuses {
		for _, capability := range status.Capabilities {
			available[capability.Stream] = capability.Cadence
		}
	}
	required := []tc.Stream{tc.Metrics, tc.Metadata, tc.AgentInventory, tc.HostInventory, tc.Processes, tc.Software}
	if platform == "windows" || available[tc.Connections] > 0 {
		required = append(required, tc.Connections)
	}
	if available[tc.HostSystemInfo] > 0 {
		required = append(required, tc.HostSystemInfo)
	}
	parts := make([]string, 0, len(required))
	for _, stream := range required {
		value := "unavailable"
		if cadence := available[stream]; cadence > 0 {
			value = cadence.String()
		}
		parts = append(parts, string(stream)+"="+value)
	}
	return strings.Join(parts, ", ")
}

func contextFailure(ctx context.Context, heartbeat <-chan error, sink Sink, session Session) error {
	select {
	case err := <-heartbeat:
		return err
	default:
	}
	if ctx.Err() == nil {
		return nil
	}
	_, coverage := sink.Coverage()
	statuses := make([]tc.Status, 0, len(session.Participants))
	for _, participant := range session.Participants {
		statuses = append(statuses, participant.Status)
	}
	return fmt.Errorf("capture incomplete: %s; effective cadences: %s: %w", coverage, readinessSummary("", statuses), ctx.Err())
}

func pause(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func cleanupProducers(ctx context.Context, producers []*producer, control tc.Control, options coordinatorOptions) bool {
	remaining := make([]*producer, 0, len(producers))
	for _, p := range producers {
		if p.attempted {
			remaining = append(remaining, p)
		}
	}
	for len(remaining) > 0 && ctx.Err() == nil {
		results := parallelControl(ctx, remaining, options, func(ctx context.Context, p *producer) (tc.Status, error) {
			return p.client.Stop(ctx, control)
		})
		next := remaining[:0]
		for i, p := range remaining {
			response := results[i]
			if response.err == nil && response.status.ProtocolVersion == tc.ProtocolVersion && response.status.Producer == p.identity && response.status.SessionID == control.SessionID {
				if response.status.State == tc.Failed || (response.status.State == tc.Stopped && response.status.FinalSequence == response.status.Acknowledged) {
					continue
				}
			}
			// Stop may have disarmed successfully while its response was lost.
			// Attempt a read even then, so valid retained data can be discarded
			// and acknowledged without waiting for the producer's lease expiry.
			request, done := context.WithTimeout(ctx, options.requestTimeout)
			batch, err := readProducer(request, p.client, tc.ReadRequest{Control: control, Cursor: p.cursor})
			done()
			if err == nil && batch.Status.ProtocolVersion == tc.ProtocolVersion && batch.Status.Producer == p.identity && batch.Status.SessionID == control.SessionID && batch.Status.Acknowledged == p.cursor && len(batch.Records) <= tc.MaxBatchRecords {
				for j := range batch.Records {
					record := &batch.Records[j]
					if record.Sequence != p.cursor+1 || record.Sequence > batch.Status.FinalSequence {
						break
					}
					p.cursor = record.Sequence
					*record = tc.Record{}
				}
				if batch.Status.State == tc.Stopped && batch.Status.FinalSequence == p.cursor && batch.Status.Acknowledged == p.cursor {
					continue
				}
			}
			next = append(next, p)
		}
		remaining = next
		if len(remaining) != 0 {
			_ = pause(ctx, options.recordPoll)
		}
	}
	return len(remaining) == 0
}
