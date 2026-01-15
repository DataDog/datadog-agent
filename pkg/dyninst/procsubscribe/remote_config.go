// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package procsubscribe hosts ProcessSubscriber implementations that source
// configuration from Remote Config.
package procsubscribe

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe/procscan"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	rcInitialReconnectDelay = 200 * time.Millisecond
	rcMaxReconnectDelay     = 30 * time.Second

	defaultScanInterval = 3 * time.Second
)

// defaultProcessDelays defines the default delays for process discovery.
//
// The 3s delay will capture most processes relatively quickly, but should
// avoid scanning short-lived processes.
//
// The 100s delay will catch processes that start their tracer after 100s which
// will catch processes that start their tracer after 1 minute.
//
// The 1000s will catch extreme outliers that start their tracer really quite
// late.
var defaultProcessDelays = []time.Duration{
	3 * time.Second,
	100 * time.Second,  // a bit more than 1 minute
	1000 * time.Second, // quite a while after the process started
}

type config struct {
	scanInterval   time.Duration
	processDelays  []time.Duration
	processScanner processScanner
	clk            clock.Clock
	jitterFactor   float64
	wait           func(ctx context.Context, duration time.Duration) error
}

var defaultConfig = config{
	scanInterval:  defaultScanInterval,
	processDelays: defaultProcessDelays,
	clk:           clock.New(),
	wait: func(ctx context.Context, duration time.Duration) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(duration):
			return nil
		}
	},
}

// RemoteConfigSubscriber represents the subset of the Remote Config gRPC client
// used by the process subscriber.
type RemoteConfigSubscriber interface {
	CreateConfigSubscription(
		ctx context.Context, opts ...grpc.CallOption,
	) (pbgo.AgentSecure_CreateConfigSubscriptionClient, error)
}

// Subscriber implements the module.ProcessSubscriber interface using Remote
// Config subscription streams to drive process updates.
type Subscriber struct {
	client         RemoteConfigSubscriber
	scanner        processScanner
	clk            clock.Clock
	notifyRequests chan struct{}

	mu struct {
		sync.Mutex
		state           subscriberState
		started         bool
		pendingRequests []*pbgo.ConfigSubscriptionRequest
		callback        func(process.ProcessesUpdate)
	}

	scanInterval time.Duration
	jitterFactor float64
	wait         func(ctx context.Context, duration time.Duration) error

	start sync.Once
	stop  sync.Once

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// processScanner is an interface that allows for the discovery of processes.
//
// It is exported for testing purposes.
type processScanner interface {
	Scan() (
		added []procscan.DiscoveredProcess,
		removed []procscan.ProcessID,
		_ error,
	)
}

// Option configures a RemoteConfigProcessSubscriber.
type Option interface {
	apply(*config)
}

type optionFunc func(*config)

func (f optionFunc) apply(c *config) { f(c) }

// NewSubscriber creates a Subscriber that sources updates directly from Remote
// Config.
func NewSubscriber(
	client RemoteConfigSubscriber,
	opts ...Option,
) *Subscriber {
	cfg := defaultConfig
	for _, opt := range opts {
		opt.apply(&cfg)
	}
	scanner := cfg.processScanner
	if scanner == nil {
		scanner = procscan.NewScanner(kernel.ProcFSRoot(), cfg.processDelays...)
	}
	s := &Subscriber{
		client:         client,
		notifyRequests: make(chan struct{}, 1),
		scanner:        scanner,
		clk:            cfg.clk,
		jitterFactor:   cfg.jitterFactor,
		scanInterval:   cfg.scanInterval,
		wait:           cfg.wait,
	}
	s.mu.state = makeSubscriberState()
	return s
}

// Subscribe registers the callback that will receive process updates.
//
// Must be called before Start. Cannot be called more than once.
func (s *Subscriber) Subscribe(cb func(process.ProcessesUpdate)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mu.callback != nil {
		panic("callback already set")
	}
	if s.mu.started {
		panic("already started")
	}
	s.mu.callback = cb
}

// Start begins delivering updates to the registered callback.
func (s *Subscriber) Start() {
	s.start.Do(func() {
		cbCtx := context.Background()
		ctx, cancel := context.WithCancel(cbCtx)
		s.cancel = cancel

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runScanner(ctx)
		}()

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runStreamManager(ctx)
		}()
	})
}

