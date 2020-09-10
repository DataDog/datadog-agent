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
	"syscall"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"github.com/pkg/errors"
)

/*
#include "c/offset-guess.h"
*/
import "C"

type tracerStatus C.tracer_status_t

const (
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
	guessRTT               = 6
	guessDaddrIPv6         = 7
	// Following values are associated with an UDP connection, used for guessing offsets
	// in the flowi4 data structure
	guessSaddrFl4 = 8
	guessDaddrFl4 = 9
	guessSportFl4 = 10
	guessDportFl4 = 11
)

const (
	notApplicable = 99999 // An arbitrary large number to indicate that the value should be ignored
)

// These constants should be in sync with the equivalent definitions in the ebpf program.
const (
	disabled C.__u8 = 0
	enabled         = 1
)

var whatString = map[C.__u64]string{
	guessSaddr:     "source address",
	guessDaddr:     "destination address",
	guessFamily:    "family",
	guessSport:     "source port",
	guessDport:     "destination port",
	guessNetns:     "network namespace",
	guessRTT:       "Round Trip Time",
	guessDaddrIPv6: "destination address IPv6",

	// Guess offsets in struct flowi4
	guessSaddrFl4: "source address flowi4",
	guessDaddrFl4: "destination address flowi4",
	guessSportFl4: "source port flowi4",
	guessDportFl4: "destination port flowi4",
}

const (
	tcpInfoKProbeNotCalled C.__u64 = 0
	tcpInfoKProbeCalled            = 1
)

var tcpKprobeCalledString = map[C.__u64]string{
	tcpInfoKProbeNotCalled: "tcp_get_info kprobe not executed",
	tcpInfoKProbeCalled:    "tcp_get_info kprobe executed",
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
	rtt       uint32
	rttVar    uint32
	daddrIPv6 [4]uint32

	// Used for guessing offsets in struct flowi4
	saddrFl4 uint32
	daddrFl4 uint32
	sportFl4 uint16
	dportFl4 uint16
}

func extractIPsAndPorts(conn net.Conn) (
	saddr, daddr uint32,
	sport, dport uint16,
	err error,
) {
	saddrStr, sportStr, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return
	}
	saddr = binary.LittleEndian.Uint32(net.ParseIP(saddrStr).To4())
	sportn, err := strconv.Atoi(sportStr)
	if err != nil {
		return
	}
	sport = uint16(sportn)

	daddrStr, dportStr, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return
	}
	daddr = binary.LittleEndian.Uint32(net.ParseIP(daddrStr).To4())
	dportn, err := strconv.Atoi(dportStr)
	if err != nil {
		return
	}

	dport = uint16(dportn)
	return
}

func expectedValues(conn net.Conn) (*fieldValues, error) {
	netns, err := ownNetNS()
	if err != nil {
		return nil, err
	}

	tcpInfo, err := tcpGetInfo(conn)
	if err != nil {
		return nil, err
	}

	saddr, daddr, sport, dport, err := extractIPsAndPorts(conn)
	if err != nil {
		return nil, err
	}

	return &fieldValues{
		saddr:  saddr,
		daddr:  daddr,
		sport:  sport,
		dport:  dport,
		netns:  uint32(netns),
		family: syscall.AF_INET,
		rtt:    tcpInfo.Rtt,
		rttVar: tcpInfo.Rttvar,
	}, nil
}

func waitUntilStable(conn net.Conn, window time.Duration, attempts int) (*fieldValues, error) {
	var (
		current *fieldValues
		prev    *fieldValues
		err     error
	)
	for i := 0; i <= attempts; i++ {
		current, err = expectedValues(conn)
		if err != nil {
			return nil, err
		}

		if prev != nil && *prev == *current {
			return current, nil
		}

		prev = current
		time.Sleep(window)
	}

	return nil, errors.New("unstable TCP socket params")
}

