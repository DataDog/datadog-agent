// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`): the transport
// comes from commit 4961722de9009afdbbb711fc0adf14a8f6ff9277, and the broadcast
// transport from commit 788233d2ffcc1e9d19b8e8202ca7908b64c64687
// (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

package fitcore

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// broadcastSessionCounter keeps shared-memory names unique even when several
// publishers open in the same instant.
var broadcastSessionCounter atomic.Uint64

func newBroadcastSessionID() uint64 {
	nanos := uint64(time.Now().UnixNano())
	counter := broadcastSessionCounter.Add(1)
	return ((nanos & 0xFFFFFFFF) << 32) | (counter & 0xFFFFFFFF)
}

// Broadcast control frame tags. They are separate from the SPSC handshake
// bytes; only the length-prefixed framing helper is shared.
const (
	broadcastTagJoin         = 1
	broadcastTagReject       = 2
	broadcastTagOffer        = 3
	broadcastTagReady        = 4
	broadcastTagStart        = 5
	broadcastTagUnsubscribe  = 6
	broadcastTagUnsubscribed = 7
)

const (
	maxPendingHandshakes = 8
	acceptPoll           = 10 * time.Millisecond
)

// BroadcastPublisher publishes to every active subscriber. It owns the
// endpoint, the listener, and the shared mapping for the whole session, so
// subscribers may join and leave while it runs.
type BroadcastPublisher struct {
	inner        *broadcastInner
	listener     net.Listener
	done         chan struct{}
	wg           sync.WaitGroup
	closeOnce    sync.Once
	endpointPath string
}

// OpenBroadcastPublisher creates the shared mapping, binds the long-lived
// listener, and starts the control worker. It returns without waiting for a
// subscriber.
func OpenBroadcastPublisher(config BroadcastConfig, protocol ProtocolDescriptor) (*BroadcastPublisher, error) {
	if _, err := config.validate(); err != nil {
		return nil, err
	}
	if err := protocol.validate(); err != nil {
		return nil, err
	}
	if err := checkOSVersion(); err != nil {
		return nil, err
	}
	id := newBroadcastSessionID()
	shared, name, err := createBroadcastMapping(id, config.RingCapacity, protocol.Version, config.MaxSubscribers)
	if err != nil {
		return nil, phase("creating broadcast shared memory", err)
	}
	listener, endpointPath, err := bindBroadcastListener(config.Endpoint)
	if err != nil {
		shared.close()
		return nil, err
	}
	inner := newBroadcastInner(shared, protocol, id, config.RingCapacity, config.MaxSubscribers, name, config.SetupTimeout)
	publisher := &BroadcastPublisher{
		inner:        inner,
		listener:     listener,
		done:         make(chan struct{}),
		endpointPath: endpointPath,
	}
	publisher.wg.Add(1)
	go func() {
		defer publisher.wg.Done()
		publisher.controlLoop()
	}()
	return publisher, nil
}

// SessionID reports the established session identifier.
func (p *BroadcastPublisher) SessionID() uint64 { return p.inner.session }

// SubscriberCount reports the current number of active subscribers. It is
// diagnostic only; a sampled count does not prove publication is safe.
func (p *BroadcastPublisher) SubscriberCount() int { return p.inner.subscriberCount() }

// SendBatch publishes the fitting prefix, blocking for capacity and for a first
// subscriber.
func (p *BroadcastPublisher) SendBatch(records []Record) BroadcastOutcome {
	return p.inner.sendBatch(records, nil, nil)
}

// SendBatchContext publishes while allowing the caller's context to cancel a
// blocked send. A cancelled outcome reports exactly how many records were
// already published.
func (p *BroadcastPublisher) SendBatchContext(ctx context.Context, records []Record) BroadcastOutcome {
	if ctx == nil {
		return p.inner.sendBatch(records, nil, nil)
	}
	if ctx.Err() != nil {
		return BroadcastOutcome{Rejection: RejectionNone, Cancelled: true}
	}
	reg := newWaitRegistry()
	stop := watchContext(ctx, reg)
	defer stop()
	return p.inner.sendBatch(records, reg, ctx)
}

