// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build linux

package portlist

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

var errForcedSockDiag = errors.New("forced sock_diag failure")

// testSock is a raw TCP socket opened by a test.
type testSock struct {
	fd    int
	inode string // "socket:[N]", the key used by linuxImpl.known
	port  uint16
}

type sockOpts struct {
	v6only    bool
	reuseport bool
	noListen  bool
}

func openTCP(addr netip.Addr, port uint16, o sockOpts) (testSock, error) {
	family := unix.AF_INET
	if addr.Is6() {
		family = unix.AF_INET6
	}
	fd, err := unix.Socket(family, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return testSock{}, err
	}
	ok := false
	defer func() {
		if !ok {
			unix.Close(fd)
		}
	}()
	if family == unix.AF_INET6 {
		v := 0
		if o.v6only {
			v = 1
		}
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_V6ONLY, v); err != nil {
			return testSock{}, err
		}
	}
	if o.reuseport {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
			return testSock{}, err
		}
	}
	var sa unix.Sockaddr
	if family == unix.AF_INET {
		sa = &unix.SockaddrInet4{Port: int(port), Addr: addr.As4()}
	} else {
		sa = &unix.SockaddrInet6{Port: int(port), Addr: addr.As16()}
	}
	if err := unix.Bind(fd, sa); err != nil {
		return testSock{}, fmt.Errorf("bind %s:%d: %w", addr, port, err)
	}
	if !o.noListen {
		if err := unix.Listen(fd, 4096); err != nil {
			return testSock{}, err
		}
	}
	s, err := describe(fd)
	if err != nil {
		return testSock{}, err
	}
	ok = true
	return s, nil
}

func describe(fd int) (testSock, error) {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return testSock{}, err
	}
	sa, err := unix.Getsockname(fd)
	if err != nil {
		return testSock{}, err
	}
	var port int
	switch a := sa.(type) {
	case *unix.SockaddrInet4:
		port = a.Port
	case *unix.SockaddrInet6:
		port = a.Port
	}
	return testSock{fd: fd, inode: "socket:[" + strconv.FormatUint(st.Ino, 10) + "]", port: uint16(port)}, nil
}

// connectPair opens an established connection to l and returns both ends.
func connectPair(l testSock, addr netip.Addr) (client, server int, err error) {
	family := unix.AF_INET6
	var sa unix.Sockaddr = &unix.SockaddrInet6{Port: int(l.port), Addr: addr.As16()}
	if addr.Is4() {
		family = unix.AF_INET
		sa = &unix.SockaddrInet4{Port: int(l.port), Addr: addr.As4()}
	}
	client, err = unix.Socket(family, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return -1, -1, err
	}
	if err = unix.Connect(client, sa); err != nil {
		unix.Close(client)
		return -1, -1, err
	}
	server, _, err = unix.Accept4(l.fd, unix.SOCK_CLOEXEC)
	if err != nil {
		unix.Close(client)
		return -1, -1, err
	}
	return client, server, nil
}

// closeNoTimeWait closes fd with a zero linger so no TIME_WAIT entry is left.
func closeNoTimeWait(fd int) {
	_ = unix.SetsockoptLinger(fd, unix.SOL_SOCKET, unix.SO_LINGER, &unix.Linger{Onoff: 1, Linger: 0})
	unix.Close(fd)
}

func setLoopbackUp() error {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	ifr, err := unix.NewIfreq("lo")
	if err != nil {
		return err
	}
	ifr.SetUint16(unix.IFF_UP | unix.IFF_LOOPBACK | unix.IFF_RUNNING)
	return unix.IoctlIfreq(fd, unix.SIOCSIFFLAGS, ifr)
}

