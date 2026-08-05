// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || darwin

package fdhandoff

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// socketPath returns a path in a per-test directory, short enough to fit in a
// sockaddr_un.
func socketPath(t *testing.T, name string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "fdhandoff")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, name)
}

// openFileDescriptors counts the file descriptors open in this process.
func openFileDescriptors(t *testing.T) int {
	t.Helper()
	count := 0
	for fd := 0; fd < 4096; fd++ {
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err == nil {
			count++
		}
	}
	return count
}

// startHolder binds a datagram socket and serves its file descriptor, the way
// the dsd-socket-holder binary does.
func startHolder(t *testing.T) (handoffPath string, socket *DatagramSocket) {
	t.Helper()

	socket, err := BindDatagramSocket(socketPath(t, "dsd.socket"))
	require.NoError(t, err)

	handoffPath = socketPath(t, "handoff.sock")
	server, err := NewServer(handoffPath, 0700, socket)
	require.NoError(t, err)
	go func() { _ = server.Serve() }()

	t.Cleanup(func() {
		assert.NoError(t, server.Close())
		assert.NoError(t, socket.Close())
	})

	return handoffPath, socket
}

func TestReceivePacketConn(t *testing.T) {
	handoffPath, socket := startHolder(t)

	conn, err := ReceivePacketConn(handoffPath)
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, socket.Path(), conn.LocalAddr().String())
	assert.Equal(t, datagramTransport, conn.LocalAddr().Network())

	client, err := net.Dial(datagramTransport, socket.Path())
	require.NoError(t, err)
	defer client.Close()
	_, err = client.Write([]byte("daemon:666|g"))
	require.NoError(t, err)

	buf := make([]byte, 64)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	n, _, err := conn.ReadFrom(buf)
	require.NoError(t, err)
	assert.Equal(t, "daemon:666|g", string(buf[:n]))
}

// TestReceivePacketConnNoFDLeak makes sure neither side accumulates file
// descriptors when the Agent restarts and adopts the socket again.
func TestReceivePacketConnNoFDLeak(t *testing.T) {
	handoffPath, _ := startHolder(t)

	// The first handoff allocates the runtime poller descriptors.
	conn, err := ReceivePacketConn(handoffPath)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	before := openFileDescriptors(t)
	for i := 0; i < 20; i++ {
		conn, err := ReceivePacketConn(handoffPath)
		require.NoError(t, err)
		require.NoError(t, conn.Close())
	}
	assert.Equal(t, before, openFileDescriptors(t))
}

// TestNewServerHandoffSocketNeverWorldAccessible asserts the handoff socket is
// never reachable by another user, not even between being bound and being
// chmod-ed: it hands out a descriptor that reads every DogStatsD datagram.
func TestNewServerHandoffSocketNeverWorldAccessible(t *testing.T) {
	// A permissive umask is what makes the window observable.
	defer unix.Umask(unix.Umask(0))

	socket, err := BindDatagramSocket(socketPath(t, "dsd.socket"))
	require.NoError(t, err)
	defer socket.Close()

	handoffPath := socketPath(t, "handoff.sock")

	// Watch the path until it shows up and record the first mode it ever had.
	modes := make(chan os.FileMode, 1)
	stop := make(chan struct{})
	var watcher sync.WaitGroup
	watcher.Add(1)
	go func() {
		defer watcher.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if fi, err := os.Lstat(handoffPath); err == nil {
				select {
				case modes <- fi.Mode().Perm():
				default:
				}
				return
			}
		}
	}()

	server, err := NewServer(handoffPath, 0700, socket)
	require.NoError(t, err)
	defer server.Close()
	go func() { _ = server.Serve() }()
	close(stop)
	watcher.Wait()

	fi, err := os.Lstat(handoffPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), fi.Mode().Perm())

	select {
	case mode := <-modes:
		assert.Zero(t, mode&0o077, "the handoff socket was reachable by other users with mode %v", mode)
	default:
		// The watcher never caught the path, which is fine: it can only ever
		// have appeared with its final mode.
	}

	// The rename must not have broken the listener.
	assert.Equal(t, handoffPath, server.Addr().String())
	conn, err := ReceivePacketConn(handoffPath)
	require.NoError(t, err)
	assert.NoError(t, conn.Close())
}