func offsetGuessProbes(c *Config) map[bytecode.ProbeName]struct{} {
	probes := map[bytecode.ProbeName]struct{}{
		bytecode.TCPGetInfo: {},
		bytecode.IPMakeSkb:  {},
	}

	if c.CollectIPv6Conns {
		probes[bytecode.TCPv6Connect] = struct{}{}
		probes[bytecode.TCPv6ConnectReturn] = struct{}{}
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
func checkAndUpdateCurrentOffset(mp *ebpf.Map, status *tracerStatus, expected *fieldValues, maxRetries *int, threshold uint64) error {
	// get the updated map value so we can check if the current offset is
	// the right one
	if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(status)); err != nil {
		return fmt.Errorf("error reading tracer_status: %v", err)
	}

	if status.state != stateChecked {
		if *maxRetries == 0 {
			return fmt.Errorf("invalid guessing state while guessing %v, got %v expected %v. %v",
				whatString[status.what], stateString[status.state], stateString[stateChecked], tcpKprobeCalledString[status.tcp_info_kprobe_status])
		}
		*maxRetries--
		time.Sleep(10 * time.Millisecond)
		return nil
	}
	switch status.what {
	case guessSaddr:
		if status.saddr == C.__u32(expected.saddr) {
			logAndAdvance(status, status.offset_saddr, guessDaddr)
			break
		}
		status.offset_saddr++
		status.saddr = C.__u32(expected.saddr)
	case guessDaddr:
		if status.daddr == C.__u32(expected.daddr) {
			logAndAdvance(status, status.offset_daddr, guessFamily)
			break
		}
		status.offset_daddr++
		status.daddr = C.__u32(expected.daddr)
	case guessFamily:
		if status.family == C.__u16(expected.family) {
			logAndAdvance(status, status.offset_family, guessSport)
			// we know the sport ((struct inet_sock)->inet_sport) is
			// after the family field, so we start from there
			status.offset_sport = status.offset_family
			break
		}
		status.offset_family++
	case guessSport:
		if status.sport == C.__u16(htons(expected.sport)) {
			logAndAdvance(status, status.offset_sport, guessDport)
			break
		}
		status.offset_sport++
	case guessDport:
		if status.dport == C.__u16(htons(expected.dport)) {
			logAndAdvance(status, status.offset_dport, guessSaddrFl4)
			break
		}
		status.offset_dport++
	case guessSaddrFl4:
		if status.saddr_fl4 == C.__u32(expected.saddrFl4) {
			logAndAdvance(status, status.offset_saddr_fl4, guessDaddrFl4)
			break
		}
		status.offset_saddr_fl4++
		if uint64(status.offset_saddr_fl4) == threshold {
			// Let's skip all other flowi4 fields
			logAndAdvance(status, notApplicable, guessNetns)
			status.fl4_offsets = disabled
			break
		}
	case guessDaddrFl4:
		if status.daddr_fl4 == C.__u32(expected.daddrFl4) {
			logAndAdvance(status, status.offset_daddr_fl4, guessSportFl4)
			break
		}
		status.offset_daddr_fl4++
		if uint64(status.offset_daddr_fl4) == threshold {
			logAndAdvance(status, notApplicable, guessNetns)
			status.fl4_offsets = disabled
			break
		}
	case guessSportFl4:
		if status.sport_fl4 == C.__u16(htons(expected.sportFl4)) {
			logAndAdvance(status, status.offset_sport_fl4, guessDportFl4)
			break
		}
		status.offset_sport_fl4++
		if uint64(status.offset_sport_fl4) == threshold {
			logAndAdvance(status, notApplicable, guessNetns)
			status.fl4_offsets = disabled
			break
		}
	case guessDportFl4:
		if status.dport_fl4 == C.__u16(htons(expected.dportFl4)) {
			logAndAdvance(status, status.offset_dport_fl4, guessNetns)
			status.fl4_offsets = enabled
			break
		}
		status.offset_dport_fl4++
		if uint64(status.offset_dport_fl4) == threshold {
			logAndAdvance(status, notApplicable, guessNetns)
			status.fl4_offsets = disabled
			break
		}
	case guessNetns:
		if status.netns == C.__u32(expected.netns) {
			logAndAdvance(status, status.offset_netns, guessRTT)
			break
		}
		status.offset_ino++
		// go to the next offset_netns if we get an error
		if status.err != 0 || uint64(status.offset_ino) >= threshold {
			status.offset_ino = 0
			status.offset_netns++
		}
	case guessRTT:
		// For more information on the bit shift operations see:
		// https://elixir.bootlin.com/linux/v4.6/source/net/ipv4/tcp.c#L2686
		if status.rtt>>3 == C.__u32(expected.rtt) && status.rtt_var>>2 == C.__u32(expected.rttVar) {
			logAndAdvance(status, status.offset_rtt, guessDaddrIPv6)
			break
		}
		// We know that these two fields are always next to each other, 4 bytes apart:
		// https://elixir.bootlin.com/linux/v4.6/source/include/linux/tcp.h#L232
		// rtt -> srtt_us
		// rtt_var -> mdev_us
		status.offset_rtt++
		status.offset_rtt_var = status.offset_rtt + 4

	case guessDaddrIPv6:
		if compareIPv6(status.daddr_ipv6, expected.daddrIPv6) {
			logAndAdvance(status, status.offset_rtt, notApplicable)
			// at this point, we've guessed all the offsets we need,
			// set the status to "stateReady"
			return setReadyState(mp, status)
		}
		status.offset_daddr_ipv6++
	default:
		return fmt.Errorf("unexpected field to guess: %v", whatString[status.what])
	}

	// This assumes `guessDaddrIPv6` is the last stage of the process.
	if status.what == guessDaddrIPv6 && status.ipv6_enabled == disabled {
		return setReadyState(mp, status)
	}

	status.state = stateChecking
	// update the map with the new offset/field to check
	if err := mp.Put(unsafe.Pointer(&zero), unsafe.Pointer(status)); err != nil {
		return fmt.Errorf("error updating tracer_status: %v", err)
	}

	return nil
}

func setReadyState(mp *ebpf.Map, status *tracerStatus) error {
	status.state = stateReady
	if err := mp.Put(unsafe.Pointer(&zero), unsafe.Pointer(status)); err != nil {
		return fmt.Errorf("error updating tracer_status: %v", err)
	}
	return nil
}

// guessOffsets expects manager.Manager to contain a map named tracer_status and helps initialize the
// tracer by guessing the right struct sock kernel struct offsets. Results are
// returned as constants which are runtime-edited into the tracer eBPF code.
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
func guessOffsets(m *manager.Manager, cfg *Config) ([]manager.ConstantEditor, error) {
	mp, _, err := m.GetMap(string(bytecode.TracerStatusMap))
	if err != nil {
		return nil, fmt.Errorf("unable to find map %s: %s", string(bytecode.TracerStatusMap), err)
	}

	// When reading kernel structs at different offsets, don't go over the set threshold
	// Defaults to 400, with a max of 3000. This is an arbitrary choice to avoid infinite loops.
	threshold := cfg.OffsetGuessThreshold

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
		ipv6_enabled: enabled,
	}
	if !cfg.CollectIPv6Conns {
		status.ipv6_enabled = disabled
	}

	// if we already have the offsets, just return
	err = mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(status))
	if err == nil && status.state == stateReady {
		return getConstantEditors(status), nil
	}

	eventGenerator, err := newEventGenerator()
	if err != nil {
		return nil, err
	}
	defer eventGenerator.Close()

	// initialize map
	if err := mp.Put(unsafe.Pointer(&zero), unsafe.Pointer(status)); err != nil {
		return nil, fmt.Errorf("error initializing tracer_status map: %v", err)
	}

	// If the kretprobe for tcp_v4_connect() is configured with a too-low maxactive, some kretprobe might be missing.
	// In this case, we detect it and try again. See: https://github.com/weaveworks/tcptracer-bpf/issues/24
	maxRetries := 100

	// Retrieve expected values from local connection
	expected, err := waitUntilStable(eventGenerator.conn, 200*time.Millisecond, 5)
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving expected value")
	}

	err = eventGenerator.populateUDPExpectedValues(expected)
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving expected value")
	}

	log.Debugf("Checking for offsets with threshold of %d", threshold)
	for status.state != stateReady {
		if err := eventGenerator.Generate(status, expected); err != nil {
			return nil, err
		}

		if err := checkAndUpdateCurrentOffset(mp, status, expected, &maxRetries, threshold); err != nil {
			return nil, err
		}

		// Stop at a reasonable offset so we don't run forever.
		// Reading too far away in kernel memory is not a big deal:
		// probe_kernel_read() handles faults gracefully.
		if uint64(status.offset_saddr) >= threshold || uint64(status.offset_daddr) >= threshold ||
			status.offset_sport >= thresholdInetSock || uint64(status.offset_dport) >= threshold ||
			uint64(status.offset_netns) >= threshold || uint64(status.offset_family) >= threshold ||
			uint64(status.offset_daddr_ipv6) >= threshold || status.offset_rtt >= thresholdInetSock {
			return nil, fmt.Errorf("overflow while guessing %v, bailing out", whatString[status.what])
		}
	}

	return getConstantEditors(status), nil
}