// inNewNetns runs fn on a thread in a fresh, anonymous network namespace with
// lo up, then moves the thread back. Sockets created by fn stay in the new
// namespace, which goes away once its last socket is closed. The thread must
// be restored rather than abandoned: if it is the thread-group leader,
// /proc/self/net follows its namespace.
func inNewNetns(fn func() error) error {
	errc := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		orig, err := unix.Open("/proc/thread-self/ns/net", unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			runtime.UnlockOSThread()
			errc <- err
			return
		}
		defer unix.Close(orig)
		if err := unix.Unshare(unix.CLONE_NEWNET); err != nil {
			runtime.UnlockOSThread()
			errc <- fmt.Errorf("unshare: %w", err)
			return
		}
		err = setLoopbackUp()
		if err == nil {
			err = fn()
		}
		if serr := unix.Setns(orig, unix.CLONE_NEWNET); serr != nil {
			// Leave the thread locked so it is discarded with the goroutine.
			errc <- fmt.Errorf("restoring netns: %w", serr)
			return
		}
		runtime.UnlockOSThread()
		errc <- err
	}()
	return <-errc
}

func requireRoot(tb testing.TB) {
	if os.Geteuid() != 0 {
		tb.Skip("requires root to create network namespaces")
	}
}

// pollKnown runs one AppendListeningPorts in TCPOnly mode with sock_diag
// either required or forced to fail, and returns the per-inode entries and
// the final list.
func pollKnown(t *testing.T, includeLocalhost, useSockDiag bool) (map[string]string, []string) {
	t.Helper()
	orig := dumpTCPListeners
	defer func() { dumpTCPListeners = orig }()
	var dumpErr error
	dumped := false
	if useSockDiag {
		dumpTCPListeners = func() ([]sockDiagListener, error) {
			ls, err := orig()
			dumped, dumpErr = true, err
			return ls, err
		}
	} else {
		dumpTCPListeners = func() ([]sockDiagListener, error) { return nil, errForcedSockDiag }
	}

	li := newLinuxImpl(includeLocalhost, true)
	ports, err := li.AppendListeningPorts(nil)
	if err != nil {
		t.Fatalf("AppendListeningPorts: %v", err)
	}
	if dumpErr != nil {
		t.Fatalf("sock_diag dump failed: %v", dumpErr)
	}
	if useSockDiag && !dumped {
		t.Fatal("sock_diag was not attempted (latched off?)")
	}
	known := make(map[string]string, len(li.known))
	for ino, pm := range li.known {
		known[ino] = fmtPort(pm.port)
	}
	list := make([]string, len(ports))
	for i, p := range ports {
		list[i] = fmtPort(p)
	}
	return known, list
}

func fmtPort(p Port) string {
	return fmt.Sprintf("%s %s %d pid=%d %s", p.Proto, p.IP, p.Port, p.Pid, p.Process)
}

func mapDiff(want, got map[string]string) string {
	var b strings.Builder
	for k, v := range want {
		if g, ok := got[k]; !ok {
			fmt.Fprintf(&b, "  only procfs: %s %s\n", k, v)
		} else if g != v {
			fmt.Fprintf(&b, "  differs: %s procfs=%q sock_diag=%q\n", k, v, g)
		}
	}
	for k, v := range got {
		if _, ok := want[k]; !ok {
			fmt.Fprintf(&b, "  only sock_diag: %s %s\n", k, v)
		}
	}
	return b.String()
}

type wantSock struct {
	s         testSock
	ip        netip.Addr // as reported, after unmapping
	localhost bool       // dropped when IncludeLocalhost is false
}

