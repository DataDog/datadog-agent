// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"errors"
	"fmt"
	"io"
	"math"
	"math/bits"
	"net/netip"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"github.com/google/gopacket/layers"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sys/unix"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/nstat"
	processutil "github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	nstatQueryInterval       = time.Second
	nstatPollInterval        = 250 * time.Millisecond
	nstatPendingRemovalTTL   = 2 * darwinLibprocInterval
	nstatDescriptionRetry    = 100 * time.Millisecond
	nstatDescriptionBatch    = 8
	nstatSubscriptionTimeout = 2 * time.Second
	nstatDatagramBufferSize  = 65535
	nstatTCPRTTScale         = 32
	nstatTCPRTTVarianceScale = 16

	tcpStateClosed      = 0
	tcpStateListen      = 1
	tcpStateSynSent     = 2
	tcpStateSynReceived = 3
	tcpStateEstablished = 4
)

// darwinDirectionEvidence ranks direction signals from weakest to strongest.
// Stronger evidence may replace weaker evidence, while equal or weaker
// evidence cannot flip an established direction.
type darwinDirectionEvidence uint8

const (
	directionEvidenceNone darwinDirectionEvidence = iota
	directionEvidencePeer
	directionEvidenceListener
	directionEvidenceTCPState
	directionEvidencePacket
)

type darwinTCPListenerKey struct {
	address netip.Addr
	port    uint16
	family  network.ConnectionFamily
}

var nstatProviders = [...]uint32{
	nstat.ProviderTCPKernel,
	nstat.ProviderTCPUserland,
	nstat.ProviderUDPKernel,
	nstat.ProviderUDPUserland,
}

var nstatTracerTelemetry = struct {
	datagrams          telemetry.Counter
	events             telemetry.Counter
	parserErrors       telemetry.Counter
	kernelErrors       telemetry.Counter
	descriptionErrors  telemetry.Counter
	droppedSources     telemetry.Counter
	removals           telemetry.Counter
	runtimeFailures    telemetry.Counter
	directionConflicts telemetry.Counter
	activeSources      telemetry.Gauge
}{
	datagrams:          telemetryimpl.GetCompatComponent().NewCounter("network_tracer__nstat", "datagrams", nil, "NStat datagrams received"),
	events:             telemetryimpl.GetCompatComponent().NewCounter("network_tracer__nstat", "events", nil, "NStat messages decoded"),
	parserErrors:       telemetryimpl.GetCompatComponent().NewCounter("network_tracer__nstat", "parser_errors", []string{"reason"}, "NStat datagrams rejected"),
	kernelErrors:       telemetryimpl.GetCompatComponent().NewCounter("network_tracer__nstat", "kernel_errors", nil, "Error responses from the NStat kernel control"),
	descriptionErrors:  telemetryimpl.GetCompatComponent().NewCounter("network_tracer__nstat", "description_errors", nil, "NStat source description requests that failed"),
	droppedSources:     telemetryimpl.GetCompatComponent().NewCounter("network_tracer__nstat", "dropped_sources", nil, "NStat sources dropped at the tracking limit"),
	removals:           telemetryimpl.GetCompatComponent().NewCounter("network_tracer__nstat", "removals", []string{"resolution"}, "NStat source removals by identity resolution"),
	runtimeFailures:    telemetryimpl.GetCompatComponent().NewCounter("network_tracer__nstat", "runtime_failures", nil, "Fatal NStat runtime failures"),
	directionConflicts: telemetryimpl.GetCompatComponent().NewCounter("network_tracer__nstat", "direction_conflicts", nil, "Conflicting direction evidence observed"),
	activeSources:      telemetryimpl.GetCompatComponent().NewGauge("network_tracer__nstat", "active_sources", nil, "NStat sources currently tracked"),
}

type nstatControl interface {
	Subscribe(provider uint32) (uint64, error)
	QueryAll() error
	RequestDescription(sourceRef uint64) error
	Poll(timeout time.Duration) (bool, error)
	Receive(buffer []byte) (int, error)
	Close() error
}

type nstatSource struct {
	provider  uint32
	flow      *nstat.Flow
	counts    nstat.Counts
	conn      *network.ConnectionStats
	createdAt time.Time

	descriptionRequested     bool
	nextDescription          time.Time
	removed                  bool
	removedAt                time.Time
	tcpStateObserved         bool
	tcpEstablished           bool
	tcpEstablishedAfterStart bool
	connectSuccessesObserved bool
	afterEnumeration         bool
	closed                   bool
	connectAttempts          uint32
	connectSuccesses         uint32
	direction                network.ConnectionDirection
	directionEvidence        darwinDirectionEvidence
	listenerKey              darwinTCPListenerKey
	listenerIndexed          bool
}

