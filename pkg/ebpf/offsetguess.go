// +build linux_bpf

package ebpf

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/iovisor/gobpf/elf"
	"github.com/pkg/errors"
)

/*
#include "c/tracer-ebpf.h"
*/
import "C"

type tracerStatus C.tracer_status_t

const (
	// When reading kernel structs at different offsets, don't go over that
	// limit. This is an arbitrary choice to avoid infinite loops.
	threshold = 400

	// The source port is much further away in the inet sock.
	thresholdInetSock = 2000

	procNameMaxSize = 15
)

// These constants should be in sync with the equivalent definitions in the ebpf program.
const (
	stateUninitialized C.__u64 = 0
	stateChecking              = 1 // status set by userspace, waiting for eBPF
	stateChecked               = 2 // status set by eBPF, waiting for userspace
	stateReady                 = 3 // fully initialized, all offset known
)

var stateString = map[C.__u64]string{
	stateUninitialized: "uninitialized",
	stateChecking:      "checking",
	stateChecked:       "checked",
	stateReady:         "ready",
}

// These constants should be in sync with the equivalent definitions in the ebpf program.
const (
	guessSaddr     C.__u64 = 0
	guessDaddr             = 1
	guessFamily            = 2
	guessSport             = 3
	guessDport             = 4
	guessNetns             = 5
	guessDaddrIPv6         = 6
)

// These constants should be in sync with the equivalent definitions in the ebpf program.
const (
	disableV6 C.__u8 = 0
	enableV6         = 1
)

var whatString = map[C.__u64]string{
	guessSaddr:     "source address",
	guessDaddr:     "destination address",
	guessFamily:    "family",
	guessSport:     "source port",
	guessDport:     "destination port",
	guessNetns:     "network namespace",
	guessDaddrIPv6: "destination address IPv6",
}

const listenIP = "127.0.0.2"

var zero uint64

type fieldValues struct {
	saddr     uint32
	daddr     uint32
	sport     uint16
	dport     uint16
	netns     uint32
	family    uint16
	daddrIPv6 [4]uint32
}

func offsetGuessProbes(c *Config) []KProbeName {
	probes := []KProbeName{TCPGetInfo}
	if c.CollectIPv6Conns {
		probes = append(probes, TCPv6Connect, TCPv6ConnectReturn)
	}
	return probes
}

func compareIPv6(a [4]C.__u32, b [4]uint32) bool {
	for i := 0; i < 4; i++ {
		if a[i] != C.__u32(b[i]) {
			return false
		}
	}
	return true
}

func ownNetNS() (uint64, error) {
	var s syscall.Stat_t
	if err := syscall.Stat("/proc/self/ns/net", &s); err != nil {
		return 0, err
	}
	return s.Ino, nil
}

func ipv6FromUint32Arr(ipv6Addr [4]uint32) net.IP {
	buf := make([]byte, 16)
	for i := 0; i < 16; i++ {
		buf[i] = *(*byte)(unsafe.Pointer((uintptr(unsafe.Pointer(&ipv6Addr[0])) + uintptr(i))))
	}
	return net.IP(buf)
}

func htons(a uint16) uint16 {
	var arr [2]byte
	binary.BigEndian.PutUint16(arr[:], a)
	return nativeEndian.Uint16(arr[:])
}

func generateRandomIPv6Address() (addr [4]uint32) {
	// multicast (ff00::/8) or link-local (fe80::/10) addresses don't work for
	// our purposes so let's choose a "random number" for the first 32 bits.
	//
	// chosen by fair dice roll.
	// guaranteed to be random.
	// https://xkcd.com/221/
	addr[0] = 0x87586031
	addr[1] = rand.Uint32()
	addr[2] = rand.Uint32()
	addr[3] = rand.Uint32()

	return
}