// Close stops processing updates and releases resources.
func (s *Subscriber) Close() {
	var notStarted bool
	if s.start.Do(func() { notStarted = true }); notStarted {
		s.stop.Do(func() {})
		return
	}
	s.stop.Do(func() {
		defer s.wg.Wait()
		s.mu.Lock()
		defer s.mu.Unlock()
		s.cancel()
	})
}

func (s *Subscriber) runScanner(ctx context.Context) {
	var next time.Duration
	for {
		if err := s.wait(ctx, next); err != nil {
			return
		}

		start := s.clk.Now()
		added, removed, err := s.scanner.Scan()
		if err != nil {
			log.Warnf("process subscriber: scanner error: %v", err)
		} else if len(added) > 0 || len(removed) > 0 {
			if log.ShouldLog(log.TraceLvl) {
				log.Tracef("process subscriber: onScanUpdate: added=%v, removed=%v", added, removed)
			}
			s.withlocked(func(l *lockedSubscriber) {
				l.mu.state.onScanUpdate(added, removed, l)
			})
		} else if log.ShouldLog(log.TraceLvl) {
			log.Tracef("process subscriber: onScanUpdate: no changes")
		}
		// Add a factor of 100 from how long the scan took to ensure that if
		// scanning is slow, that we don't scan too frequently. This should
		// mean we are never scanning for more than 1% of any core time.
		//
		// Generally speaking, scanning should be very fast relative to the
		// interval, so we expect this factor to be small.
		took := s.clk.Since(start)
		interval := s.scanInterval
		interval = interval + 100*took
		jittered := jitter(interval, s.jitterFactor)
		next = jittered
	}
}

func (s *Subscriber) withlocked(fn func(*lockedSubscriber)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn((*lockedSubscriber)(s))
}

type lockedSubscriber Subscriber

// clearPendingRequests implements effects.
func (l *lockedSubscriber) clearPendingRequests() {
	l.mu.pendingRequests = nil
}

// emitUpdate implements effects.
func (l *lockedSubscriber) emitUpdate(update process.ProcessesUpdate) {
	l.mu.callback(update)
}

// track implements effects.
func (l *lockedSubscriber) track(runtimeID string) {
	l.queueRequest(&pbgo.ConfigSubscriptionRequest{
		RuntimeId: runtimeID,
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	})
}

// untrack implements effects.
func (l *lockedSubscriber) untrack(runtimeID string) {
	l.queueRequest(&pbgo.ConfigSubscriptionRequest{
		RuntimeId: runtimeID,
		Action:    pbgo.ConfigSubscriptionRequest_UNTRACK,
	})
}

func (l *lockedSubscriber) queueRequest(req *pbgo.ConfigSubscriptionRequest) {
	l.mu.pendingRequests = append(l.mu.pendingRequests, req)
	select {
	case l.notifyRequests <- struct{}{}:
	default:
	}
}

var _ effects = (*lockedSubscriber)(nil)

func (s *Subscriber) runStreamManager(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // to ensure that the stream is closed
	reconnectDelay := time.Duration(0)

	var lastConnected time.Time
	for {
		if reconnectDelay > 0 {
			log.Debugf("process subscriber: waiting %s before reconnecting", reconnectDelay)
		}
		if err := s.wait(ctx, reconnectDelay); err != nil {
			return
		}

		if lastConnected.IsZero() {
			log.Debugf("connecting to remote config subscription")
		} else {
			log.Debugf("reconnecting to remote config subscription")
		}
		lastConnected = s.clk.Now()
		stream, err := s.client.CreateConfigSubscription(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Warnf(
				"process subscriber: failed to create remote config subscription: %v",
				err,
			)
			reconnectDelay = nextReconnectDelay(reconnectDelay)
			continue
		}
		log.Debug("process subscriber: remote config stream opened")
		s.withlocked(func(l *lockedSubscriber) {
			l.mu.state.onStreamEstablished(l)
		})
		_, err = stream.Header()
		if err != nil {
			log.Warnf("process subscriber: failed to get stream header: %v", err)
			reconnectDelay = nextReconnectDelay(reconnectDelay)
			continue
		}

		err = s.runConnectedStream(ctx, stream)
		if ctx.Err() != nil {
			return
		}
		log.Warnf("process subscriber: remote config stream error: %v", err)
		reconnectDelay = nextReconnectDelay(reconnectDelay)
		if s.clk.Since(lastConnected) > reconnectDelay {
			reconnectDelay = 0
		}
	}
}