func TestSockDiagMatchesProcNet(t *testing.T) {
	var (
		any4   = netip.IPv4Unspecified()
		any6   = netip.IPv6Unspecified()
		lo4    = netip.MustParseAddr("127.0.0.1")
		lo4b   = netip.MustParseAddr("127.0.0.2")
		lo6    = netip.IPv6Loopback()
		mapped = netip.MustParseAddr("::ffff:127.0.0.1")
	)

	var want []wantSock
	var excluded []testSock
	closeLater := func(fd int) { t.Cleanup(func() { closeNoTimeWait(fd) }) }
	mustOpen := func(addr netip.Addr, port uint16, o sockOpts) testSock {
		t.Helper()
		s, err := openTCP(addr, port, o)
		if err != nil {
			t.Fatal(err)
		}
		closeLater(s.fd)
		return s
	}
	add := func(addr netip.Addr, port uint16, o sockOpts, ip netip.Addr, localhost bool) testSock {
		t.Helper()
		s := mustOpen(addr, port, o)
		want = append(want, wantSock{s: s, ip: ip, localhost: localhost})
		return s
	}

	wild4 := add(any4, 0, sockOpts{}, any4, false)
	add(lo4, 0, sockOpts{}, lo4, true)
	add(lo4b, 0, sockOpts{}, lo4b, false)
	// IPv6 cases need a usable ::1, which some containers lack.
	var shared6 testSock
	hasV6 := false
	if s, err := openTCP(lo6, 0, sockOpts{v6only: true}); err == nil {
		unix.Close(s.fd)
		hasV6 = true
	} else {
		t.Logf("IPv6 unavailable, skipping IPv6 cases: %v", err)
	}
	shared := add(any4, 0, sockOpts{}, any4, false)
	rp4 := add(lo4, 0, sockOpts{reuseport: true}, lo4, true)
	add(lo4, rp4.port, sockOpts{reuseport: true}, lo4, true)
	add(lo4, rp4.port, sockOpts{reuseport: true}, lo4, true)
	if hasV6 {
		add(any6, 0, sockOpts{}, any6, false) // dual-stack
		add(lo6, 0, sockOpts{v6only: true}, lo6, true)
		// v4-mapped shows up in tcp6 and is unmapped; the procfs localhost
		// filter doesn't match it, so it's kept either way.
		add(mapped, 0, sockOpts{}, lo4, false)
		// IPv4 and v6only IPv6 wildcards sharing one port.
		shared6 = add(any6, shared.port, sockOpts{v6only: true}, any6, false)
		// SO_REUSEPORT on the IPv6 wildcard.
		rp6 := add(any6, 0, sockOpts{v6only: true, reuseport: true}, any6, false)
		add(any6, rp6.port, sockOpts{v6only: true, reuseport: true}, any6, false)
	}

	// A non-loopback local address, when the host has one.
	if addrs, err := net.InterfaceAddrs(); err == nil {
		var got4, got6 bool
		for _, a := range addrs {
			pfx, err := netip.ParsePrefix(a.String())
			if err != nil || !pfx.Addr().IsGlobalUnicast() {
				continue
			}
			ip := pfx.Addr()
			if (ip.Is4() && got4) || (ip.Is6() && (got6 || ip.Zone() != "")) {
				continue
			}
			if s, err := openTCP(ip, 0, sockOpts{}); err == nil {
				closeLater(s.fd)
				want = append(want, wantSock{s: s, ip: ip})
				got4 = got4 || ip.Is4()
				got6 = got6 || ip.Is6()
			}
		}
	}

	// Established connections and a bound, non-listening socket must not appear.
	type dst struct {
		l    testSock
		addr netip.Addr
	}
	dsts := []dst{{wild4, lo4}}
	if hasV6 {
		dsts = append(dsts, dst{shared6, lo6})
	}
	for _, c := range dsts {
		client, server, err := connectPair(c.l, c.addr)
		if err != nil {
			t.Fatal(err)
		}
		closeLater(client)
		closeLater(server)
		for _, fd := range []int{client, server} {
			s, err := describe(fd)
			if err != nil {
				t.Fatal(err)
			}
			excluded = append(excluded, s)
		}
	}
	excluded = append(excluded, mustOpen(lo4, 0, sockOpts{noListen: true}))

	// Listeners in another network namespace must not appear.
	if os.Geteuid() == 0 {
		err := inNewNetns(func() error {
			addrs := []netip.Addr{any4, lo4}
			if hasV6 {
				addrs = append(addrs, any6)
			}
			for _, a := range addrs {
				s, err := openTCP(a, 0, sockOpts{})
				if err != nil {
					return err
				}
				closeLater(s.fd)
				excluded = append(excluded, s)
			}
			return nil
		})
		if errors.Is(err, unix.EPERM) {
			t.Logf("cannot create a network namespace, skipping those listeners: %v", err)
		} else if err != nil {
			t.Fatal(err)
		}
	} else {
		t.Log("not root: skipping the other-namespace listeners")
	}

	for _, includeLocalhost := range []bool{true, false} {
		t.Run(fmt.Sprintf("includeLocalhost=%v", includeLocalhost), func(t *testing.T) {
			// Other processes on the host may change listeners between the
			// two reads, so allow a few attempts to get a consistent pair.
			var sdKnown map[string]string
			var diff string
			for attempt := 0; attempt < 5; attempt++ {
				pfKnown, pfList := pollKnown(t, includeLocalhost, false)
				var sdList []string
				sdKnown, sdList = pollKnown(t, includeLocalhost, true)
				diff = mapDiff(pfKnown, sdKnown)
				if !slices.Equal(pfList, sdList) {
					diff += fmt.Sprintf("  lists differ:\n    procfs:    %q\n    sock_diag: %q\n", pfList, sdList)
				}
				if diff == "" {
					break
				}
			}
			if diff != "" {
				t.Fatalf("sock_diag and /proc/net disagree:\n%s", diff)
			}

			pid := os.Getpid()
			for _, w := range want {
				got, ok := sdKnown[w.s.inode]
				if !includeLocalhost && w.localhost {
					if ok {
						t.Errorf("%s (%s:%d) should be filtered as localhost, got %s", w.s.inode, w.ip, w.s.port, got)
					}
					continue
				}
				if !ok {
					t.Errorf("missing listener %s %s:%d", w.s.inode, w.ip, w.s.port)
					continue
				}
				if prefix := fmt.Sprintf("tcp %s %d pid=%d ", w.ip, w.s.port, pid); len(got) < len(prefix) || got[:len(prefix)] != prefix {
					t.Errorf("listener %s: got %q, want prefix %q", w.s.inode, got, prefix)
				}
			}
			for _, s := range excluded {
				if got, ok := sdKnown[s.inode]; ok {
					t.Errorf("non-listening or other-namespace socket %s reported: %s", s.inode, got)
				}
			}
			for k, v := range sdKnown {
				if v[:4] != "tcp " {
					t.Errorf("non-TCP entry %s: %s", k, v)
				}
			}
		})
	}
}

