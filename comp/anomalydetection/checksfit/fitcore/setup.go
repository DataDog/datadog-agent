// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`) at commit
// 4961722de9009afdbbb711fc0adf14a8f6ff9277 (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

package fitcore

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/netip"
	"os"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	maxFrame            = 256
	cancelCheckInterval = 50 * time.Millisecond
)

func phase(name string, err error) error {
	return fmt.Errorf("%s: %w", name, err)
}

func setupDeadlineExpired() error {
	return fmt.Errorf("setup deadline expired: %w", os.ErrDeadlineExceeded)
}

// remaining returns the time left before the setup deadline, or a timeout
// error once it passed.
func remaining(deadline time.Time) (time.Duration, error) {
	rest := time.Until(deadline)
	if rest <= 0 {
		return 0, setupDeadlineExpired()
	}
	return rest, nil
}

func ctxCheck(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return cancelled(err)
	}
	return nil
}

// sleepFor sleeps for the duration, or until the context fires.
func sleepFor(ctx context.Context, d time.Duration) error {
	if ctx == nil {
		time.Sleep(d)
		return nil
	}
	select {
	case <-ctx.Done():
		return cancelled(ctx.Err())
	case <-time.After(d):
		return nil
	}
}

func isTimeout(err error) bool {
	return errors.Is(err, os.ErrDeadlineExceeded)
}

// writeAllUntil writes the whole buffer, slicing the deadline into
// cancellation-check intervals when a context is supplied.
func writeAllUntil(conn net.Conn, buf []byte, deadline time.Time, ctx context.Context) error {
	for len(buf) > 0 {
		if err := ctxCheck(ctx); err != nil {
			return err
		}
		rest, err := remaining(deadline)
		if err != nil {
			return err
		}
		slice := rest
		if ctx != nil && slice > cancelCheckInterval {
			slice = cancelCheckInterval
		}
		_ = conn.SetWriteDeadline(time.Now().Add(slice))
		n, werr := conn.Write(buf)
		buf = buf[n:]
		if werr == nil {
			continue
		}
		if isTimeout(werr) {
			if slice < rest {
				continue // cancellation-check slice elapsed
			}
			return setupDeadlineExpired()
		}
		return werr
	}
	return nil
}

// readExactUntil reads exactly len(buf) bytes, treating a closed peer as a
// setup failure rather than a valid frame boundary.
func readExactUntil(conn net.Conn, buf []byte, deadline time.Time, ctx context.Context) error {
	for len(buf) > 0 {
		if err := ctxCheck(ctx); err != nil {
			return err
		}
		rest, err := remaining(deadline)
		if err != nil {
			return err
		}
		slice := rest
		if ctx != nil && slice > cancelCheckInterval {
			slice = cancelCheckInterval
		}
		_ = conn.SetReadDeadline(time.Now().Add(slice))
		n, rerr := conn.Read(buf)
		buf = buf[n:]
		if rerr == nil {
			continue
		}
		if errors.Is(rerr, io.EOF) {
			return fmt.Errorf("setup connection closed: %w", io.ErrUnexpectedEOF)
		}
		if isTimeout(rerr) {
			if slice < rest {
				continue // cancellation-check slice expired
			}
			return setupDeadlineExpired()
		}
		return rerr
	}
	return nil
}

// sendFrame writes one setup control frame: a four-byte unsigned big-endian
// body length followed by 1-256 body bytes whose first byte is the tag.
func sendFrame(conn net.Conn, body []byte, deadline time.Time, ctx context.Context) error {
	if err := ctxCheck(ctx); err != nil {
		return err
	}
	if len(body) == 0 || len(body) > maxFrame {
		return invalid("invalid outgoing setup frame size")
	}
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(len(body)))
	if err := writeAllUntil(conn, prefix[:], deadline, ctx); err != nil {
		return err
	}
	return writeAllUntil(conn, body, deadline, ctx)
}

// receiveFrame reads and bounds one setup control frame.
func receiveFrame(conn net.Conn, deadline time.Time, ctx context.Context) ([]byte, error) {
	var prefix [4]byte
	if err := readExactUntil(conn, prefix[:], deadline, ctx); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(prefix[:])
	if length == 0 || length > maxFrame {
		return nil, invalid(fmt.Sprintf("setup frame length %d exceeds 1..=%d", length, maxFrame))
	}
	body := make([]byte, length)
	if err := readExactUntil(conn, body, deadline, ctx); err != nil {
		return nil, err
	}
	return body, nil
}