type nstatTracer struct {
	mu sync.Mutex

	config    *config.Config
	control   nstatControl
	sources   map[uint64]*nstatSource
	byCookie  map[uint64]*nstatSource
	tuples    *darwinTupleIndex
	listeners map[darwinTCPListenerKey]map[uint64]struct{}

	// Sources without any flow description take priority over retries for
	// partially described sources. Both queues are FIFO so retries cannot
	// repeatedly overtake an older request.
	descriptionQueue      []uint64
	descriptionRetryQueue []uint64
	descriptionQueued     map[uint64]struct{}

	subscriptionContexts map[uint64]struct{}
	subscriptionReady    chan struct{}
	subscriptionErrors   chan error
	subscriptionOnce     sync.Once
	enumerationComplete  bool

	closeCallback func(*network.ConnectionStats)
	cookieHasher  *cookieHasher
	now           func() time.Time
	subscribed    bool
	started       bool
	stopped       bool
	runtimeErr    error

	runtimeFailureCallback func(error)

	exit        chan struct{}
	stopOnce    sync.Once
	failureOnce sync.Once
	wg          sync.WaitGroup
}

//nolint:unused // The production backend selection is introduced in the integration commit.
func newNStatTracer(cfg *config.Config) (*nstatTracer, error) {
	control, err := nstat.OpenControl()
	if err != nil {
		return nil, err
	}
	tracer := newNStatTracerWithControl(cfg, control)
	if err := tracer.subscribe(); err != nil {
		_ = control.Close()
		return nil, err
	}
	return tracer, nil
}

func newNStatTracerWithControl(cfg *config.Config, control nstatControl) *nstatTracer {
	return &nstatTracer{
		config:               cfg,
		control:              control,
		sources:              make(map[uint64]*nstatSource),
		byCookie:             make(map[uint64]*nstatSource),
		tuples:               newDarwinTupleIndex(),
		listeners:            make(map[darwinTCPListenerKey]map[uint64]struct{}),
		descriptionQueued:    make(map[uint64]struct{}),
		subscriptionContexts: make(map[uint64]struct{}),
		subscriptionReady:    make(chan struct{}),
		subscriptionErrors:   make(chan error, 1),
		cookieHasher:         newCookieHasher(),
		now:                  time.Now,
		exit:                 make(chan struct{}),
	}
}

func (t *nstatTracer) Start(closeCallback func(*network.ConnectionStats)) error {
	if err := t.subscribe(); err != nil {
		return err
	}

	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return io.ErrClosedPipe
	}
	if t.started {
		t.mu.Unlock()
		return errors.New("nstat tracer already started")
	}
	t.started = true
	t.closeCallback = closeCallback
	t.wg.Add(1)
	t.mu.Unlock()

	go t.run()
	timer := time.NewTimer(nstatSubscriptionTimeout)
	defer timer.Stop()
	select {
	case <-t.subscriptionReady:
		return nil
	case err := <-t.subscriptionErrors:
		t.Stop()
		return err
	case <-timer.C:
		t.Stop()
		return errors.New("timed out waiting for NStat provider subscriptions")
	}
}

func (t *nstatTracer) subscribe() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.subscribed {
		return nil
	}
	for _, provider := range nstatProviders {
		if !nstatProviderEnabled(t.config, provider) {
			continue
		}
		context, err := t.control.Subscribe(provider)
		if err != nil {
			return fmt.Errorf("subscribe to nstat provider %d: %w", provider, err)
		}
		if context != 0 {
			t.subscriptionContexts[context] = struct{}{}
		}
	}
	t.subscribed = true
	if len(t.subscriptionContexts) == 0 {
		t.markEnumerationCompleteLocked()
	}
	return nil
}

func (t *nstatTracer) run() {
	defer t.wg.Done()
	buffer := make([]byte, nstatDatagramBufferSize)
	nextQuery := t.now().Add(nstatQueryInterval)

	for {
		select {
		case <-t.exit:
			return
		default:
		}

		timeout := min(nextQuery.Sub(t.now()), nstatPollInterval)
		if timeout < 0 {
			timeout = 0
		}
		ready, err := t.control.Poll(timeout)
		if err != nil {
			if !t.isStopping() {
				t.handleRuntimeFailure(fmt.Errorf("poll nstat control: %w", err))
			}
			return
		}
		if ready {
			if err := t.drainDatagrams(buffer); err != nil {
				if !t.isStopping() {
					t.handleRuntimeFailure(err)
				}
				return
			}
		}

		now := t.now()
		t.queueUnresolvedDescriptions(now)
		t.pumpDescriptionRequests(now)
		t.expirePendingRemovals(now)
		if !now.Before(nextQuery) {
			if err := t.queryAll(); err != nil {
				if !t.isStopping() {
					t.handleRuntimeFailure(fmt.Errorf("query nstat sources: %w", err))
				}
				return
			}
			nextQuery = now.Add(nstatQueryInterval)
		}
	}
}

func (t *nstatTracer) queryAll() error {
	err := t.control.QueryAll()
	if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
		return nil
	}
	return err
}

