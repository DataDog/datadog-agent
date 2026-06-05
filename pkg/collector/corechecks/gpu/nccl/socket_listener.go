// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nccl

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// maxPendingEvents bounds the in-memory buffer between two Drain() calls.
// At default check interval (15s), a typical NCCL job emits ~100s of events
// per second per rank; this cap covers ~100x typical with no growth, and
// caps total buffer memory at a few tens of MB. When exceeded, the newest
// events are dropped and `dropped` is incremented so operators can see it
// via the datadog.agent.nccl.events_dropped telemetry counter.
const maxPendingEvents = 100_000

// SocketListener listens on a Unix domain socket for NCCL inspector events.
// Each connecting process (one per rank) gets a persistent connection.
// The host PID of each sender is obtained from the kernel via SO_PEERCRED,
// enabling correct container/pod tag correlation without PID namespace issues.
type SocketListener struct {
	listener   *net.UnixListener
	socketPath string
	mu         sync.Mutex
	pending    []ParsedEvent
	parseErrs  uint64 // count of failed parses; reset by Drain()
	dropped    uint64 // count of events dropped due to maxPendingEvents cap; reset by DrainDropped()
	stopCh     chan struct{}
	conns      map[*net.UnixConn]struct{} // in-flight conns, closed on Stop
	wg         sync.WaitGroup             // tracks run() + handleConn() goroutines
}

// newSocketListener creates and starts a UDS listener at socketPath.
//
// Restart/crash recovery flow:
//  1. DialTimeout probe: if another live listener owns the path, return an
//     error so callers (like "agent check nccl") fall back without
//     disrupting it.
//  2. os.Remove: clean up the stale socket file from a prior process that
//     didn't Stop() cleanly (crash, SIGKILL, OOM).
//  3. ListenUnix: bind a fresh listener at the path.
//  4. Chmod 0o722: allow non-root NCCL plugin clients to connect (write-only
//     for group/others; the listener side requires only owner permissions).
//
// NCCL plugin clients auto-reconnect on EPIPE (see inspector_dd_socket.cc),
// so agent restarts are transparent to training pods. Up to one check cycle
// worth of in-memory pending events is lost on crash — acceptable for
// observability data, not transactional.
func newSocketListener(socketPath string) (*SocketListener, error) {
	// Check if the socket is already being actively listened on. If a
	// connection succeeds, another check instance owns the socket — do not
	// remove it; return an error so the caller uses file-based collection.
	if conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond); err == nil {
		conn.Close()
		return nil, fmt.Errorf("socket %s already in use by another instance; using file-based collection", socketPath)
	}

	// Remove stale socket if it exists
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to remove stale socket %s: %w", socketPath, err)
	}

	addr, err := net.ResolveUnixAddr("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve unix addr %s: %w", socketPath, err)
	}

	ln, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", socketPath, err)
	}

	// Allow training pods (running as non-root) to connect
	if err := os.Chmod(socketPath, 0722); err != nil {
		ln.Close()
		return nil, fmt.Errorf("failed to chmod socket %s: %w", socketPath, err)
	}

	sl := &SocketListener{
		listener:   ln,
		socketPath: socketPath,
		stopCh:     make(chan struct{}),
		conns:      make(map[*net.UnixConn]struct{}),
	}
	sl.wg.Add(1)
	go sl.run()
	log.Infof("NCCL socket listener started at %s", socketPath)
	return sl, nil
}

func (sl *SocketListener) run() {
	defer sl.wg.Done()
	for {
		conn, err := sl.listener.AcceptUnix()
		if err != nil {
			select {
			case <-sl.stopCh:
				return
			default:
				log.Debugf("NCCL socket accept error: %v", err)
				continue
			}
		}
		sl.mu.Lock()
		select {
		case <-sl.stopCh:
			sl.mu.Unlock()
			conn.Close()
			return
		default:
		}
		sl.conns[conn] = struct{}{}
		sl.wg.Add(1)
		sl.mu.Unlock()
		go sl.handleConn(conn)
	}
}