// rejectFrame reports a setup failure to the peer, best-effort.
func rejectFrame(conn net.Conn, err error, deadline time.Time) {
	detail := err.Error()
	if len(detail) > maxFrame-1 {
		detail = detail[:maxFrame-1]
	}
	frame := append([]byte{2}, detail...)
	_ = sendFrame(conn, frame, deadline, nil)
}

// expectedMessage checks the frame tag and returns the body after it. A
// Reject frame surfaces the peer's error text.
func expectedMessage(frame []byte, tag byte) ([]byte, error) {
	if len(frame) > 0 && frame[0] == 2 {
		return nil, invalid("peer rejected setup: " + string(frame[1:]))
	}
	if len(frame) == 0 || frame[0] != tag {
		received := "no tag"
		if len(frame) > 0 {
			received = fmt.Sprintf("%d", frame[0])
		}
		return nil, invalid(fmt.Sprintf("expected setup message %d, received %s", tag, received))
	}
	return frame[1:], nil
}

// offerFrame builds the Offer: consumer contract tuple, session id, resource
// bounds, and the shared-memory object name.
func offerFrame(id uint64, name string, capacity int, protocol ProtocolDescriptor) []byte {
	offer := make([]byte, 0, 1+29+8+16+1+len(name))
	offer = append(offer, 3)
	offer = append(offer, contractBytes(2, protocol)...)
	var session [8]byte
	binary.BigEndian.PutUint64(session[:], id)
	offer = append(offer, session[:]...)
	offer = appendUint32BE(offer, uint32(ringOffset+capacity))
	offer = appendUint32BE(offer, uint32(ringOffset))
	offer = appendUint32BE(offer, uint32(capacity))
	offer = appendUint32BE(offer, uint32(recordHeaderSize))
	offer = append(offer, byte(len(name)))
	return append(offer, name...)
}

// parseOffer validates the Offer body and maps the offered resource.
func parseOffer(body []byte, protocol ProtocolDescriptor) (id uint64, shared *mapping, err error) {
	contractLen := len(contractBytes(2, protocol))
	if len(body) < contractLen+8+16+1 {
		return 0, nil, invalid("Offer is truncated")
	}
	if err := checkContract(body[:contractLen], 2, protocol); err != nil {
		return 0, nil, err
	}
	tail := body[contractLen:]
	id = binary.BigEndian.Uint64(tail[:8])
	bounds := tail[8:24]
	capacity := int(binary.BigEndian.Uint32(bounds[8:12]))
	if err := validateCapacity(capacity); err != nil {
		return 0, nil, err
	}
	var expected [16]byte
	binary.BigEndian.PutUint32(expected[0:4], uint32(ringOffset+capacity))
	binary.BigEndian.PutUint32(expected[4:8], uint32(ringOffset))
	binary.BigEndian.PutUint32(expected[8:12], uint32(capacity))
	binary.BigEndian.PutUint32(expected[12:16], recordHeaderSize)
	if !bytes.Equal(bounds, expected[:]) {
		return 0, nil, invalid("Offer resource bounds mismatch")
	}
	nameLen := int(tail[24])
	if len(tail) != 25+nameLen {
		return 0, nil, invalid("Offer shared-memory name length mismatch")
	}
	name := string(tail[25:])
	if !utf8.ValidString(name) {
		return 0, nil, invalid("Offer name is not UTF-8")
	}
	shared, err = openMapping(name, id, capacity, protocol.Version)
	if err != nil {
		return 0, nil, phase("opening offered shared memory", err)
	}
	return id, shared, nil
}

func ensureLoopback(addr netip.AddrPort) error {
	if addr.Addr().IsLoopback() {
		return nil
	}
	return invalid("TCP setup address must use a loopback IP")
}