// Close stops the control worker, unlinks the shared-memory name, and removes
// the owned endpoint. It must not run concurrently with SendBatch.
func (p *BroadcastPublisher) Close() error {
	var first error
	p.closeOnce.Do(func() {
		p.inner.shutdown.Store(true)
		p.inner.mu.Lock()
		p.inner.notifyChangedLocked()
		p.inner.mu.Unlock()
		close(p.done)
		p.listener.Close()
		p.wg.Wait()
		if err := p.inner.shared.unlinkName(); err != nil {
			first = err
		}
		if err := p.inner.shared.close(); err != nil && first == nil {
			first = err
		}
		if p.endpointPath != "" {
			os.Remove(p.endpointPath)
		}
	})
	return first
}

func bindBroadcastListener(endpoint SetupEndpoint) (net.Listener, string, error) {
	if endpoint.isTCP {
		if err := ensureLoopback(endpoint.addr); err != nil {
			return nil, "", err
		}
		listener, err := net.ListenTCP("tcp", net.TCPAddrFromAddrPort(endpoint.addr))
		if err != nil {
			return nil, "", phase("binding TCP setup listener", err)
		}
		return listener, "", nil
	}
	path := endpoint.path
	if err := endpointParentChecks(path); err != nil {
		return nil, "", err
	}
	// Never remove an existing path to make bind succeed.
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, "", phase("binding setup socket", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		listener.Close()
		return nil, "", err
	}
	return listener, path, nil
}

func (p *BroadcastPublisher) controlLoop() {
	for {
		select {
		case <-p.done:
			return
		default:
		}
		if deadlineSetter, ok := p.listener.(interface{ SetDeadline(time.Time) error }); ok {
			_ = deadlineSetter.SetDeadline(time.Now().Add(acceptPoll))
		}
		conn, err := p.listener.Accept()
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			return
		}
		if p.inner.shutdown.Load() {
			conn.Close()
			continue
		}
		if p.inner.handshakes.Load() >= maxPendingHandshakes {
			conn.Close()
			continue
		}
		p.inner.handshakes.Add(1)
		p.wg.Add(1)
		go func(conn net.Conn) {
			defer p.wg.Done()
			defer p.inner.handshakes.Add(-1)
			defer conn.Close()
			deadline := time.Now().Add(p.inner.setupTimeout)
			_ = p.handleControl(conn, deadline)
		}(conn)
	}
}

func (p *BroadcastPublisher) handleControl(conn net.Conn, deadline time.Time) error {
	frame, err := receiveFrame(conn, deadline, nil)
	if err != nil {
		return err
	}
	if len(frame) == 0 {
		return invalid("empty broadcast control frame")
	}
	switch frame[0] {
	case broadcastTagJoin:
		return p.handleJoin(conn, frame, deadline)
	case broadcastTagUnsubscribe:
		return p.handleUnsubscribe(conn, frame, deadline)
	default:
		controlErr := invalid("unexpected broadcast control message")
		rejectFrame(conn, controlErr, deadline)
		return controlErr
	}
}