// TestServerCloseRemovesHandoffSocket checks Close still cleans the socket file
// up now that package net no longer unlinks it.
func TestServerCloseRemovesHandoffSocket(t *testing.T) {
	socket, err := BindDatagramSocket(socketPath(t, "dsd.socket"))
	require.NoError(t, err)
	defer socket.Close()

	handoffPath := socketPath(t, "handoff.sock")
	server, err := NewServer(handoffPath, 0700, socket)
	require.NoError(t, err)
	require.NoError(t, server.Close())

	_, err = os.Stat(handoffPath)
	assert.True(t, os.IsNotExist(err), "the handoff socket was left behind: %v", err)

	// No temporary socket left over either.
	entries, err := os.ReadDir(filepath.Dir(handoffPath))
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestBindDatagramSocketRefusesLiveSocket asserts a second holder, or a holder
// restarting while the Agent still holds the adopted descriptor, does not
// silently replace the socket inode: doing so leaves the Agent reading an
// unlinked socket that no client can reach any more.
func TestBindDatagramSocketRefusesLiveSocket(t *testing.T) {
	path := socketPath(t, "dsd.socket")

	first, err := BindDatagramSocket(path)
	require.NoError(t, err)
	defer first.Close()

	second, err := BindDatagramSocket(path)
	require.Error(t, err)
	assert.Nil(t, second)
	assert.Contains(t, err.Error(), "still bound")

	// The first socket still receives, and it is still the inode clients reach.
	client, err := net.Dial(datagramTransport, path)
	require.NoError(t, err)
	defer client.Close()
	_, err = client.Write([]byte("daemon:666|g"))
	require.NoError(t, err)
}

// TestBindDatagramSocketReplacesStaleSocket is the counterpart: once nobody is
// bound any more the socket file is stale and must be replaced.
func TestBindDatagramSocketReplacesStaleSocket(t *testing.T) {
	path := socketPath(t, "dsd.socket")

	first, err := BindDatagramSocket(path)
	require.NoError(t, err)
	require.NoError(t, first.Close())

	second, err := BindDatagramSocket(path)
	require.NoError(t, err)
	defer second.Close()

	client, err := net.Dial(datagramTransport, path)
	require.NoError(t, err)
	defer client.Close()
	_, err = client.Write([]byte("daemon:666|g"))
	require.NoError(t, err)
}

// TestReceivePacketConnWaitsForHolder asserts the Agent does not give up when
// the holder is not listening yet: the two processes start independently, and
// failing here would leave the Agent with no UDS listener at all.
func TestReceivePacketConnWaitsForHolder(t *testing.T) {
	socket, err := BindDatagramSocket(socketPath(t, "dsd.socket"))
	require.NoError(t, err)
	defer socket.Close()
	handoffPath := socketPath(t, "handoff.sock")

	var holder sync.WaitGroup
	holder.Add(1)
	go func() {
		defer holder.Done()
		time.Sleep(300 * time.Millisecond)
		server, err := NewServer(handoffPath, 0700, socket)
		if err != nil {
			t.Errorf("NewServer: %v", err)
			return
		}
		go func() { _ = server.Serve() }()
		t.Cleanup(func() { server.Close() })
	}()

	conn, err := ReceivePacketConn(handoffPath)
	holder.Wait()
	require.NoError(t, err)
	assert.NoError(t, conn.Close())
}

// TestReceiveFileNoHolder makes sure a missing holder is reported rather than
// hanging forever.
func TestReceiveFileNoHolder(t *testing.T) {
	// Shorten the wait: the point is that it is bounded and returns an error.
	defer func(d time.Duration) { waitTimeout = d }(waitTimeout)
	waitTimeout = 500 * time.Millisecond

	start := time.Now()
	_, err := ReceivePacketConn(socketPath(t, "nothing.sock"))
	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)

	// The Agent falls back to binding the socket itself on this error, so it has
	// to be distinguishable from every other handoff failure.
	assert.ErrorIs(t, err, ErrHolderUnavailable)
}

// TestReceiveFileHolderPresentButFailingIsNotUnavailable pins the distinction
// the fallback depends on: when a holder answers but the handoff fails, it still
// owns the DogStatsD socket. Reporting ErrHolderUnavailable there would make the
// Agent rebind the socket, orphaning the inode the holder's clients are
// connected to and losing their datagrams silently.
func TestReceiveFileHolderPresentButFailingIsNotUnavailable(t *testing.T) {
	// A holder that accepts the connection and then hangs up without sending a
	// descriptor.
	handoffPath := serveOnce(t, func(conn *net.UnixConn) { conn.Close() })

	_, err := ReceivePacketConn(handoffPath)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrHolderUnavailable)
}

// serveOnce accepts a single connection on a handoff socket and lets fn send
// whatever it wants on it. It returns once the listener is up, so that a
// descriptor count taken afterwards is stable.
func serveOnce(t *testing.T, fn func(*net.UnixConn)) string {
	t.Helper()
	path := socketPath(t, "handoff.sock")
	addr, err := net.ResolveUnixAddr(handoffTransport, path)
	require.NoError(t, err)
	ln, err := net.ListenUnix(handoffTransport, addr)
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, err := ln.AcceptUnix()
		if err != nil {
			return
		}
		defer conn.Close()
		fn(conn)
	}()
	return path
}