// checkAndUpdateCurrentOffset checks the value for the current offset stored
// in the eBPF map against the expected value, incrementing the offset if it
// doesn't match, or going to the next field to guess if it does
func checkAndUpdateCurrentOffset(module *elf.Module, mp *elf.Map, status *tracerStatus, expected *fieldValues, maxRetries *int) error {
	// get the updated map value so we can check if the current offset is
	// the right one
	if err := module.LookupElement(mp, unsafe.Pointer(&zero), unsafe.Pointer(status)); err != nil {
		return fmt.Errorf("error reading tracer_status: %v", err)
	}

	if status.state != stateChecked {
		if *maxRetries == 0 {
			return fmt.Errorf("invalid guessing state while guessing %v, got %v expected %v",
				whatString[status.what], stateString[status.state], stateString[stateChecked])
		} else {
			*maxRetries--
			time.Sleep(10 * time.Millisecond)
			return nil
		}
	}

	switch status.what {
	case guessSaddr:
		if status.saddr == C.__u32(expected.saddr) {
			status.what = guessDaddr
		} else {
			status.offset_saddr++
			status.saddr = C.__u32(expected.saddr)
		}
		status.state = stateChecking
	case guessDaddr:
		if status.daddr == C.__u32(expected.daddr) {
			status.what = guessFamily
		} else {
			status.offset_daddr++
			status.daddr = C.__u32(expected.daddr)
		}
		status.state = stateChecking
	case guessFamily:
		if status.family == C.__u16(expected.family) {
			status.what = guessSport
			// we know the sport ((struct inet_sock)->inet_sport) is
			// after the family field, so we start from there
			status.offset_sport = status.offset_family
		} else {
			status.offset_family++
		}
		status.state = stateChecking
	case guessSport:
		if status.sport == C.__u16(htons(expected.sport)) {
			status.what = guessDport
		} else {
			status.offset_sport++
		}
		status.state = stateChecking
	case guessDport:
		if status.dport == C.__u16(htons(expected.dport)) {
			status.what = guessNetns
		} else {
			status.offset_dport++
		}
		status.state = stateChecking
	case guessNetns:
		if status.netns == C.__u32(expected.netns) {
			status.what = guessDaddrIPv6
		} else {
			status.offset_ino++
			// go to the next offset_netns if we get an error
			if status.err != 0 || status.offset_ino >= threshold {
				status.offset_ino = 0
				status.offset_netns++
			}
		}
		status.state = stateChecking
	case guessDaddrIPv6:
		if compareIPv6(status.daddr_ipv6, expected.daddrIPv6) {
			// at this point, we've guessed all the offsets we need,
			// set the status to "stateReady"
			status.state = stateReady
		} else {
			status.offset_daddr_ipv6++
			status.state = stateChecking
		}
	default:
		return fmt.Errorf("unexpected field to guess: %v", whatString[status.what])
	}

	// update the map with the new offset/field to check
	if err := module.UpdateElement(mp, unsafe.Pointer(&zero), unsafe.Pointer(status), 0); err != nil {
		return fmt.Errorf("error updating tracer_status: %v", err)
	}

	return nil
}

func setReadyState(m *elf.Module, mp *elf.Map, status *tracerStatus) error {
	status.state = stateReady
	if err := m.UpdateElement(mp, unsafe.Pointer(&zero), unsafe.Pointer(status), 0); err != nil {
		return fmt.Errorf("error updating tracer_status: %v", err)
	}
	return nil
}

// guessOffsets expects elf.Module to hold a tracer-bpf object and initializes the
// tracer by guessing the right struct sock kernel struct offsets. Results are
// stored in the `tracer_status` map as used by the module.
//
// To guess the offsets, we create connections from localhost (127.0.0.1) to
// 127.0.0.2:$PORT, where we have a server listening. We store the current
// possible offset and expected value of each field in a eBPF map. In kernel-space
// we rely on two different kprobes: `tcp_get_info` and `tcp_connect_v6`. When they're
// are triggered, we store the value of
//     (struct sock *)skp + possible_offset
// in the eBPF map. Then, back in userspace (checkAndUpdateCurrentOffset()), we
// check that value against the expected value of the field, advancing the
// offset and repeating the process until we find the value we expect. Then, we
// guess the next field.
func guessOffsets(m *elf.Module, cfg *Config) error {
	currentNetns, err := ownNetNS()
	if err != nil {
		return fmt.Errorf("error getting current netns: %v", err)
	}

	mp := m.Map(string(tracerStatusMap))

	// pid & tid must not change during the guessing work: the communication
	// between ebpf and userspace relies on it
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	processName := filepath.Base(os.Args[0])
	if len(processName) > procNameMaxSize { // Truncate process name if needed
		processName = processName[:procNameMaxSize]
	}

	cProcName := [procNameMaxSize + 1]C.char{} // Last char has to be null character, so add one
	for i := range processName {
		cProcName[i] = C.char(processName[i])
	}

	status := &tracerStatus{
		state:        stateChecking,
		proc:         C.proc_t{comm: cProcName},
		ipv6_enabled: enableV6,
	}
	if !cfg.CollectIPv6Conns {
		status.ipv6_enabled = disableV6
	}

	// if we already have the offsets, just return
	err = m.LookupElement(mp, unsafe.Pointer(&zero), unsafe.Pointer(status))
	if err == nil && status.state == stateReady {
		return nil
	}

	eventGenerator, err := newEventGenerator()
	if err != nil {
		return err
	}
	defer eventGenerator.Close()

	// initialize map
	if err := m.UpdateElement(mp, unsafe.Pointer(&zero), unsafe.Pointer(status), 0); err != nil {
		return fmt.Errorf("error initializing tracer_status map: %v", err)
	}

	expected := &fieldValues{
		// 127.0.0.1
		saddr: 0x0100007F,
		// 127.0.0.2
		daddr: 0x0200007F,
		// will be set later
		sport:  0,
		dport:  eventGenerator.lport,
		netns:  uint32(currentNetns),
		family: syscall.AF_INET,
	}

	// If the kretprobe for tcp_v4_connect() is configured with a too-low maxactive, some kretprobe might be missing.
	// In this case, we detect it and try again. See: https://github.com/weaveworks/tcptracer-bpf/issues/24
	maxRetries := 100

	for status.state != stateReady {
		// If IPv6 is not enabled, then set state to ready as its the last field we guess
		if status.what == guessDaddrIPv6 && !cfg.CollectIPv6Conns {
			if err := setReadyState(m, mp, status); err != nil {
				return err
			}
			continue
		}

		if err := eventGenerator.Generate(status, expected); err != nil {
			return err
		}

		if err := checkAndUpdateCurrentOffset(m, mp, status, expected, &maxRetries); err != nil {
			return err
		}

		// Stop at a reasonable offset so we don't run forever.
		// Reading too far away in kernel memory is not a big deal:
		// probe_kernel_read() handles faults gracefully.
		if status.offset_saddr >= threshold || status.offset_daddr >= threshold ||
			status.offset_sport >= thresholdInetSock || status.offset_dport >= threshold ||
			status.offset_netns >= threshold || status.offset_family >= threshold ||
			status.offset_daddr_ipv6 >= threshold {
			return fmt.Errorf("overflow while guessing %v, bailing out", whatString[status.what])
		}
	}
	return nil
}