func (p *BroadcastPublisher) handleJoin(conn net.Conn, hello []byte, deadline time.Time) error {
	body, err := expectedMessage(hello, broadcastTagJoin)
	if err != nil {
		rejectFrame(conn, err, deadline)
		return err
	}
	if err := checkBroadcastContract(body, 1, p.inner.protocol); err != nil {
		rejectFrame(conn, err, deadline)
		return err
	}
	p.inner.mu.Lock()
	slot, generation, err := p.inner.reserveSlotLocked()
	p.inner.mu.Unlock()
	if err != nil {
		rejectFrame(conn, err, deadline)
		return err
	}
	if err := sendFrame(conn, broadcastOfferFrame(p.inner, slot, generation), deadline, nil); err != nil {
		p.inner.mu.Lock()
		p.inner.releaseReservationLocked(slot)
		p.inner.mu.Unlock()
		return err
	}
	ready, err := receiveFrame(conn, deadline, nil)
	if err != nil {
		p.inner.mu.Lock()
		p.inner.releaseReservationLocked(slot)
		p.inner.mu.Unlock()
		return err
	}
	if err := checkBroadcastReady(p.inner, slot, generation, ready); err != nil {
		p.inner.mu.Lock()
		p.inner.releaseReservationLocked(slot)
		p.inner.mu.Unlock()
		rejectFrame(conn, err, deadline)
		return err
	}
	p.inner.mu.Lock()
	activation, err := p.inner.activateSlotLocked(slot, generation)
	p.inner.mu.Unlock()
	if err != nil {
		rejectFrame(conn, err, deadline)
		return err
	}
	if err := sendFrame(conn, broadcastStartFrame(p.inner.session, slot, generation, activation), deadline, nil); err != nil {
		// Activation already happened; the subscription may be live. Retain it
		// conservatively rather than reclaim a possibly active reader.
		return err
	}
	return nil
}

func (p *BroadcastPublisher) handleUnsubscribe(conn net.Conn, frame []byte, deadline time.Time) error {
	body, err := expectedMessage(frame, broadcastTagUnsubscribe)
	if err != nil {
		rejectFrame(conn, err, deadline)
		return err
	}
	if len(body) != 29+16 {
		unsubErr := invalid("Unsubscribe frame length mismatch")
		rejectFrame(conn, unsubErr, deadline)
		return unsubErr
	}
	if err := checkBroadcastContract(body[:29], 1, p.inner.protocol); err != nil {
		rejectFrame(conn, err, deadline)
		return err
	}
	session := binary.BigEndian.Uint64(body[29:37])
	slot := binary.BigEndian.Uint32(body[37:41])
	generation := binary.BigEndian.Uint32(body[41:45])
	if session != p.inner.session {
		sessionErr := invalid("Unsubscribe session identifier mismatch")
		rejectFrame(conn, sessionErr, deadline)
		return sessionErr
	}
	p.inner.mu.Lock()
	err = p.inner.retireSlotLocked(slot, generation)
	p.inner.mu.Unlock()
	if err != nil {
		rejectFrame(conn, err, deadline)
		return err
	}
	return sendFrame(conn, broadcastUnsubscribedFrame(p.inner.session, slot, generation), deadline, nil)
}

func checkBroadcastReady(inner *broadcastInner, slot, generation uint32, frame []byte) error {
	body, err := expectedMessage(frame, broadcastTagReady)
	if err != nil {
		return err
	}
	if len(body) != 16 {
		return invalid("Ready frame length mismatch")
	}
	session := binary.BigEndian.Uint64(body[0:8])
	readySlot := binary.BigEndian.Uint32(body[8:12])
	readyGeneration := binary.BigEndian.Uint32(body[12:16])
	if session != inner.session || readySlot != slot || readyGeneration != generation {
		return invalid("Ready does not match the offered subscription")
	}
	return nil
}

func broadcastOfferFrame(inner *broadcastInner, slot, generation uint32) []byte {
	offer := make([]byte, 0, 1+29+8+9*4+1+len(inner.name))
	offer = append(offer, broadcastTagOffer)
	offer = append(offer, broadcastContractBytes(2, inner.protocol)...)
	offer = append(offer, be64(inner.session)...)
	for _, value := range []uint32{
		uint32(inner.shared.regionLen),
		uint32(inner.shared.ringOffset),
		uint32(inner.capacity),
		recordHeaderSize,
		broadcastSlotStride,
		uint32(inner.maxSubscribers),
		broadcastSlotsOffset,
		slot,
		generation,
	} {
		offer = appendUint32BE(offer, value)
	}
	offer = append(offer, byte(len(inner.name)))
	return append(offer, inner.name...)
}

func broadcastStartFrame(session uint64, slot, generation uint32, activation int) []byte {
	frame := make([]byte, 0, 1+20)
	frame = append(frame, broadcastTagStart)
	frame = append(frame, be64(session)...)
	frame = appendUint32BE(frame, slot)
	frame = appendUint32BE(frame, generation)
	return appendUint32BE(frame, uint32(activation))
}