// acceptUntil waits for one connection, bounded by the setup deadline and
// the cancellation context. A TCP listener additionally requires the peer
// to come from loopback.
func acceptUntil(listener net.Listener, deadline time.Time, ctx context.Context, requireLoopback bool) (net.Conn, error) {
	type acceptResult struct {
		conn net.Conn
		err  error
	}
	results := make(chan acceptResult, 1)
	go func() {
		conn, err := listener.Accept()
		results <- acceptResult{conn: conn, err: err}
	}()
	var ctxDone <-chan struct{}
	if ctx != nil {
		ctxDone = ctx.Done()
	}
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case result := <-results:
		if result.err != nil {
			return nil, result.err
		}
		if requireLoopback {
			peer, ok := result.conn.RemoteAddr().(*net.TCPAddr)
			if !ok || !peer.IP.IsLoopback() {
				result.conn.Close()
				return nil, fmt.Errorf("TCP setup connection did not come from loopback: %w", fs.ErrPermission)
			}
		}
		return result.conn, nil
	case <-timer.C:
		listener.Close()
		result := <-results
		if result.conn != nil {
			result.conn.Close()
		}
		return nil, setupDeadlineExpired()
	case <-ctxDone:
		listener.Close()
		result := <-results
		if result.conn != nil {
			result.conn.Close()
		}
		return nil, cancelled(ctx.Err())
	}
}

// dialUnix connects once to the consumer's pathname socket. Connection-refused
// and similar errors are one-shot, like the Rust template; only a deadline
// slice that expired for cancellation checking is retried.
func dialUnix(path string, deadline time.Time, ctx context.Context) (net.Conn, error) {
	if len(path) >= 104 || strings.IndexByte(path, 0) >= 0 {
		return nil, invalid("invalid or too-long socket path")
	}
	for {
		if err := ctxCheck(ctx); err != nil {
			return nil, err
		}
		rest, err := remaining(deadline)
		if err != nil {
			return nil, err
		}
		slice := rest
		if ctx != nil && slice > cancelCheckInterval {
			slice = cancelCheckInterval
		}
		conn, derr := net.DialTimeout("unix", path, slice)
		if derr == nil {
			return conn, nil
		}
		if isTimeout(derr) && ctx != nil {
			continue // cancellation-check slice expired; recheck and retry
		}
		return nil, derr
	}
}

// dialTCP connects to the loopback TCP setup endpoint, retrying
// connection-refused errors (and deadline slices while cancellable) until the
// setup deadline, so independently started peers can race during listener
// startup.
func dialTCP(addr netip.AddrPort, deadline time.Time, ctx context.Context) (net.Conn, error) {
	for {
		if err := ctxCheck(ctx); err != nil {
			return nil, err
		}
		rest, err := remaining(deadline)
		if err != nil {
			return nil, err
		}
		slice := rest
		if ctx != nil && slice > cancelCheckInterval {
			slice = cancelCheckInterval
		}
		conn, derr := net.DialTimeout("tcp", addr.String(), slice)
		if derr == nil {
			return conn, nil
		}
		refused := false
		var opErr *net.OpError
		if errors.As(derr, &opErr) {
			refused = errors.Is(opErr.Err, syscall.ECONNREFUSED)
		}
		if refused || (ctx != nil && isTimeout(derr)) {
			if _, rerr := remaining(deadline); rerr != nil {
				return nil, rerr
			}
			if serr := sleepFor(ctx, 10*time.Millisecond); serr != nil {
				return nil, serr
			}
			continue
		}
		return nil, derr
	}
}