type eventGenerator struct {
	listener net.Listener
	lport    uint16
	conn     net.Conn
}

func newEventGenerator() (*eventGenerator, error) {
	// port 0 means we let the kernel choose a free port
	addr := fmt.Sprintf("%s:0", listenIP)
	l, err := net.Listen("tcp4", addr)
	if err != nil {
		return nil, err
	}

	lport, err := strconv.Atoi(strings.Split(l.Addr().String(), ":")[1])
	if err != nil {
		l.Close()
		return nil, err
	}

	go acceptHandler(l)
	e := &eventGenerator{listener: l, lport: uint16(lport)}
	return e, nil
}

// Generate an event for offset guessing
func (e *eventGenerator) Generate(status *tracerStatus, expected *fieldValues) error {
	// Are we guessing the IPv6 field?
	if status.what == guessDaddrIPv6 {
		expected.daddrIPv6 = generateRandomIPv6Address()

		// For ipv6, we don't need the source port because we already guessed it doing ipv4 connections so
		// we use a random destination address and try to connect to it.
		expected.daddrIPv6 = generateRandomIPv6Address()
		bindAddress := fmt.Sprintf("[%s]:9092", ipv6FromUint32Arr(expected.daddrIPv6))

		// Since we connect to a random IP, this will most likely fail. In the unlikely case where it connects
		// successfully, we close the connection to avoid a leak.
		if conn, err := net.DialTimeout("tcp6", bindAddress, 10*time.Millisecond); err == nil {
			conn.Close()
		}

		return nil
	}

	// Ensure v4 connection is up. The same connection is used for guessing all v4 offsets.
	if e.conn == nil {
		bindAddress := fmt.Sprintf("%s:%d", listenIP, expected.dport)
		conn, err := net.Dial("tcp4", bindAddress)
		if err != nil {
			return fmt.Errorf("error dialing %q: %v", bindAddress, err)
		}

		e.conn = conn

		// get the source port assigned by the kernel
		sport, err := strconv.Atoi(strings.Split(conn.LocalAddr().String(), ":")[1])
		if err != nil {
			return fmt.Errorf("error converting source port: %v", err)
		}

		expected.sport = uint16(sport)

		// Set SO_LINGER to 0 so the connection state after closing is CLOSE instead of TIME_WAIT.
		// In this way, they will disappear from the conntrack table after around 10 seconds instead of 2 mins
		tcpConn, ok := conn.(*net.TCPConn)
		if !ok {
			return fmt.Errorf("not a tcp connection unexpectedly")
		}
		tcpConn.SetLinger(0)
	}

	// This triggers the KProbe handler attached to `tcp_get_info`
	_, err := tcpGetInfo(e.conn)
	return err
}

func (e *eventGenerator) Close() {
	if e.conn != nil {
		e.conn.Close()
	}

	e.listener.Close()
}

func acceptHandler(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}

		io.Copy(ioutil.Discard, conn)
		conn.Close()
	}
}

// tcpGetInfo obtains information from a TCP socket via GETSOCKOPT(2) system call.
// The motivation for using this is twofold: 1) it is a way of triggering the kprobe
// responsible for the V4 offset guessing in kernel-space and 2) using it we can obtain
// in user-space TCP socket information such as RTT and use it for setting the expected
// values in the `fieldValues` struct.
func tcpGetInfo(conn net.Conn) (*syscall.TCPInfo, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil, errors.New("not a TCPConn")
	}

	file, err := tcpConn.File()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var tcpInfo syscall.TCPInfo
	size := uint32(unsafe.Sizeof(tcpInfo))

	_, _, errno := syscall.Syscall6(
		syscall.SYS_GETSOCKOPT,
		uintptr(file.Fd()),
		uintptr(syscall.SOL_TCP),
		uintptr(syscall.TCP_INFO),
		uintptr(unsafe.Pointer(&tcpInfo)),
		uintptr(unsafe.Pointer(&size)),
		0,
	)

	if errno != 0 {
		return nil, errors.Wrap(errno, "error calling syscall.SYS_GETSOCKOPT")
	}

	return &tcpInfo, nil
}