func getConstantEditors(status *tracerStatus) []manager.ConstantEditor {
	return []manager.ConstantEditor{
		{Name: "offset_saddr", Value: uint64(status.offset_saddr)},
		{Name: "offset_daddr", Value: uint64(status.offset_daddr)},
		{Name: "offset_sport", Value: uint64(status.offset_sport)},
		{Name: "offset_dport", Value: uint64(status.offset_dport)},
		{Name: "offset_netns", Value: uint64(status.offset_netns)},
		{Name: "offset_ino", Value: uint64(status.offset_ino)},
		{Name: "offset_family", Value: uint64(status.offset_family)},
		{Name: "offset_rtt", Value: uint64(status.offset_rtt)},
		{Name: "offset_rtt_var", Value: uint64(status.offset_rtt_var)},
		{Name: "offset_daddr_ipv6", Value: uint64(status.offset_daddr_ipv6)},
		{Name: "ipv6_enabled", Value: uint64(status.ipv6_enabled)},
		{Name: "offset_saddr_fl4", Value: uint64(status.offset_saddr_fl4)},
		{Name: "offset_daddr_fl4", Value: uint64(status.offset_daddr_fl4)},
		{Name: "offset_sport_fl4", Value: uint64(status.offset_sport_fl4)},
		{Name: "offset_dport_fl4", Value: uint64(status.offset_dport_fl4)},
		{Name: "fl4_offsets", Value: uint64(status.fl4_offsets)},
	}
}