func broadcastUnsubscribedFrame(session uint64, slot, generation uint32) []byte {
	frame := make([]byte, 0, 1+16)
	frame = append(frame, broadcastTagUnsubscribed)
	frame = append(frame, be64(session)...)
	frame = appendUint32BE(frame, slot)
	return appendUint32BE(frame, generation)
}

// Subscribe joins a running publisher. The subscriber maps the existing
// mapping and starts at its activation boundary, so it never receives history.
func Subscribe(config SubscriberConfig, protocol ProtocolDescriptor) (*Subscription, error) {
	return subscribeBroadcast(nil, config, protocol)
}

// SubscribeContext joins while allowing the caller's context to cancel setup.
func SubscribeContext(ctx context.Context, config SubscriberConfig, protocol ProtocolDescriptor) (*Subscription, error) {
	return subscribeBroadcast(ctx, config, protocol)
}

func subscribeBroadcast(ctx context.Context, config SubscriberConfig, protocol ProtocolDescriptor) (*Subscription, error) {
	if err := ctxCheck(ctx); err != nil {
		return nil, err
	}
	deadline, err := config.validate()
	if err != nil {
		return nil, err
	}
	if err := protocol.validate(); err != nil {
		return nil, err
	}
	if err := checkOSVersion(); err != nil {
		return nil, err
	}
	conn, err := dialBroadcast(ctx, config.Endpoint, deadline)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	hello := append([]byte{broadcastTagJoin}, broadcastContractBytes(1, protocol)...)
	if err := sendFrame(conn, hello, deadline, ctx); err != nil {
		return nil, phase("sending Hello", err)
	}
	offer, err := receiveFrame(conn, deadline, ctx)
	if err != nil {
		return nil, phase("waiting for Offer", err)
	}
	body, err := expectedMessage(offer, broadcastTagOffer)
	if err != nil {
		if len(offer) == 0 || offer[0] != broadcastTagReject {
			rejectFrame(conn, err, deadline)
		}
		return nil, err
	}
	session, shared, slot, generation, err := parseBroadcastOffer(body, protocol)
	if err != nil {
		rejectFrame(conn, err, deadline)
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			shared.close()
		}
	}()
	ready := make([]byte, 0, 1+16)
	ready = append(ready, broadcastTagReady)
	ready = append(ready, be64(session)...)
	ready = appendUint32BE(ready, slot)
	ready = appendUint32BE(ready, generation)
	if err := sendFrame(conn, ready, deadline, ctx); err != nil {
		return nil, phase("sending Ready", err)
	}
	startFrame, err := receiveFrame(conn, deadline, ctx)
	if err != nil {
		return nil, phase("waiting for Start", err)
	}
	startBody, err := expectedMessage(startFrame, broadcastTagStart)
	if err != nil {
		if len(startFrame) == 0 || startFrame[0] != broadcastTagReject {
			rejectFrame(conn, err, deadline)
		}
		return nil, err
	}
	if len(startBody) != 20 {
		startErr := invalid("Start frame length mismatch")
		rejectFrame(conn, startErr, deadline)
		return nil, startErr
	}
	startSession := binary.BigEndian.Uint64(startBody[0:8])
	startSlot := binary.BigEndian.Uint32(startBody[8:12])
	startGeneration := binary.BigEndian.Uint32(startBody[12:16])
	if startSession != session || startSlot != slot || startGeneration != generation {
		startErr := invalid("Start does not match the offered subscription")
		rejectFrame(conn, startErr, deadline)
		return nil, startErr
	}
	keep = true
	return &Subscription{
		shared:       shared,
		protocol:     protocol,
		session:      session,
		slot:         slot,
		generation:   generation,
		capacity:     shared.capacity,
		endpoint:     config.Endpoint,
		setupTimeout: config.SetupTimeout,
	}, nil
}