func TestTCPOnlyFallsBackToProcNet(t *testing.T) {
	l, err := openTCP(netip.MustParseAddr("127.0.0.1"), 0, sockOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer closeNoTimeWait(l.fd)
	u, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()

	orig := dumpTCPListeners
	defer func() { dumpTCPListeners = orig }()
	calls := 0
	dumpTCPListeners = func() ([]sockDiagListener, error) {
		calls++
		return nil, errForcedSockDiag
	}

	var diff string
	for attempt := 0; attempt < 5; attempt++ {
		got, _, err := (&Poller{IncludeLocalhost: true, TCPOnly: true}).Poll()
		if err != nil {
			t.Fatal(err)
		}
		all, _, err := (&Poller{IncludeLocalhost: true}).Poll()
		if err != nil {
			t.Fatal(err)
		}
		var want []Port
		for _, p := range all {
			if p.Proto == "tcp" {
				want = append(want, p)
			}
		}
		diff = ""
		if !slices.EqualFunc(want, got, func(a, b Port) bool { return a == b }) {
			diff = fmt.Sprintf("want %v\ngot  %v", want, got)
		}
		if diff == "" {
			if !slices.ContainsFunc(got, func(p Port) bool { return p.Port == l.port && p.Pid == os.Getpid() }) {
				t.Fatalf("fallback result is missing our listener on port %d: %v", l.port, got)
			}
			break
		}
	}
	if diff != "" {
		t.Fatalf("fallback differs from /proc/net TCP entries:\n%s", diff)
	}
	if calls == 0 {
		t.Fatal("sock_diag path was never attempted")
	}
}

// fakeNetlink feeds crafted datagrams to recvInetDiag through a socketpair,
// reporting them as sent by netlink port ID senderPid (0 is the kernel).
func fakeNetlink(t *testing.T, senderPid uint32, msgs ...[]byte) recvFunc {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unix.Close(fds[0]); unix.Close(fds[1]) })
	for _, m := range msgs {
		if _, err := unix.Write(fds[1], m); err != nil {
			t.Fatal(err)
		}
	}
	return func(p []byte) (int, int, unix.Sockaddr, error) {
		n, _, flags, _, err := unix.Recvmsg(fds[0], p, nil, unix.MSG_DONTWAIT)
		return n, flags, &unix.SockaddrNetlink{Family: unix.AF_NETLINK, Pid: senderPid}, err
	}
}