// openConsumer accepts one producer and initializes its shared queue.
func openConsumer(ctx context.Context, config ConsumerConfig, protocol ProtocolDescriptor) (*Consumer, error) {
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
	var conn net.Conn
	var listener net.Listener
	if config.Endpoint.isTCP {
		if err := ensureLoopback(config.Endpoint.addr); err != nil {
			return nil, err
		}
		listener, err = net.ListenTCP("tcp", net.TCPAddrFromAddrPort(config.Endpoint.addr))
		if err != nil {
			return nil, phase("binding TCP setup listener", err)
		}
		conn, err = acceptUntil(listener, deadline, ctx, true)
		if err != nil {
			listener.Close()
			return nil, err
		}
	} else {
		path := config.Endpoint.path
		if err := endpointParentChecks(path); err != nil {
			return nil, err
		}
		// Never remove an existing path to make bind succeed.
		listener, err = net.Listen("unix", path)
		if err != nil {
			return nil, phase("binding setup socket", err)
		}
		// The consumer owns the socket and removes it on every exit.
		defer os.Remove(path)
		if err := os.Chmod(path, 0o600); err != nil {
			listener.Close()
			return nil, err
		}
		conn, err = acceptUntil(listener, deadline, ctx, false)
		if err != nil {
			listener.Close()
			return nil, err
		}
	}
	hello, err := receiveFrame(conn, deadline, ctx)
	if err != nil {
		conn.Close()
		listener.Close()
		return nil, phase("waiting for Hello", err)
	}
	contractErr := func() error {
		body, err := expectedMessage(hello, 1)
		if err != nil {
			return err
		}
		return checkContract(body, 1, protocol)
	}()
	if contractErr != nil {
		rejectFrame(conn, contractErr, deadline)
		conn.Close()
		listener.Close()
		return nil, contractErr
	}
	id := uint64(time.Now().UnixNano()) ^ uint64(os.Getpid())
	shared, name, err := createMapping(id, config.RingCapacity, protocol.Version)
	if err != nil {
		conn.Close()
		listener.Close()
		return nil, phase("creating shared memory", err)
	}
	keep := false
	defer func() {
		if !keep {
			shared.close()
		}
	}()
	if err := sendFrame(conn, offerFrame(id, name, config.RingCapacity, protocol), deadline, ctx); err != nil {
		conn.Close()
		listener.Close()
		return nil, phase("sending Offer", err)
	}
	ready, err := receiveFrame(conn, deadline, ctx)
	if err != nil {
		conn.Close()
		listener.Close()
		return nil, phase("waiting for Ready", err)
	}
	readyErr := func() error {
		body, err := expectedMessage(ready, 4)
		if err != nil {
			return err
		}
		if !bytes.Equal(body, be64(id)) {
			return invalid("Ready session identifier mismatch")
		}
		return nil
	}()
	if readyErr != nil {
		if len(ready) == 0 || ready[0] != 2 {
			rejectFrame(conn, readyErr, deadline)
		}
		conn.Close()
		listener.Close()
		return nil, readyErr
	}
	if err := shared.unlinkName(); err != nil {
		conn.Close()
		listener.Close()
		return nil, phase("unlinking offered shared memory", err)
	}
	if err := sendFrame(conn, append([]byte{5}, be64(id)...), deadline, ctx); err != nil {
		conn.Close()
		listener.Close()
		return nil, phase("sending Start", err)
	}
	conn.Close()
	listener.Close()
	keep = true
	return &Consumer{id: id, shared: shared, protocol: protocol}, nil
}

// connectProducer connects once to the consumer and initializes a producer
// queue session.
func connectProducer(ctx context.Context, config ProducerConfig, protocol ProtocolDescriptor) (*Producer, error) {
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
	var conn net.Conn
	if config.Endpoint.isTCP {
		if err := ensureLoopback(config.Endpoint.addr); err != nil {
			return nil, err
		}
		conn, err = dialTCP(config.Endpoint.addr, deadline, ctx)
		if err != nil {
			return nil, phase("connecting to TCP setup endpoint", err)
		}
	} else {
		conn, err = dialUnix(config.Endpoint.path, deadline, ctx)
		if err != nil {
			return nil, phase("connecting to setup socket", err)
		}
	}
	if err := sendFrame(conn, append([]byte{1}, contractBytes(1, protocol)...), deadline, ctx); err != nil {
		conn.Close()
		return nil, phase("sending Hello", err)
	}
	offer, err := receiveFrame(conn, deadline, ctx)
	if err != nil {
		conn.Close()
		return nil, phase("waiting for Offer", err)
	}
	body, offerErr := expectedMessage(offer, 3)
	if offerErr != nil {
		if len(offer) == 0 || offer[0] != 2 {
			rejectFrame(conn, offerErr, deadline)
		}
		conn.Close()
		return nil, offerErr
	}
	id, shared, err := parseOffer(body, protocol)
	if err != nil {
		rejectFrame(conn, err, deadline)
		conn.Close()
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			shared.close()
		}
	}()
	if err := sendFrame(conn, append([]byte{4}, be64(id)...), deadline, ctx); err != nil {
		conn.Close()
		return nil, phase("sending Ready", err)
	}
	startFrame, err := receiveFrame(conn, deadline, ctx)
	if err != nil {
		conn.Close()
		return nil, phase("waiting for Start", err)
	}
	startErr := func() error {
		body, err := expectedMessage(startFrame, 5)
		if err != nil {
			return err
		}
		if !bytes.Equal(body, be64(id)) {
			return invalid("Start session identifier mismatch")
		}
		return nil
	}()
	if startErr != nil {
		if len(startFrame) == 0 || startFrame[0] != 2 {
			rejectFrame(conn, startErr, deadline)
		}
		conn.Close()
		return nil, startErr
	}
	conn.Close()
	keep = true
	return &Producer{id: id, shared: shared, protocol: protocol}, nil
}

func be64(id uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, id)
	return b
}
