// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

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

// SocketListener listens on a Unix domain socket for NCCL inspector events.
// Each connecting process (one per rank) gets a persistent connection.
// The host PID of each sender is obtained from the kernel via SO_PEERCRED,
// enabling correct container/pod tag correlation without PID namespace issues.
type SocketListener struct {
	listener *net.UnixListener
	mu       sync.Mutex
	pending  []ParsedEvent
	stopCh   chan struct{}
}

// newSocketListener creates and starts a UDS listener at socketPath.
// Removes any stale socket file before binding. Returns an error (without
// removing the socket) if another instance is already listening on the path,
// so that "agent check nccl" invocations fall back to file-based collection
// without disrupting the running agent's socket listener.
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
		listener: ln,
		stopCh:   make(chan struct{}),
	}
	go sl.run()
	log.Infof("NCCL socket listener started at %s", socketPath)
	return sl, nil
}

func (sl *SocketListener) run() {
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
		go sl.handleConn(conn)
	}
}

func (sl *SocketListener) handleConn(conn *net.UnixConn) {
	defer conn.Close()

	// Get the host PID of the connecting process via SO_PEERCRED.
	// This is the process's PID in the HOST namespace (not the container namespace),
	// so it works directly with workloadmeta for container/pod tag correlation.
	hostPID := getPeerPID(conn)
	log.Debugf("NCCL socket: new connection from host PID %d", hostPID)

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	parseTime := time.Now()
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		event, err := parseEvent(line)
		if err != nil {
			log.Debugf("NCCL socket: failed to parse event: %v", err)
			continue
		}

		if event.CollPerf == nil && event.ProxyOp == nil {
			continue
		}

		sl.mu.Lock()
		sl.pending = append(sl.pending, ParsedEvent{
			Event:     event,
			Filename:  fmt.Sprintf("socket:rank%d-pid%d", event.Rank, event.PID),
			ParseTime: parseTime,
			HostPID:   hostPID,
		})
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

// Stop shuts down the listener and removes the socket file.
func (sl *SocketListener) Stop() {
	close(sl.stopCh)
	sl.listener.Close()
}