func (s *Subscriber) runConnectedStream(
	ctx context.Context,
	stream pbgo.AgentSecure_CreateConfigSubscriptionClient,
) error {
	log.Infof("runConnectedStream started")
	defer func() { log.Infof("runConnectedStream done") }()
	var wg sync.WaitGroup
	defer wg.Wait()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	sendErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	popPendingRequest := func() *pbgo.ConfigSubscriptionRequest {
		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.mu.pendingRequests) == 0 {
			return nil
		}
		req := s.mu.pendingRequests[0]
		s.mu.pendingRequests[0] = nil
		s.mu.pendingRequests = s.mu.pendingRequests[1:]
		return req
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		for {
			for {
				req := popPendingRequest()
				if req == nil {
					break
				}
				if err := stream.Send(req); err != nil {
					sendErr(err)
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-s.notifyRequests:
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			resp, err := stream.Recv()
			if err != nil {
				sendErr(err)
				return
			}
			s.withlocked(func(l *lockedSubscriber) {
				l.mu.state.onStreamConfig(resp, l)
			})
		}
	}()
	go func() { wg.Wait(); close(errCh) }()

	return <-errCh
}

// ProcessReport contains information about a Go process that has been detected
// and is being monitored for Dynamic Instrumentation updates.
type ProcessReport struct {
	RuntimeID    string             `json:"runtime_id"`
	ProcessID    int32              `json:"process_id"`
	Executable   process.Executable `json:"executable"`
	SymDBEnabled bool               `json:"symdb_enabled"`
	Probes       []ProbeInfo        `json:"probes"`
	// ProcessAlive is set if the procscan.Scanner reports the process as alive.
	// This should be true, except for races between the report and the Scanner
	// recently figuring out that a process is dead.
	ProcessAlive bool `json:"process_alive"`
}

// ProbeInfo contains information about a probe for the ProcessReport.
type ProbeInfo struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

// Report is a snapshot of the current state of the subscriber.
type Report struct {
	// Processes contains the state for all the currently tracked processes.
	Processes []ProcessReport `json:"processes"`
	// ProcessesNotTracked contains the PIDs of processes known to the scanner
	// that are not tracked. This should be empty, except for to race conditions
	// between producing this report and the scanner discovering new processes
	// that have not been added to the tracked set yet.
	ProcessesNotTracked []int32 `json:"processes_not_tracked"`
}

// GetReport returns a snapshot of the current state of the subscriber.
func (s *Subscriber) GetReport() Report {
	s.mu.Lock()
	defer s.mu.Unlock()
	liveProcs := map[int32]struct{}{}
	if scanner, ok := s.scanner.(*procscan.Scanner); ok {
		procs := scanner.LiveProcesses()
		for _, proc := range procs {
			liveProcs[int32(proc)] = struct{}{}
		}
	}

	var ret Report
	for _, entry := range s.mu.state.tracked {
		pid := entry.Info.ProcessID.PID
		_, alive := liveProcs[pid]
		pr := ProcessReport{
			RuntimeID:    entry.runtimeID,
			ProcessID:    pid,
			Executable:   entry.Executable,
			SymDBEnabled: entry.symdbEnabled,
			ProcessAlive: alive,
		}
		for _, probe := range entry.probesByPath {
			pr.Probes = append(pr.Probes, ProbeInfo{
				ID:      probe.GetID(),
				Version: probe.GetVersion(),
			})
		}
		ret.Processes = append(ret.Processes, pr)
	}
	// Look for processes known to the scanner that are not tracked. There
	// should be no such processes, modulo race conditions between producing
	// this report and the scanner discovering new processes that have not been
	// added to the tracked set yet.
	for pid := range liveProcs {
		_, ok := s.mu.state.pidToRuntime[pid]
		if ok {
			continue
		}
		ret.ProcessesNotTracked = append(ret.ProcessesNotTracked, pid)
	}
	return ret
}

type parsedRemoteConfigUpdate struct {
	probes        map[string]ir.ProbeDefinition
	haveSymdbFile bool
	symdbEnabled  bool
}

func parseRemoteConfigFiles(
	runtimeID string,
	files []*pbgo.File,
) parsedRemoteConfigUpdate {
	r := parsedRemoteConfigUpdate{
		probes:        make(map[string]ir.ProbeDefinition, len(files)),
		haveSymdbFile: false,
		symdbEnabled:  false,
	}

	for _, file := range files {
		path := file.GetPath()
		if path == "" {
			continue
		}
		cfgPath, err := data.ParseConfigPath(path)
		if err != nil {
			log.Warnf(
				"process subscriber: runtime %s: failed to parse config path %q: %v",
				runtimeID, path, err,
			)
			continue
		}
		switch cfgPath.Product {
		case data.ProductLiveDebugging:
			raw := file.GetRaw()
			if len(raw) == 0 {
				continue
			}
			probe, err := rcjson.UnmarshalProbe(raw)
			if err != nil {
				log.Warnf(
					"process subscriber: runtime %s: failed to parse probe from %q: %v",
					runtimeID, path, err,
				)
				continue
			}
			r.probes[path] = probe
			if log.ShouldLog(log.TraceLvl) {
				log.Tracef(
					"process subscriber: runtime %s parsed probe %s version=%d",
					runtimeID, probe.GetID(), probe.GetVersion(),
				)
			}
		case data.ProductLiveDebuggingSymbolDB:
			r.haveSymdbFile = true
			raw := file.GetRaw()
			if len(raw) == 0 {
				r.symdbEnabled = false
				continue
			}
			var payload struct {
				UploadSymbols bool `json:"upload_symbols"`
			}
			if err := json.Unmarshal(raw, &payload); err != nil {
				log.Warnf(
					"process subscriber: runtime %s: failed to parse symdb payload from %q: %v",
					runtimeID, path, err,
				)
				continue
			}
			r.symdbEnabled = payload.UploadSymbols
		}
	}

	return r
}

func gitInfoFromTags(tags []string) *process.GitInfo {
	var info process.GitInfo
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		switch {
		case strings.HasPrefix(tag, "git.repository_url:"):
			info.RepositoryURL = strings.TrimPrefix(tag, "git.repository_url:")
		case strings.HasPrefix(tag, "git.commit.sha:"):
			info.CommitSha = strings.TrimPrefix(tag, "git.commit.sha:")
		}
	}
	if info == (process.GitInfo{}) {
		return nil
	}
	return &info
}

func containerIDFromTracer(tracer *pbgo.ClientTracer) string {
	if tracer == nil {
		return ""
	}
	containerID := containerIDFromTags(tracer.GetContainerTags())
	if containerID != "" {
		return containerID
	}
	return containerIDFromTags(tracer.GetTags())
}

func containerIDFromTags(tags []string) string {
	const key = "container_id"
	prefix := key + ":"
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if strings.HasPrefix(tag, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(tag, prefix))
		}
	}
	return ""
}

func nextReconnectDelay(current time.Duration) time.Duration {
	next := time.Duration(current.Seconds() * 2 * float64(time.Second))
	if next > rcMaxReconnectDelay {
		return rcMaxReconnectDelay
	}
	if next < rcInitialReconnectDelay {
		return rcInitialReconnectDelay
	}
	return next
}

func jitter(duration time.Duration, fraction float64) time.Duration {
	multiplier := 1 + ((rand.Float64()*2 - 1) * fraction)
	return time.Duration(float64(duration) * multiplier)
}