type eventGenerator struct {
	listener net.Listener
	conn     net.Conn
	udpConn  net.Conn
}

func newEventGenerator() (*eventGenerator, error) {
	// port 0 means we let the kernel choose a free port
	addr := fmt.Sprintf("%s:0", listenIP)
	l, err := net.Listen("tcp4", addr)
	if err != nil {
		return nil, err
	}

	go acceptHandler(l)

	// Establish connection that will be used in the offset guessing
	c, err := net.Dial(l.Addr().Network(), l.Addr().String())
	if err != nil {
		l.Close()
		return nil, err
	}

	udpConn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return nil, err
	}
	return &eventGenerator{listener: l, conn: c, udpConn: udpConn}, nil
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
	} else if status.what == guessSaddrFl4 || status.what == guessDaddrFl4 || status.what == guessSportFl4 || status.what == guessDportFl4 {
		payload := []byte("test")
		_, err := e.udpConn.Write(payload)

		return err
	}

	// This triggers the KProbe handler attached to `tcp_get_info`
	_, err := tcpGetInfo(e.conn)
	return err
}

func (e *eventGenerator) populateUDPExpectedValues(expected *fieldValues) error {
	saddr, daddr, sport, dport, err := extractIPsAndPorts(e.udpConn)
	if err != nil {
		return err
	}
	expected.saddrFl4 = saddr
	expected.sportFl4 = sport
	expected.daddrFl4 = daddr
	expected.dportFl4 = dport
	return nil
}

func (e *eventGenerator) Close() {
	if e.conn != nil {
		e.conn.Close()
	}

	e.listener.Close()

	if e.udpConn != nil {
		e.udpConn.Close()
	}
}

func acceptHandler(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}

		_, _ = io.Copy(ioutil.Discard, conn)
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
		file.Fd(),
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

func logAndAdvance(status *tracerStatus, offset C.__u64, next C.__u64) {
	guess := status.what
	if offset != notApplicable {
		log.Debugf("Successfully guessed %v with offset of %d bytes", whatString[guess], offset)
	} else {
		log.Debugf("Could not guess offset for %v", whatString[guess])
	}
	if next != notApplicable {
		log.Debugf("Started offset guessing for %v", whatString[next])
		status.what = next
	}
}