func dialBroadcast(ctx context.Context, endpoint SetupEndpoint, deadline time.Time) (net.Conn, error) {
	if endpoint.isTCP {
		if err := ensureLoopback(endpoint.addr); err != nil {
			return nil, err
		}
		return dialTCP(endpoint.addr, deadline, ctx)
	}
	return dialUnix(endpoint.path, deadline, ctx)
}

func parseBroadcastOffer(body []byte, protocol ProtocolDescriptor) (uint64, *mapping, uint32, uint32, error) {
	contractLen := len(broadcastContractBytes(2, protocol))
	if len(body) < contractLen+8+9*4+1 {
		return 0, nil, 0, 0, invalid("Offer is truncated")
	}
	if err := checkBroadcastContract(body[:contractLen], 2, protocol); err != nil {
		return 0, nil, 0, 0, err
	}
	tail := body[contractLen:]
	session := binary.BigEndian.Uint64(tail[0:8])
	index := 8
	readU32 := func() uint32 {
		value := binary.BigEndian.Uint32(tail[index : index+4])
		index += 4
		return value
	}
	regionSize := readU32()
	ringOffsetValue := readU32()
	capacity := readU32()
	_ = readU32() // record header
	slotStride := readU32()
	maxSubscribers := readU32()
	slotsOffset := readU32()
	slot := readU32()
	generation := readU32()
	if slotStride != broadcastSlotStride || slotsOffset != broadcastSlotsOffset {
		return 0, nil, 0, 0, invalid("Offer broadcast layout mismatch")
	}
	nameLen := int(tail[index])
	index++
	if len(tail) != index+nameLen {
		return 0, nil, 0, 0, invalid("Offer shared-memory name length mismatch")
	}
	name := string(tail[index:])
	shared, err := openBroadcastMapping(name, session, int(capacity), protocol.Version, int(maxSubscribers), int(regionSize), int(ringOffsetValue))
	if err != nil {
		return 0, nil, 0, 0, phase("opening offered shared memory", err)
	}
	return session, shared, slot, generation, nil
}

// Unsubscribe performs a graceful leave. After it returns the slot no longer
// pins publication. Calling it again is a successful no-op.
func (s *Subscription) Unsubscribe() error {
	if s.unsubscribed {
		return nil
	}
	deadline := time.Now().Add(s.setupTimeout)
	if s.setupTimeout <= 0 {
		deadline = time.Now().Add(DefaultSetupTimeout)
	}
	conn, err := dialBroadcast(nil, s.endpoint, deadline)
	if err != nil {
		return phase("connecting to setup socket", err)
	}
	defer conn.Close()
	frame := make([]byte, 0, 1+29+16)
	frame = append(frame, broadcastTagUnsubscribe)
	frame = append(frame, broadcastContractBytes(1, s.protocol)...)
	frame = append(frame, be64(s.session)...)
	frame = appendUint32BE(frame, s.slot)
	frame = appendUint32BE(frame, s.generation)
	if err := sendFrame(conn, frame, deadline, nil); err != nil {
		return phase("sending Unsubscribe", err)
	}
	ack, err := receiveFrame(conn, deadline, nil)
	if err != nil {
		return phase("waiting for Unsubscribed", err)
	}
	ackBody, err := expectedMessage(ack, broadcastTagUnsubscribed)
	if err != nil {
		return err
	}
	if len(ackBody) != 16 {
		return invalid("Unsubscribed frame length mismatch")
	}
	if binary.BigEndian.Uint64(ackBody[0:8]) != s.session ||
		binary.BigEndian.Uint32(ackBody[8:12]) != s.slot ||
		binary.BigEndian.Uint32(ackBody[12:16]) != s.generation {
		return invalid("Unsubscribed does not match the subscription")
	}
	s.unsubscribed = true
	return nil
}

// Close releases the subscriber mapping. It attempts a graceful unsubscribe
// first; a failure conservatively leaves the slot pinned, so a crashed or
// non-cooperating subscriber can stall publication.
func (s *Subscription) Close() error {
	_ = s.Unsubscribe()
	if s.shared == nil {
		return nil
	}
	err := s.shared.close()
	s.shared = nil
	return err
}