// connIdleTimeout bounds how long a connection can be silent before we
// drop it. NCCL plugins emit at least once per training step; minutes of
// silence means the producer is gone (pod terminated, network broken)
// and we should reclaim the goroutine + conn slot.
const connIdleTimeout = 5 * time.Minute

func (sl *SocketListener) handleConn(conn *net.UnixConn) {
	defer sl.wg.Done()
	defer func() {
		conn.Close()
		sl.mu.Lock()
		delete(sl.conns, conn)
		sl.mu.Unlock()
	}()

	// Get the host PID of the connecting process via SO_PEERCRED.
	// This is the process's PID in the HOST namespace (not the container namespace),
	// so it works directly with workloadmeta for container/pod tag correlation.
	hostPID := getPeerPID(conn)
	log.Debugf("NCCL socket: new connection from host PID %d", hostPID)

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		// Refresh the read deadline before every Scan so an active
		// stream stays open indefinitely but a silent one is dropped.
		_ = conn.SetReadDeadline(time.Now().Add(connIdleTimeout))
		if !scanner.Scan() {
			break
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		event, err := parseEvent(line)
		if err != nil {
			log.Debugf("NCCL socket: failed to parse event: %v", err)
			sl.mu.Lock()
			sl.parseErrs++
			sl.mu.Unlock()
			continue
		}

		if event.CollPerf == nil {
			continue
		}

		sl.mu.Lock()
		if len(sl.pending) >= maxPendingEvents {
			sl.dropped++
		} else {
			sl.pending = append(sl.pending, ParsedEvent{
				Event:     event,
				ParseTime: time.Now(),
				HostPID:   hostPID,
			})
		}
		sl.mu.Unlock()
	}

	if err := scanner.Err(); err != nil {
		log.Debugf("NCCL socket: connection from host PID %d closed: %v", hostPID, err)
	}
}

// getPeerPID returns the host PID of the process on the other end of conn,
// using the kernel's SO_PEERCRED mechanism.
func getPeerPID(conn *net.UnixConn) int {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return 0
	}
	var pid int32
	_ = rawConn.Control(func(fd uintptr) {
		cred, err := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if err == nil {
			pid = cred.Pid
		}
	})
	return int(pid)
}

// Drain returns all buffered events and clears the buffer.
// Called by check.Run() each interval.
func (sl *SocketListener) Drain() []ParsedEvent {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if len(sl.pending) == 0 {
		return nil
	}
	events := sl.pending
	sl.pending = nil
	return events
}

// DrainParseErrors returns the count of parse failures since the last call.
func (sl *SocketListener) DrainParseErrors() uint64 {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	n := sl.parseErrs
	sl.parseErrs = 0
	return n
}

// DrainDropped returns the count of events dropped due to the maxPendingEvents
// cap since the last call.
func (sl *SocketListener) DrainDropped() uint64 {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	n := sl.dropped
	sl.dropped = 0
	return n
}

// Stop shuts down the listener, closes any in-flight connections,
// waits for handlers to finish, and removes the socket file.
func (sl *SocketListener) Stop() {
	sl.mu.Lock()
	select {
	case <-sl.stopCh:
		sl.mu.Unlock()
		return
	default:
		close(sl.stopCh)
	}
	conns := make([]*net.UnixConn, 0, len(sl.conns))
	for c := range sl.conns {
		conns = append(conns, c)
	}
	sl.mu.Unlock()

	sl.listener.Close()
	// Force in-flight handleConn goroutines out of scanner.Scan().
	for _, c := range conns {
		c.Close()
	}
	sl.wg.Wait()

	if err := os.Remove(sl.socketPath); err != nil && !os.IsNotExist(err) {
		log.Debugf("NCCL socket: failed to remove %s: %v", sl.socketPath, err)
	}
}