// devNulls opens n descriptors to pass around. They are opened by the caller,
// before any descriptor count is taken, so that only the receiving side's
// descriptors are measured.
func devNulls(t *testing.T, n int) []int {
	t.Helper()
	fds := make([]int, 0, n)
	for i := 0; i < n; i++ {
		f, err := os.Open(os.DevNull)
		require.NoError(t, err)
		t.Cleanup(func() { f.Close() })
		fds = append(fds, int(f.Fd()))
	}
	return fds
}

// TestReceiveFileClosesExtraFDs asserts every descriptor beyond the first is
// closed instead of being leaked for the lifetime of the Agent. A holder that
// sends more than one descriptor used to leak all of them, because the
// ancillary buffer only had room for one and the resulting truncated control
// message could no longer be parsed.
func TestReceiveFileClosesExtraFDs(t *testing.T) {
	for _, count := range []int{2, 3, maxHandoffFDs} {
		t.Run(fmt.Sprintf("%d-descriptors", count), func(t *testing.T) {
			fds := devNulls(t, count)
			handoffPath := serveOnce(t, func(conn *net.UnixConn) {
				_, _, _ = conn.WriteMsgUnix([]byte(handoffMessage), unix.UnixRights(fds...), nil)
			})

			before := openFileDescriptors(t)
			f, err := receiveFileWithin(handoffPath, waitTimeout)
			require.NoError(t, err)
			require.NoError(t, f.Close())
			assert.Equal(t, before, openFileDescriptors(t))
		})
	}
}

// TestReceiveFileRejectsTruncatedControl asserts a peer sending more
// descriptors than the ancillary buffer holds is refused rather than silently
// accepted.
func TestReceiveFileRejectsTruncatedControl(t *testing.T) {
	fds := devNulls(t, maxHandoffFDs+1)
	handoffPath := serveOnce(t, func(conn *net.UnixConn) {
		_, _, _ = conn.WriteMsgUnix([]byte(handoffMessage), unix.UnixRights(fds...), nil)
	})

	_, err := receiveFileWithin(handoffPath, waitTimeout)
	require.Error(t, err)
}

// TestReceiveFileNoFD covers a peer that writes on the handoff socket without
// sending any descriptor.
func TestReceiveFileNoFD(t *testing.T) {
	handoffPath := serveOnce(t, func(conn *net.UnixConn) {
		_, _ = conn.Write([]byte(handoffMessage))
	})

	_, err := receiveFileWithin(handoffPath, waitTimeout)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no file descriptor received")
}

// TestServeSurvivesClientHangingUp asserts one client going away does not stop
// the holder from serving the next one.
func TestServeSurvivesClientHangingUp(t *testing.T) {
	handoffPath, _ := startHolder(t)

	for i := 0; i < 5; i++ {
		conn, err := net.Dial(handoffTransport, handoffPath)
		require.NoError(t, err)
		require.NoError(t, conn.Close())
	}

	conn, err := ReceivePacketConn(handoffPath)
	require.NoError(t, err)
	assert.NoError(t, conn.Close())
}

// TestConcurrentReceive exercises several Agents adopting the socket at once,
// which is what the race detector is interested in.
func TestConcurrentReceive(t *testing.T) {
	handoffPath, _ := startHolder(t)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := ReceivePacketConn(handoffPath)
			if !assert.NoError(t, err) {
				return
			}
			assert.NoError(t, conn.Close())
		}()
	}
	wg.Wait()
}

func TestIsTemporaryAcceptError(t *testing.T) {
	assert.True(t, isTemporaryAcceptError(unix.EMFILE))
	assert.True(t, isTemporaryAcceptError(&net.OpError{Op: "accept", Err: unix.ECONNABORTED}))
	assert.False(t, isTemporaryAcceptError(net.ErrClosed))
	assert.False(t, isTemporaryAcceptError(unix.EINVAL))
}

// TestParseMode asserts a mode written without a leading zero is still read as
// octal. Parsing "770" as decimal yields mode 0402, which is world writable and
// therefore lets any local user be handed the DogStatsD file descriptor.
func TestParseMode(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want os.FileMode
	}{
		{"0700", 0700},
		{"700", 0700},
		{"0770", 0770},
		{"770", 0770},
		{"777", 0777},
	} {
		mode, err := ParseMode(tc.in)
		require.NoError(t, err, tc.in)
		assert.Equal(t, tc.want, mode, "ParseMode(%q)", tc.in)
	}

	for _, in := range []string{"", "0800", "abc", "1700", "-1", "07000"} {
		_, err := ParseMode(in)
		assert.Error(t, err, "ParseMode(%q) should fail", in)
	}
}

func TestRemoveStaleSocketRefusesRegularFile(t *testing.T) {
	path := socketPath(t, "notasocket")
	require.NoError(t, os.WriteFile(path, []byte("keep me"), 0600))

	_, err := BindDatagramSocket(path)
	require.Error(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "keep me", string(content))
}