func nlmsg(typ uint16, flags uint16, seq uint32, payload []byte) []byte {
	n := nlmsgHdrLen + len(payload)
	b := make([]byte, (n+3)&^3)
	ne := binary.NativeEndian
	ne.PutUint32(b[0:4], uint32(n))
	ne.PutUint16(b[4:6], typ)
	ne.PutUint16(b[6:8], flags)
	ne.PutUint32(b[8:12], seq)
	copy(b[nlmsgHdrLen:], payload)
	return b
}

func diagMsg(family, state uint8, addr netip.Addr, port uint16, inode uint32) []byte {
	m := make([]byte, inetDiagMsgLen)
	m[0], m[1] = family, state
	binary.BigEndian.PutUint16(m[4:6], port)
	if family == unix.AF_INET {
		a := addr.As4()
		copy(m[8:12], a[:])
	} else {
		a := addr.As16()
		copy(m[8:24], a[:])
	}
	binary.NativeEndian.PutUint32(m[68:72], inode)
	return m
}

func errPayload(errno int32) []byte {
	b := make([]byte, 4)
	binary.NativeEndian.PutUint32(b, uint32(errno))
	return b
}

func TestRecvInetDiag(t *testing.T) {
	const seq = 7
	lo4 := netip.MustParseAddr("127.0.0.1")
	mapped := netip.MustParseAddr("::ffff:127.0.0.1")
	cat := func(bs ...[]byte) []byte { return slices.Concat(bs...) }

	t.Run("multipart", func(t *testing.T) {
		recv := fakeNetlink(t, 0,
			cat(nlmsg(unix.SOCK_DIAG_BY_FAMILY, unix.NLM_F_MULTI, seq, diagMsg(unix.AF_INET6, tcpStateListen, mapped, 80, 1)),
				nlmsg(unix.SOCK_DIAG_BY_FAMILY, unix.NLM_F_MULTI, seq, diagMsg(unix.AF_INET6, 1, mapped, 81, 2))),
			cat(nlmsg(unix.SOCK_DIAG_BY_FAMILY, unix.NLM_F_MULTI, seq, diagMsg(unix.AF_INET6, tcpStateListen, netip.IPv6Loopback(), 82, 3)),
				nlmsg(unix.NLMSG_DONE, unix.NLM_F_MULTI, seq, errPayload(0))),
		)
		got, err := recvInetDiag(recv, make([]byte, sockDiagRecvBuf), unix.AF_INET6, seq, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := []sockDiagListener{
			{ip: lo4, port: 80, inode: 1},
			{ip: netip.IPv6Loopback(), port: 82, inode: 3, loopback: true},
		}
		if !slices.Equal(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("not_from_kernel", func(t *testing.T) {
		recv := fakeNetlink(t, 4242, nlmsg(unix.NLMSG_DONE, unix.NLM_F_MULTI, seq, errPayload(0)))
		if _, err := recvInetDiag(recv, make([]byte, sockDiagRecvBuf), unix.AF_INET, seq, nil); !errors.Is(err, errNotFromKernel) {
			t.Fatalf("got %v, want errNotFromKernel", err)
		}
	})

	t.Run("non_netlink_sender", func(t *testing.T) {
		recv := func([]byte) (int, int, unix.Sockaddr, error) { return 0, 0, nil, nil }
		if _, err := recvInetDiag(recv, make([]byte, sockDiagRecvBuf), unix.AF_INET, seq, nil); !errors.Is(err, errNotFromKernel) {
			t.Fatalf("got %v, want errNotFromKernel", err)
		}
	})

	for _, tc := range []struct {
		name string
		msgs [][]byte
		buf  int
	}{
		{"nlmsg_error", [][]byte{nlmsg(unix.NLMSG_ERROR, 0, seq, errPayload(-int32(unix.EPERM)))}, sockDiagRecvBuf},
		{"done_with_error", [][]byte{nlmsg(unix.NLMSG_DONE, unix.NLM_F_MULTI, seq, errPayload(-int32(unix.EINVAL)))}, sockDiagRecvBuf},
		{"wrong_seq", [][]byte{nlmsg(unix.NLMSG_DONE, unix.NLM_F_MULTI, seq+1, errPayload(0))}, sockDiagRecvBuf},
		{"dump_interrupted", [][]byte{nlmsg(unix.NLMSG_DONE, unix.NLM_F_MULTI|unix.NLM_F_DUMP_INTR, seq, errPayload(0))}, sockDiagRecvBuf},
		{"unexpected_type", [][]byte{nlmsg(99, 0, seq, nil)}, sockDiagRecvBuf},
		{"short_msg", [][]byte{nlmsg(unix.SOCK_DIAG_BY_FAMILY, unix.NLM_F_MULTI, seq, make([]byte, 10))}, sockDiagRecvBuf},
		{"truncated", [][]byte{nlmsg(unix.SOCK_DIAG_BY_FAMILY, unix.NLM_F_MULTI, seq, diagMsg(unix.AF_INET, tcpStateListen, lo4, 80, 1))}, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recv := fakeNetlink(t, 0, tc.msgs...)
			if _, err := recvInetDiag(recv, make([]byte, tc.buf), unix.AF_INET, seq, nil); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// resetSockDiagLatch clears the process-wide latches for the test's duration.
func resetSockDiagLatch(t *testing.T) {
	sockDiagOff.Store(false)
	sockDiagNoV6.Store(false)
	t.Cleanup(func() {
		sockDiagOff.Store(false)
		sockDiagNoV6.Store(false)
	})
}

func TestSockDiagLatch(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		latch bool
	}{
		{"socket_EPROTONOSUPPORT", fmt.Errorf("netlink socket: %w", unix.EPROTONOSUPPORT), true},
		{"socket_EAFNOSUPPORT", fmt.Errorf("netlink socket: %w", unix.EAFNOSUPPORT), true},
		{"nlmsg_error_ENOENT", fmt.Errorf("sock_diag request: %w", unix.ENOENT), true},
		{"nlmsg_error_EINVAL", fmt.Errorf("sock_diag request: %w", unix.EINVAL), true},
		{"nlmsg_error_EPERM", fmt.Errorf("sock_diag request: %w", unix.EPERM), true},
		{"socket_EACCES", fmt.Errorf("netlink socket: %w", unix.EACCES), true},
		{"recv_EAGAIN", fmt.Errorf("receiving sock_diag reply: %w", unix.EAGAIN), false},
		{"recv_EINTR", fmt.Errorf("receiving sock_diag reply: %w", unix.EINTR), false},
		{"send_ENOBUFS", fmt.Errorf("sending sock_diag request: %w", unix.ENOBUFS), false},
		{"truncated", errSockDiagTruncated, false},
		{"dump_interrupted", errors.New("sock_diag dump interrupted"), false},
		{"wrong_seq", errors.New("unexpected netlink seq 2, want 1"), false},
		{"parse", errors.New("short inet_diag_msg (10 bytes)"), false},
		{"not_from_kernel", errNotFromKernel, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetSockDiagLatch(t)
			orig := dumpTCPListeners
			defer func() { dumpTCPListeners = orig }()
			calls := 0
			dumpTCPListeners = func() ([]sockDiagListener, error) {
				calls++
				return nil, tc.err
			}
			li := newLinuxImpl(true, true)
			const polls = 3
			for i := 0; i < polls; i++ {
				if _, err := li.AppendListeningPorts(nil); err != nil {
					t.Fatal(err)
				}
			}
			want := polls
			if tc.latch {
				want = 1
			}
			if calls != want {
				t.Fatalf("sock_diag attempted %d times over %d polls, want %d", calls, polls, want)
			}
			if sockDiagOff.Load() != tc.latch {
				t.Fatalf("latch = %v, want %v", sockDiagOff.Load(), tc.latch)
			}
		})
	}
}

func TestSockDiagLatchConcurrent(t *testing.T) {
	resetSockDiagLatch(t)
	orig := dumpTCPListeners
	defer func() { dumpTCPListeners = orig }()
	dumpTCPListeners = func() ([]sockDiagListener, error) {
		return nil, fmt.Errorf("sock_diag request: %w", unix.ENOENT)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			li := newLinuxImpl(true, true)
			for j := 0; j < 5; j++ {
				if err := li.appendSockDiagListeners(); err == nil {
					t.Error("expected an error")
				}
			}
		}()
	}
	wg.Wait()
	if !sockDiagOff.Load() {
		t.Fatal("latch not set")
	}
}

// BenchmarkTCPListeners compares one TCPOnly poll via /proc/net and via
// sock_diag while many established TCP sockets exist in another network
// namespace or in this one. Set PORTLIST_BENCH_CONNS to change the number of
// connections (each is two sockets).
func BenchmarkTCPListeners(b *testing.B) {
	requireRoot(b)
	conns := 50000
	if v, err := strconv.Atoi(os.Getenv("PORTLIST_BENCH_CONNS")); err == nil {
		conns = v
	}

	for _, load := range []string{"none", "otherns", "samens"} {
		b.Run("load="+load, func(b *testing.B) {
			var fds []int
			defer func() {
				for _, fd := range fds {
					closeNoTimeWait(fd)
				}
			}()
			if load != "none" {
				mk := func() error {
					var err error
					fds, err = openConns(conns)
					return err
				}
				var err error
				if load == "otherns" {
					err = inNewNetns(mk)
				} else {
					err = mk()
				}
				if err != nil {
					b.Fatal(err)
				}
			}

			for _, mode := range []string{"procfs-all", "procfs-tcp", "sockdiag"} {
				b.Run("mode="+mode, func(b *testing.B) {
					orig := dumpTCPListeners
					defer func() { dumpTCPListeners = orig }()
					if mode == "procfs-tcp" {
						dumpTCPListeners = func() ([]sockDiagListener, error) { return nil, errForcedSockDiag }
					}
					li := newLinuxImpl(true, mode != "procfs-all")
					// Warm the inode cache so the loop measures enumeration only.
					buf, err := li.AppendListeningPorts(nil)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if buf, err = li.AppendListeningPorts(buf[:0]); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// openConns opens n loopback connections spread over several listeners in the
// calling thread's network namespace. It returns every fd, listeners included.
func openConns(n int) (fds []int, err error) {
	defer func() {
		if err != nil {
			for _, fd := range fds {
				closeNoTimeWait(fd)
			}
			fds = nil
		}
	}()
	lo4 := netip.MustParseAddr("127.0.0.1")
	// Several destinations keep us well inside the ephemeral port range.
	const listeners = 8
	ls := make([]testSock, listeners)
	for i := range ls {
		if ls[i], err = openTCP(lo4, 0, sockOpts{}); err != nil {
			return fds, err
		}
		fds = append(fds, ls[i].fd)
	}
	for i := 0; i < n; i++ {
		c, s, err := connectPair(ls[i%listeners], lo4)
		if err != nil {
			return fds, fmt.Errorf("connection %d: %w", i, err)
		}
		fds = append(fds, c, s)
	}
	return fds, nil
}