func (t *nstatTracer) drainDatagrams(buffer []byte) error {
	for {
		n, err := t.control.Receive(buffer)
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				return nil
			}
			return fmt.Errorf("receive nstat datagram: %w", err)
		}
		if n == 0 {
			return io.EOF
		}
		nstatTracerTelemetry.datagrams.Inc()
		events, err := nstat.ParseDatagram(buffer[:n])
		if err != nil {
			if errors.Is(err, nstat.ErrABIMismatch) {
				nstatTracerTelemetry.parserErrors.Inc("abi_mismatch")
				return fmt.Errorf("nstat revision-%d ABI validation failed: %w", nstat.ABIRevision, err)
			}
			nstatTracerTelemetry.parserErrors.Inc("malformed")
			log.Warnf("rejected malformed nstat datagram: %v", err)
		}
		nstatTracerTelemetry.events.Add(float64(len(events)))
		for _, event := range events {
			t.processEvent(event)
		}
	}
}

func (t *nstatTracer) setRuntimeFailureCallback(callback func(error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.runtimeFailureCallback = callback
}

func (t *nstatTracer) handleRuntimeFailure(err error) {
	t.failureOnce.Do(func() {
		log.Errorf("NStat connection tracer failed at runtime: %v", err)
		nstatTracerTelemetry.runtimeFailures.Inc()

		t.mu.Lock()
		t.runtimeErr = err
		closed := make([]*network.ConnectionStats, 0, len(t.sources))
		for sourceRef, source := range t.sources {
			if source.conn == nil {
				t.removeTCPListener(sourceRef, source)
				delete(t.sources, sourceRef)
				delete(t.descriptionQueued, sourceRef)
				continue
			}
			closed = append(closed, t.closeAndRemoveSource(sourceRef, source))
		}
		nstatTracerTelemetry.activeSources.Set(0)
		closeCallback := t.closeCallback
		failureCallback := t.runtimeFailureCallback
		t.mu.Unlock()

		if closeCallback != nil {
			for _, conn := range closed {
				closeCallback(conn)
			}
		}
		if failureCallback != nil {
			failureCallback(err)
		}
	})
}

func (t *nstatTracer) isStopping() bool {
	select {
	case <-t.exit:
		return true
	default:
		return false
	}
}

func (t *nstatTracer) processEvent(event nstat.Event) {
	var closed *network.ConnectionStats

	t.mu.Lock()
	switch event.Kind {
	case nstat.EventError:
		nstatTracerTelemetry.kernelErrors.Inc()
		if _, subscribed := t.subscriptionContexts[event.Context]; subscribed {
			delete(t.subscriptionContexts, event.Context)
			select {
			case t.subscriptionErrors <- fmt.Errorf("NStat provider subscription failed with kernel error %d", event.Error):
			default:
			}
		}
	case nstat.EventSuccess:
		if _, subscribed := t.subscriptionContexts[event.Context]; subscribed {
			delete(t.subscriptionContexts, event.Context)
			if len(t.subscriptionContexts) == 0 {
				t.markEnumerationCompleteLocked()
			}
		}
	case nstat.EventAdded:
		source := t.getSource(event.SourceRef)
		if source != nil {
			if event.Provider != 0 {
				source.provider = event.Provider
			}
			if !source.descriptionRequested {
				source.descriptionRequested = true
				t.queueDescriptionLocked(event.SourceRef, source)
			}
		}
	case nstat.EventDescription, nstat.EventCounts, nstat.EventUpdate:
		source := t.getSource(event.SourceRef)
		if source != nil {
			if event.Flow != nil && source.flow != nil &&
				event.Flow.UniquePID != 0 && source.flow.UniquePID != 0 &&
				event.Flow.UniquePID != source.flow.UniquePID {
				if source.conn != nil {
					closed = t.closeAndRemoveSource(event.SourceRef, source)
				} else {
					t.removeTCPListener(event.SourceRef, source)
					delete(t.sources, event.SourceRef)
					delete(t.descriptionQueued, event.SourceRef)
				}
				source = t.getSource(event.SourceRef)
			}
			t.updateSource(event.SourceRef, source, event)
			if source.removed && source.conn != nil {
				closed = t.closeAndRemoveSource(event.SourceRef, source)
			}
		}
	case nstat.EventRemoved:
		if source := t.sources[event.SourceRef]; source != nil {
			t.removeTCPListener(event.SourceRef, source)
			if source.conn != nil {
				closed = t.closeAndRemoveSource(event.SourceRef, source)
			} else {
				source.removed = true
				source.removedAt = t.now()
				source.descriptionRequested = true
				source.nextDescription = time.Time{}
				t.queueDescriptionLocked(event.SourceRef, source)
			}
		}
	}
	callback := t.closeCallback
	t.mu.Unlock()

	if closed != nil && callback != nil {
		callback(closed)
	}
}

func (t *nstatTracer) queueDescriptionLocked(sourceRef uint64, source *nstatSource) {
	if nstatSourceResolved(source) {
		return
	}
	if _, queued := t.descriptionQueued[sourceRef]; queued {
		return
	}
	if source.flow == nil {
		t.descriptionQueue = append(t.descriptionQueue, sourceRef)
	} else {
		t.descriptionRetryQueue = append(t.descriptionRetryQueue, sourceRef)
	}
	t.descriptionQueued[sourceRef] = struct{}{}
}

func (t *nstatTracer) queueUnresolvedDescriptions(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for sourceRef, source := range t.sources {
		if !nstatSourceResolved(source) && !now.Before(source.nextDescription) {
			t.queueDescriptionLocked(sourceRef, source)
		}
	}
}

func (t *nstatTracer) pumpDescriptionRequests(now time.Time) {
	t.mu.Lock()
	requests := make([]uint64, 0, nstatDescriptionBatch)
	for len(requests) < nstatDescriptionBatch {
		sourceRef, ok := t.popDescriptionRequestLocked()
		if !ok {
			break
		}
		delete(t.descriptionQueued, sourceRef)
		source := t.sources[sourceRef]
		if source == nil || nstatSourceResolved(source) || now.Before(source.nextDescription) {
			continue
		}
		source.descriptionRequested = true
		source.nextDescription = now.Add(nstatDescriptionRetry)
		requests = append(requests, sourceRef)
	}
	t.mu.Unlock()

	for _, sourceRef := range requests {
		if err := t.control.RequestDescription(sourceRef); err != nil {
			nstatTracerTelemetry.descriptionErrors.Inc()
			log.Debugf("nstat description request for source %d failed: %v", sourceRef, err)
			t.mu.Lock()
			if source := t.sources[sourceRef]; source != nil {
				source.nextDescription = time.Time{}
				t.queueDescriptionLocked(sourceRef, source)
			}
			t.mu.Unlock()
		}
	}
}

func (t *nstatTracer) popDescriptionRequestLocked() (uint64, bool) {
	if len(t.descriptionQueue) > 0 {
		sourceRef := t.descriptionQueue[0]
		t.descriptionQueue = t.descriptionQueue[1:]
		return sourceRef, true
	}
	if len(t.descriptionRetryQueue) > 0 {
		sourceRef := t.descriptionRetryQueue[0]
		t.descriptionRetryQueue = t.descriptionRetryQueue[1:]
		return sourceRef, true
	}
	return 0, false
}

func (t *nstatTracer) getSource(sourceRef uint64) *nstatSource {
	if source := t.sources[sourceRef]; source != nil {
		return source
	}
	if len(t.sources) >= int(t.config.MaxTrackedConnections) {
		t.expirePendingRemovalsLocked(t.now())
		if len(t.sources) >= int(t.config.MaxTrackedConnections) {
			nstatTracerTelemetry.droppedSources.Inc()
			return nil
		}
	}
	source := &nstatSource{createdAt: t.now(), afterEnumeration: t.enumerationComplete}
	t.sources[sourceRef] = source
	nstatTracerTelemetry.activeSources.Set(float64(len(t.sources)))
	return source
}

// markEnumerationCompleteLocked records that the subscribe dump finished so
// later sources are treated as post-start connections.
func (t *nstatTracer) markEnumerationCompleteLocked() {
	t.enumerationComplete = true
	t.subscriptionOnce.Do(func() { close(t.subscriptionReady) })
}

func (t *nstatTracer) updateSource(sourceRef uint64, source *nstatSource, event nstat.Event) {
	if event.Provider != 0 {
		source.provider = event.Provider
	}
	if event.Flow != nil {
		if nstat.IsTCPProvider(source.provider) {
			direction, evidence := source.observeTCPState(event.Flow.TCPState)
			t.setSourceDirection(source, direction, evidence)
		}
		source.flow = mergeNStatFlow(source.flow, event.Flow)
		t.syncTCPListener(sourceRef, source)
	}
	if event.Counts != nil {
		source.counts = *event.Counts
		source.connectAttempts = max(source.connectAttempts, event.Counts.ConnectAttempts)
		previousSuccesses := source.connectSuccesses
		source.connectSuccesses = max(source.connectSuccesses, event.Counts.ConnectSuccesses)
		if nstat.IsTCPProvider(source.provider) {
			source.observeConnectSuccesses(previousSuccesses, source.connectSuccesses)
		}
	}
	if source.flow == nil || (!nstat.IsTCPProvider(source.provider) && !nstat.IsUDPProvider(source.provider)) {
		return
	}
	if source.conn == nil {
		if !nstatSourceResolved(source) {
			return
		}
		if !nstatSourceEnabled(t.config, source) {
			return
		}
		source.conn = t.newConnection(sourceRef, source)
	}
	t.applySource(source)
}

func (source *nstatSource) observeTCPState(state uint32) (network.ConnectionDirection, darwinDirectionEvidence) {
	var direction network.ConnectionDirection
	switch state {
	case tcpStateSynSent:
		direction = network.OUTGOING
	case tcpStateSynReceived:
		direction = network.INCOMING
	}
	if !source.tcpStateObserved {
		// Sources seen during subscribe enumeration are a baseline, not a
		// transition. Sources created after subscription readiness count a
		// first ESTABLISHED as a post-start success.
		source.tcpStateObserved = true
		source.tcpEstablished = state >= tcpStateEstablished
		if source.afterEnumeration && source.tcpEstablished {
			source.tcpEstablishedAfterStart = true
		}
		return direction, directionEvidenceTCPState
	}
	if !source.tcpEstablished && state >= tcpStateEstablished {
		source.markTCPEstablishedAfterStart()
	}
	return direction, directionEvidenceTCPState
}

// observeConnectSuccesses treats a first post-enumeration success, or a later
// increase, as a TCP establishment that happened after system-probe start.
func (source *nstatSource) observeConnectSuccesses(previous, current uint32) {
	if !source.connectSuccessesObserved {
		source.connectSuccessesObserved = true
		if source.afterEnumeration && current > 0 {
			source.markTCPEstablishedAfterStart()
		}
		return
	}
	if current > previous {
		source.markTCPEstablishedAfterStart()
	}
}

// markTCPEstablishedAfterStart records that this source established after start.
func (source *nstatSource) markTCPEstablishedAfterStart() {
	source.tcpEstablished = true
	source.tcpEstablishedAfterStart = true
}

func nstatProviderEnabled(cfg *config.Config, provider uint32) bool {
	switch {
	case nstat.IsTCPProvider(provider):
		return cfg.CollectTCPv4Conns || cfg.CollectTCPv6Conns
	case nstat.IsUDPProvider(provider):
		return cfg.CollectUDPv4Conns || cfg.CollectUDPv6Conns
	default:
		return false
	}
}

func nstatSourceEnabled(cfg *config.Config, source *nstatSource) bool {
	if source == nil || source.flow == nil {
		return false
	}
	ipv6 := nstatFlowFamily(source.flow) == network.AFINET6
	switch {
	case nstat.IsTCPProvider(source.provider):
		if ipv6 {
			return cfg.CollectTCPv6Conns
		}
		return cfg.CollectTCPv4Conns
	case nstat.IsUDPProvider(source.provider):
		if ipv6 {
			return cfg.CollectUDPv6Conns
		}
		return cfg.CollectUDPv4Conns
	default:
		return false
	}
}

func nstatSourceResolved(source *nstatSource) bool {
	if source == nil || source.flow == nil {
		return false
	}
	flow := source.flow
	if nstat.IsTCPProvider(source.provider) {
		return flow.Local.Present &&
			flow.Local.Address.IsValid() &&
			!flow.Local.Address.IsUnspecified() &&
			flow.Local.Port != 0 &&
			flow.Remote.Present &&
			flow.Remote.Address.IsValid() &&
			!flow.Remote.Address.IsUnspecified() &&
			flow.Remote.Port != 0
	}
	if nstat.IsUDPProvider(source.provider) {
		return flow.Local.Present && flow.Local.Address.IsValid() && flow.Local.Port != 0
	}
	return false
}

func mergeNStatFlow(current, update *nstat.Flow) *nstat.Flow {
	if current == nil {
		flow := *update
		return &flow
	}
	merged := *current
	if update.UniquePID != 0 {
		merged.UniquePID = update.UniquePID
	}
	if update.EffectiveUniquePID != 0 {
		merged.EffectiveUniquePID = update.EffectiveUniquePID
	}
	if update.PID != 0 {
		merged.PID = update.PID
	}
	if update.EffectivePID != 0 {
		merged.EffectivePID = update.EffectivePID
	}
	if update.InterfaceIndex != 0 {
		merged.InterfaceIndex = update.InterfaceIndex
	}
	merged.TCPState = update.TCPState
	merged.Local = mergeNStatEndpoint(merged.Local, update.Local)
	merged.Remote = mergeNStatEndpoint(merged.Remote, update.Remote)
	return &merged
}

func mergeNStatEndpoint(current, update nstat.Endpoint) nstat.Endpoint {
	if !update.Present {
		return current
	}
	if current.Present {
		if update.Port == 0 {
			update.Port = current.Port
		}
		if current.Address.IsValid() && !current.Address.IsUnspecified() &&
			(!update.Address.IsValid() || update.Address.IsUnspecified()) {
			update.Address = current.Address
		}
	}
	return update
}

func (t *nstatTracer) newConnection(sourceRef uint64, source *nstatSource) *network.ConnectionStats {
	flow := source.flow
	conn := &network.ConnectionStats{
		ConnectionTuple: network.ConnectionTuple{
			Source: processutil.Address{Addr: flow.Local.Address},
			Dest:   processutil.Address{Addr: flow.Remote.Address},
			Pid:    flow.PID,
			SPort:  flow.Local.Port,
			DPort:  flow.Remote.Port,
			Type:   network.UDP,
			Family: network.AFINET,
		},
		Cookie:          sourceRef ^ bits.RotateLeft64(flow.UniquePID, 32),
		Duration:        time.Duration(t.now().UnixNano()),
		LastUpdateEpoch: uint64(t.now().UnixNano()),
		InterfaceIndex:  flow.InterfaceIndex,
	}
	if nstat.IsTCPProvider(source.provider) {
		conn.Type = network.TCP
	}
	conn.Family = nstatFlowFamily(flow)
	t.cookieHasher.Hash(conn)
	t.byCookie[conn.Cookie] = source
	t.tuples.add(conn)
	return conn
}

func nstatFlowFamily(flow *nstat.Flow) network.ConnectionFamily {
	address := flow.Local.Address
	if !address.IsValid() {
		address = flow.Remote.Address
	}
	if address.Is6() {
		return network.AFINET6
	}
	return network.AFINET
}

func (t *nstatTracer) applySource(source *nstatSource) {
	conn := source.conn
	flow := source.flow
	counts := source.counts
	t.syncConnectionTupleFromNStat(conn, flow)
	conn.LastUpdateEpoch = uint64(t.now().UnixNano())
	conn.InterfaceIndex = flow.InterfaceIndex
	conn.Monotonic.RecvBytes = counts.RXBytes
	conn.Monotonic.SentBytes = saturatingSubtract(counts.TXBytes, uint64(counts.TXRetransmittedBytes))
	conn.Monotonic.RecvPackets = counts.RXPackets
	conn.Monotonic.SentPackets = counts.TXPackets

	if conn.Type != network.TCP {
		return
	}
	if t.hasTCPListener(conn) {
		t.setSourceDirection(source, network.INCOMING, directionEvidenceListener)
	}
	t.reconcileSourceDirection(source)
	conn.RTT = scaledMicroseconds(counts.AverageRTT, nstatTCPRTTScale)
	conn.RTTVar = scaledMicroseconds(counts.RTTVariance, nstatTCPRTTVarianceScale)
	if source.tcpEstablishedAfterStart {
		conn.Monotonic.TCPEstablished = 1
	}
	if !source.closed && flow.TCPState == tcpStateClosed &&
		source.connectAttempts > 0 && source.connectSuccesses == 0 {
		t.markTCPClosed(source)
	}
}

func (t *nstatTracer) syncTCPListener(sourceRef uint64, source *nstatSource) {
	key, listening := nstatTCPListenerKey(source)
	if source.listenerIndexed && (!listening || source.listenerKey != key) {
		t.removeTCPListener(sourceRef, source)
	}
	if !listening || source.listenerIndexed {
		return
	}
	sources := t.listeners[key]
	if sources == nil {
		sources = make(map[uint64]struct{})
		t.listeners[key] = sources
	}
	sources[sourceRef] = struct{}{}
	source.listenerKey = key
	source.listenerIndexed = true

	// Listener descriptions can arrive after their accepted connections.
	// Revisit existing sockets once when a listener is first indexed.
	for _, candidate := range t.sources {
		if candidate.conn == nil || candidate.removed || !t.hasTCPListener(candidate.conn) {
			continue
		}
		t.setSourceDirection(candidate, network.INCOMING, directionEvidenceListener)
		t.reconcileSourceDirection(candidate)
	}
}

func (t *nstatTracer) removeTCPListener(sourceRef uint64, source *nstatSource) {
	if source == nil || !source.listenerIndexed {
		return
	}
	sources := t.listeners[source.listenerKey]
	delete(sources, sourceRef)
	if len(sources) == 0 {
		delete(t.listeners, source.listenerKey)
	}
	source.listenerIndexed = false
	source.listenerKey = darwinTCPListenerKey{}
}

func nstatTCPListenerKey(source *nstatSource) (darwinTCPListenerKey, bool) {
	if source == nil || source.removed || source.flow == nil ||
		!nstat.IsTCPProvider(source.provider) ||
		source.flow.TCPState != tcpStateListen ||
		!source.flow.Local.Present ||
		!source.flow.Local.Address.IsValid() ||
		source.flow.Local.Port == 0 {
		return darwinTCPListenerKey{}, false
	}
	return darwinTCPListenerKey{
		address: normalizeDarwinAddress(source.flow.Local.Address),
		port:    source.flow.Local.Port,
		family:  nstatFlowFamily(source.flow),
	}, true
}

func (t *nstatTracer) hasTCPListener(conn *network.ConnectionStats) bool {
	if conn == nil || conn.Type != network.TCP {
		return false
	}
	key := darwinTCPListenerKey{
		address: normalizeDarwinAddress(conn.Source.Addr),
		port:    conn.SPort,
		family:  conn.Family,
	}
	if len(t.listeners[key]) > 0 {
		return true
	}
	if conn.Family == network.AFINET6 {
		key.address = netip.IPv6Unspecified()
	} else {
		key.address = netip.IPv4Unspecified()
	}
	return len(t.listeners[key]) > 0
}

func (t *nstatTracer) setSourceDirection(
	source *nstatSource,
	direction network.ConnectionDirection,
	evidence darwinDirectionEvidence,
) {
	if source == nil || direction == network.UNKNOWN || evidence == directionEvidenceNone {
		return
	}
	if evidence < source.directionEvidence {
		return
	}
	if evidence == source.directionEvidence && source.direction != network.UNKNOWN {
		if source.direction != direction {
			nstatTracerTelemetry.directionConflicts.Inc()
		}
		return
	}
	if source.direction != network.UNKNOWN && source.direction != direction {
		nstatTracerTelemetry.directionConflicts.Inc()
	}
	source.direction = direction
	source.directionEvidence = evidence
	if source.conn != nil {
		source.conn.Direction = direction
	}
}

func (t *nstatTracer) reconcileSourceDirection(source *nstatSource) {
	if source == nil || source.conn == nil || source.removed {
		return
	}
	if source.direction != network.UNKNOWN {
		source.conn.Direction = source.direction
	}
	peerMatch := t.tuples.matchExact(reverseDarwinTuple(source.conn.ConnectionTuple))
	if !peerMatch.matched || peerMatch.ambiguous || peerMatch.cookie == source.conn.Cookie {
		return
	}
	peer := t.byCookie[peerMatch.cookie]
	if peer == nil || peer.conn == nil || peer.removed {
		return
	}
	if source.direction != network.UNKNOWN {
		t.setSourceDirection(peer, oppositeConnectionDirection(source.direction), directionEvidencePeer)
	}
	if peer.direction != network.UNKNOWN {
		t.setSourceDirection(source, oppositeConnectionDirection(peer.direction), directionEvidencePeer)
	}
}

func oppositeConnectionDirection(direction network.ConnectionDirection) network.ConnectionDirection {
	switch direction {
	case network.OUTGOING:
		return network.INCOMING
	case network.INCOMING:
		return network.OUTGOING
	default:
		return network.UNKNOWN
	}
}

func (t *nstatTracer) syncConnectionTupleFromNStat(conn *network.ConnectionStats, flow *nstat.Flow) {
	tupleChanged := false
	if flow.PID != 0 {
		conn.Pid = flow.PID
	}
	if flow.Local.Present {
		if flow.Local.Address.IsValid() && !flow.Local.Address.IsUnspecified() &&
			conn.Source.Addr != flow.Local.Address {
			conn.Source = processutil.Address{Addr: flow.Local.Address}
			tupleChanged = true
		}
		if flow.Local.Port != 0 && conn.SPort != flow.Local.Port {
			conn.SPort = flow.Local.Port
			tupleChanged = true
		}
	}
	if flow.Remote.Present {
		if flow.Remote.Address.IsValid() && !flow.Remote.Address.IsUnspecified() &&
			conn.Dest.Addr != flow.Remote.Address {
			conn.Dest = processutil.Address{Addr: flow.Remote.Address}
			tupleChanged = true
		}
		if flow.Remote.Port != 0 && conn.DPort != flow.Remote.Port {
			conn.DPort = flow.Remote.Port
			tupleChanged = true
		}
	}
	family := network.AFINET
	if conn.Source.Addr.Is6() || conn.Dest.Addr.Is6() {
		family = network.AFINET6
	}
	if conn.Family != family {
		conn.Family = family
		tupleChanged = true
	}
	if tupleChanged {
		t.tuples.add(conn)
	}
}

func (t *nstatTracer) enrichTCPPacket(
	packet network.ConnectionTuple,
	packetOutgoing bool,
	captureTruncated bool,
	tcp *layers.TCP,
	analyzer *darwinPacketAnalyzer,
) darwinTupleMatch {
	t.mu.Lock()
	match := t.tuples.match(packet)
	if !match.matched || match.ambiguous {
		t.mu.Unlock()
		return match
	}
	source := t.byCookie[match.cookie]
	if source == nil || source.conn == nil || source.removed {
		t.mu.Unlock()
		return darwinTupleMatch{}
	}
	if match.packetReversed {
		packetOutgoing = !packetOutgoing
	}
	t.mu.Unlock()

	analysis := analyzer.process(match.cookie, packetOutgoing, captureTruncated, tcp)

	t.mu.Lock()
	currentMatch := t.tuples.match(packet)
	source = t.byCookie[match.cookie]
	if !currentMatch.matched || currentMatch.ambiguous || currentMatch.cookie != match.cookie ||
		source == nil || source.conn == nil || source.removed {
		sourceGone := source == nil || source.conn == nil || source.removed
		t.mu.Unlock()
		if sourceGone {
			analyzer.remove(match.cookie)
			return darwinTupleMatch{}
		}
		return currentMatch
	}
	conn := source.conn
	if analysis.direction != network.UNKNOWN {
		t.setSourceDirection(source, analysis.direction, directionEvidencePacket)
		t.reconcileSourceDirection(source)
	}
	if analysis.retransmits > conn.Monotonic.Retransmits {
		conn.Monotonic.Retransmits = analysis.retransmits
	}
	if analysis.failure {
		if conn.TCPFailures == nil {
			conn.TCPFailures = make(map[uint16]uint32)
		}
		conn.TCPFailures[analysis.failureErrno]++
	}
	conn.ProtocolStack.MergeWith(analysis.protocolStack)
	conn.TLSTags.MergeWith(analysis.tlsTags)
	t.mu.Unlock()
	return currentMatch
}

func (t *nstatTracer) closeAndRemoveSource(sourceRef uint64, source *nstatSource) *network.ConnectionStats {
	t.removeTCPListener(sourceRef, source)
	delete(t.sources, sourceRef)
	delete(t.descriptionQueued, sourceRef)
	nstatTracerTelemetry.activeSources.Set(float64(len(t.sources)))
	nstatTracerTelemetry.removals.Inc("resolved")
	conn := source.conn
	delete(t.byCookie, conn.Cookie)
	t.tuples.remove(conn.Cookie)
	if conn.Type == network.TCP && !source.closed {
		t.markTCPClosed(source)
	}
	if !conn.IsClosed {
		conn.IsClosed = true
		conn.Duration = time.Duration(t.now().UnixNano() - int64(conn.Duration))
	}
	return conn
}

func (t *nstatTracer) markTCPClosed(source *nstatSource) {
	source.closed = true
	source.conn.Monotonic.TCPClosed = 1
	source.conn.IsClosed = true
	source.conn.Duration = time.Duration(t.now().UnixNano() - int64(source.conn.Duration))
}

func (t *nstatTracer) expirePendingRemovals(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.expirePendingRemovalsLocked(now)
}

func (t *nstatTracer) expirePendingRemovalsLocked(now time.Time) {
	for sourceRef, source := range t.sources {
		if source.removed && now.Sub(source.removedAt) >= nstatPendingRemovalTTL {
			t.removeTCPListener(sourceRef, source)
			delete(t.sources, sourceRef)
			delete(t.descriptionQueued, sourceRef)
			nstatTracerTelemetry.removals.Inc("unresolved")
		}
	}
	nstatTracerTelemetry.activeSources.Set(float64(len(t.sources)))
}

func (t *nstatTracer) Stop() {
	if t == nil {
		return
	}
	t.stopOnce.Do(func() {
		t.mu.Lock()
		t.stopped = true
		t.mu.Unlock()
		close(t.exit)
		if err := t.control.Close(); err != nil {
			log.Debugf("closing nstat control failed: %v", err)
		}
		t.wg.Wait()
	})
}

func (t *nstatTracer) GetConnections(buffer *network.ConnectionBuffer, filter func(*network.ConnectionStats) bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.runtimeErr != nil {
		return t.runtimeErr
	}
	connections := make([]network.ConnectionStats, 0, len(t.sources))
	for _, source := range t.sources {
		if source.removed || source.conn == nil {
			continue
		}
		if filter != nil && !filter(source.conn) {
			continue
		}
		conn := *source.conn
		if source.conn.TCPFailures != nil {
			conn.TCPFailures = make(map[uint16]uint32, len(source.conn.TCPFailures))
			for errno, count := range source.conn.TCPFailures {
				conn.TCPFailures[errno] = count
			}
		}
		connections = append(connections, conn)
	}
	buffer.Append(connections)
	return nil
}

func (t *nstatTracer) FlushPending() {}

func (t *nstatTracer) Remove(conn *network.ConnectionStats) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	for sourceRef, source := range t.sources {
		if source.conn != nil && source.conn.Cookie == conn.Cookie {
			t.removeTCPListener(sourceRef, source)
			delete(t.sources, sourceRef)
			delete(t.descriptionQueued, sourceRef)
			delete(t.byCookie, conn.Cookie)
			t.tuples.remove(conn.Cookie)
			nstatTracerTelemetry.activeSources.Set(float64(len(t.sources)))
			break
		}
	}
	return nil
}

func (t *nstatTracer) GetMap(string) (*ebpf.Map, error) { return nil, nil }

func (t *nstatTracer) DumpMaps(_ io.Writer, _ ...string) error {
	return errors.New("not implemented")
}

func (t *nstatTracer) Type() TracerType { return TracerTypeNStat }

func (t *nstatTracer) Pause() error  { return errors.New("not implemented") }
func (t *nstatTracer) Resume() error { return errors.New("not implemented") }

func (t *nstatTracer) Describe(_ chan<- *prometheus.Desc) {}
func (t *nstatTracer) Collect(_ chan<- prometheus.Metric) {}

func saturatingSubtract(value, subtract uint64) uint64 {
	if subtract >= value {
		return 0
	}
	return value - subtract
}

func scaledMicroseconds(value uint32, scale uint32) uint32 {
	result := uint64(value) * 1000 / uint64(scale)
	if result > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(result)
}

var _ Tracer = (*nstatTracer)(nil)
