// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && ebpf_bindata && pcap && cgo

// Package tests holds tests related files
package tests

import (
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"math"
	"net"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/safchain/baloum/pkg/baloum"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes/rawpacket"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// struct __sk_buff offsets
const (
	skbLenOffset     = 0
	skbIfindexOffset = 40
	skbDataEndOffset = 80
	skbSkOffset      = 168
	skbSize          = 192
)

const (
	tcTestPid     = 4242
	tcTestCookie  = 0x42
	tcTestNetns   = 0xf0000042
	tcTestIfindex = 2
	tcTestNow     = uint64(1) << 40
	// fake struct bpf_sock pointer, only handed to emulated helpers
	tcTestSock = 0x5000
)

var (
	tcTestHostIP = net.IP{10, 0, 0, 2}
	tcTestPeerIP = net.IP{10, 0, 0, 3}
	tcTestIMDSIP = net.IP{169, 254, 169, 254}

	tcTestRawPacketFilters = []rawpacket.Filter{
		{RuleID: "tcp-syn", BPFFilter: "tcp dst port 5555 and tcp[tcpflags] == tcp-syn"},
		{RuleID: "dns", BPFFilter: "udp port 53"},
		{RuleID: "arp", BPFFilter: "arp"},
	}
	tcTestDropActions = []rawpacket.Filter{
		{RuleID: "drop", BPFFilter: "tcp dst port 5555", Policy: rawpacket.PolicyDrop, Pid: tcTestPid},
	}
)

type tcTestEnv struct {
	name string
	// raw packet filter, pid helper, sk lookup, flow monitor and span tracking
	features bool
	// pid from the socket cookie, else from the current task on egress when features is set
	cookiePid bool
}

var tcTestEnvs = []tcTestEnv{
	{name: "cookie-pid", features: true, cookiePid: true},
	{name: "task-pid", features: true},
	{name: "no-pid"},
}

func (env tcTestEnv) constants() map[string]uint64 {
	var enabled uint64
	if env.features {
		enabled = 1
	}
	return map[string]uint64{
		"netns":                                 tcTestNetns,
		"dns_port":                              uint64(utils.HostToNetworkShort(53)),
		"raw_packet_limiter_rate":               10,
		"raw_packet_filter":                     enabled,
		"sched_cls_has_current_pid_tgid_helper": enabled,
		"sk_lookup_pid_enabled":                 enabled,
		"is_network_flow_monitor_enabled":       enabled,
		"is_span_tracking_enabled":              enabled,
	}
}

type tcTestPacket struct {
	name string
	data []byte
	// matched by tcTestDropActions
	dropped bool
}

func tcTestPackets(t *testing.T) []tcTestPacket {
	query := &layers.DNS{
		ID: 0x1234,
		RD: true,
		Questions: []layers.DNSQuestion{
			{Name: []byte("www.datadoghq.com"), Type: layers.DNSTypeA, Class: layers.DNSClassIN},
			// a second question makes the parser tail call itself
			{Name: []byte("www.datadoghq.eu"), Type: layers.DNSTypeAAAA, Class: layers.DNSClassIN},
		},
	}
	response := &layers.DNS{
		ID:        0x1234,
		QR:        true,
		RD:        true,
		RA:        true,
		Questions: query.Questions[:1],
		Answers: []layers.DNSResourceRecord{
			{Name: []byte("www.datadoghq.com"), Type: layers.DNSTypeA, Class: layers.DNSClassIN, TTL: 60, IP: net.IP{3, 233, 150, 1}},
		},
	}
	arp := &layers.ARP{
		AddrType:          layers.LinkTypeEthernet,
		Protocol:          layers.EthernetTypeIPv4,
		HwAddressSize:     6,
		ProtAddressSize:   4,
		Operation:         layers.ARPRequest,
		SourceHwAddress:   tcTestEthernet(layers.EthernetTypeARP).SrcMAC,
		SourceProtAddress: tcTestHostIP,
		DstHwAddress:      make(net.HardwareAddr, 6),
		DstProtAddress:    tcTestPeerIP,
	}
	imdsRequest := gopacket.Payload("GET /latest/meta-data/ HTTP/1.1\r\nHost: 169.254.169.254\r\n\r\n")

	return []tcTestPacket{
		{name: "dns-query", data: tcTestUDPPacket(t, tcTestHostIP, tcTestPeerIP, 41000, 53, query)},
		{name: "dns-response", data: tcTestUDPPacket(t, tcTestPeerIP, tcTestHostIP, 53, 41000, response)},
		{name: "imds-request", data: tcTestTCPPacket(t, tcTestIMDSIP, 80, &layers.TCP{PSH: true, ACK: true}, imdsRequest)},
		{name: "tcp-syn", data: tcTestTCPPacket(t, tcTestPeerIP, 5555, &layers.TCP{SYN: true}), dropped: true},
		{name: "arp", data: tcTestSerialize(t, tcTestEthernet(layers.EthernetTypeARP), arp)},
	}
}

func tcTestSerialize(t *testing.T, packetLayers ...gopacket.SerializableLayer) []byte {
	buf := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, packetLayers...))
	return buf.Bytes()
}

func tcTestEthernet(etherType layers.EthernetType) *layers.Ethernet {
	return &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x02, 0, 0, 0, 0, 0x02},
		DstMAC:       net.HardwareAddr{0x02, 0, 0, 0, 0, 0x03},
		EthernetType: etherType,
	}
}

func tcTestIPv4(src, dst net.IP, protocol layers.IPProtocol) *layers.IPv4 {
	return &layers.IPv4{Version: 4, TTL: 64, Protocol: protocol, SrcIP: src, DstIP: dst}
}

func tcTestUDPPacket(t *testing.T, src, dst net.IP, sport, dport uint16, payload gopacket.SerializableLayer) []byte {
	ip := tcTestIPv4(src, dst, layers.IPProtocolUDP)
	udp := &layers.UDP{SrcPort: layers.UDPPort(sport), DstPort: layers.UDPPort(dport)}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	return tcTestSerialize(t, tcTestEthernet(layers.EthernetTypeIPv4), ip, udp, payload)
}

func tcTestTCPPacket(t *testing.T, dst net.IP, dport uint16, tcp *layers.TCP, payload ...gopacket.SerializableLayer) []byte {
	ip := tcTestIPv4(tcTestHostIP, dst, layers.IPProtocolTCP)
	tcp.SrcPort, tcp.DstPort, tcp.Seq, tcp.Window = 41001, layers.TCPPort(dport), 1, 64240
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
	return tcTestSerialize(t, append([]gopacket.SerializableLayer{tcTestEthernet(layers.EthernetTypeIPv4), ip, tcp}, payload...)...)
}

// tcTestSpec returns the classifiers of spec, with the constants of env
func tcTestSpec(t *testing.T, spec *ebpf.CollectionSpec, env tcTestEnv) *ebpf.CollectionSpec {
	tcSpec := &ebpf.CollectionSpec{
		Maps:      maps.Clone(spec.Maps),
		Programs:  make(map[string]*ebpf.ProgramSpec),
		ByteOrder: spec.ByteOrder,
	}
	for name, progSpec := range spec.Programs {
		if progSpec.Type == ebpf.SchedCLS {
			progSpec = progSpec.Copy()
			// baloum selects programs by section, which classifiers can share
			progSpec.SectionName = name
			tcSpec.Programs[name] = progSpec
		}
	}

	for name, value := range env.constants() {
		var found bool
		for _, progSpec := range tcSpec.Programs {
			for i := range progSpec.Instructions {
				if ins := &progSpec.Instructions[i]; ins.Reference() == name && ins.IsConstantLoad(asm.DWord) {
					ins.Constant = int64(value)
					found = true
				}
			}
		}
		require.True(t, found, "no classifier loads the %s constant", name)
	}

	// baloum has no sk storage, which the emulated bpf_sk_storage_get doesn't read anyway
	skStorage := tcSpec.Maps["sk_storage_pid"].Copy()
	skStorage.Type, skStorage.MaxEntries = ebpf.Hash, 1
	tcSpec.Maps["sk_storage_pid"] = skStorage

	return tcSpec
}

type tcTestExit struct {
	prog string
	code int32
}

// tcTestRun runs a classifier in the baloum VM. Its instruction observer emulates the helpers baloum
// lacks, follows the tail calls, and records the program and the return code of each exit.
type tcTestRun struct {
	t      *testing.T
	vm     *baloum.VM
	env    tcTestEnv
	packet []byte
	// program names by baloum fd
	progs map[uint32]string
	// index of the tail call to fail, -1 for none
	failTailCall int
	tailCalls    int
	current      string
	ran          []string
	exits        []tcTestExit
	errs         []error
}

func newTCTestRun(t *testing.T, spec *ebpf.CollectionSpec, env tcTestEnv, failTailCall int) *tcTestRun {
	r := &tcTestRun{
		t:            t,
		env:          env,
		progs:        make(map[uint32]string),
		failTailCall: failTailCall,
	}
	r.vm = baloum.NewVM(spec, baloum.Opts{
		Fncs: baloum.Fncs{
			TracePrintk: func(_ *baloum.VM, format string, args ...interface{}) error {
				t.Logf(format, args...)
				return nil
			},
			KtimeGetNS: func(*baloum.VM) (uint64, error) {
				return tcTestNow, nil
			},
			GetCurrentPidTgid: func(*baloum.VM) (uint64, error) {
				return tcTestPid<<32 | tcTestPid, nil
			},
		},
		Logger:   &testLogger{t: t, trace: trace},
		Observer: r,
	})

	var imdsKey [16]byte
	copy(imdsKey[:], tcTestIMDSIP)
	r.updateMap("credential_endpoints", imdsKey[:], uint32(model.CredentialSourceIMDS))
	r.updateMap("enabled_events", uint32(0), uint64(math.MaxUint64))
	r.updateMap("raw_packet_enabled", uint32(0), uint32(1))
	r.updateMap("raw_packet_router_sel", uint32(0), uint32(0))
	if env.cookiePid {
		r.updateMap("sock_cookie_pid", uint64(tcTestCookie), uint32(tcTestPid))
	}

	for _, route := range probes.AllTailRoutes(false, true, false, true, false) {
		prog := route.ProbeIdentificationPair.EBPFFuncName
		if _, ok := spec.Programs[prog]; ok {
			fd, err := r.vm.LoadProgram(prog)
			require.NoError(t, err, prog)
			r.progs[fd] = prog
			r.updateMap(route.ProgArrayName, route.Key, fd)
		}
	}
	r.addRawPacketPrograms()

	return r
}

// addRawPacketPrograms installs filters and drop actions like the probe does
func (r *tcTestRun) addRawPacketPrograms() {
	routerName := probes.GetRawPacketClassifierRouterMapName(0)
	eventMap, router := r.loadMap("raw_packet_event"), r.loadMap(routerName)

	filterOpts := rawpacket.DefaultProgOpts()
	filterOpts.WithProgPrefix("raw_packet_filter_")
	filters, err := rawpacket.FiltersToProgramSpecs(eventMap.FD(), router.FD(), tcTestRawPacketFilters, filterOpts)
	require.NoError(r.t, err)

	dropOpts := rawpacket.DefaultProgOpts()
	dropOpts.WithProgPrefix("raw_packet_drop_action_")
	dropOpts.WithDropStatsMapFd(r.loadMap("dropped_packets").FD())
	drops, err := rawpacket.DropActionsToProgramSpecs(eventMap.FD(), router.FD(), tcTestDropActions, dropOpts)
	require.NoError(r.t, err)

	for key, progSpecs := range map[uint32][]*ebpf.ProgramSpec{
		probes.TCRawPacketFilterKey:     filters,
		probes.TCRawPacketDropActionKey: drops,
	} {
		for i, progSpec := range progSpecs {
			fd := r.vm.AddProgram(progSpec)
			r.progs[fd] = progSpec.Name
			r.updateMap(routerName, key+uint32(i), fd)
		}
	}
}

func (r *tcTestRun) loadMap(name string) *baloum.Map {
	m, err := r.vm.LoadMap(name)
	require.NoError(r.t, err, name)
	require.NotNil(r.t, m, "map %s not found", name)
	return m
}

func (r *tcTestRun) updateMap(name string, key, value interface{}) {
	updated, err := r.loadMap(name).Update(key, value, baloum.BPF_ANY)
	require.NoError(r.t, err, name)
	require.True(r.t, updated, name)
}

func (r *tcTestRun) run(prog string, packet []byte, ingress bool) int32 {
	skb := make([]byte, skbSize)
	binary.NativeEndian.PutUint32(skb[skbLenOffset:], uint32(len(packet)))
	binary.NativeEndian.PutUint32(skb[skbIfindexOffset:], tcTestIfindex)
	// skb->data stays 0: the classifiers only use packet pointers to compute bpf_skb_load_bytes offsets
	binary.NativeEndian.PutUint32(skb[skbDataEndOffset:], uint32(len(packet)))
	if !ingress {
		binary.NativeEndian.PutUint64(skb[skbSkOffset:], tcTestSock)
	}

	var regs baloum.Regs
	regs[asm.R1] = r.vm.Heap().AllocWith(skb)
	r.packet, r.current, r.ran = packet, prog, append(r.ran, prog)

	code, err := r.vm.RunProgram(&baloum.RawContext{Regs: regs}, prog, ebpf.SchedCLS)
	require.NoError(r.t, err, "programs: %v", r.ran)
	return int32(code)
}

// ObserveInst implements baloum.Observer
func (r *tcTestRun) ObserveInst(vm *baloum.VM, _ int, inst *asm.Instruction) {
	switch {
	case inst.OpCode.JumpOp() == asm.Exit:
		r.exits = append(r.exits, tcTestExit{prog: r.current, code: int32(vm.Regs()[asm.R0])})
	case inst.IsFunctionCall():
		r.errs = append(r.errs, fmt.Errorf("%s: BPF to BPF calls would return through an exit", r.current))
	case inst.IsBuiltinCall():
		r.callHelper(vm, inst)
	}
}

// callHelper emulates the helpers baloum lacks by replacing their call with their return value
func (r *tcTestRun) callHelper(vm *baloum.VM, inst *asm.Instruction) {
	regs := vm.Regs()

	var ret int64
	switch asm.BuiltinFunc(inst.Constant) {
	case asm.FnTailCall:
		if r.tailCall(vm, regs) {
			return
		}
		// r0 is undefined after a failed tail call, so a program returning it would return TC_ACT_OK
		ret = probes.TCActOk
	case asm.FnSkbLoadBytes:
		ret = r.skbLoadBytes(vm, regs)
	case asm.FnProbeReadUser, asm.FnProbeReadKernel:
		ret = r.probeRead(vm, regs)
	case asm.FnGetSocketCookie:
		if r.env.cookiePid {
			ret = tcTestCookie
		}
	case asm.FnSkLookupTcp, asm.FnSkLookupUdp:
		ret = tcTestSock
	case asm.FnSkStorageGet, asm.FnSkRelease, asm.FnSpinLock, asm.FnSpinUnlock, asm.FnGetCurrentTask:
	default:
		return
	}
	*inst = asm.Mov.Imm(asm.R0, int32(ret))
}

// tailCall reports whether the tail call jumps to a program, which becomes the current one
func (r *tcTestRun) tailCall(vm *baloum.VM, regs baloum.Regs) bool {
	index := r.tailCalls
	r.tailCalls++
	if index == r.failTailCall {
		return false
	}

	var progArray *baloum.Map
	for name, mapSpec := range vm.Spec.Maps {
		if m := vm.GetMapByName(name); mapSpec.Type == ebpf.ProgramArray && m != nil && m.FD() == int(regs[asm.R2]) {
			progArray = m
		}
	}
	if progArray == nil {
		r.errs = append(r.errs, fmt.Errorf("%s: tail call through map fd %d", r.current, regs[asm.R2]))
		return false
	}

	var fd uint32
	if err := progArray.Lookup(uint32(regs[asm.R3]), &fd); err != nil {
		return false
	}
	prog, ok := r.progs[fd]
	if ok {
		r.current = prog
		r.ran = append(r.ran, prog)
	}
	return ok
}

func (r *tcTestRun) skbLoadBytes(vm *baloum.VM, regs baloum.Regs) int64 {
	offset, size := uint64(uint32(regs[asm.R2])), uint64(uint32(regs[asm.R4]))
	dst, err := vm.GetBytes(regs[asm.R3], size)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: bpf_skb_load_bytes: %w", r.current, err))
		return -int64(unix.EFAULT)
	}
	dst = dst[:size]

	if offset+size > uint64(len(r.packet)) {
		clear(dst)
		return -int64(unix.EFAULT)
	}
	copy(dst, r.packet[offset:])
	return 0
}

func (r *tcTestRun) probeRead(vm *baloum.VM, regs baloum.Regs) int64 {
	size := uint64(uint32(regs[asm.R2]))
	dst, err := vm.GetBytes(regs[asm.R1], size)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: bpf_probe_read: %w", r.current, err))
		return -int64(unix.EFAULT)
	}
	dst = dst[:size]

	// only map values and the context live in the VM heap, task, user and NULL based pointers would
	// alias the VM stack
	if src := regs[asm.R3]; src&baloum.HEAP_ADDR_MASK != 0 {
		if data, err := vm.GetBytes(src, size); err == nil {
			copy(dst, data)
			return 0
		}
	}
	clear(dst)
	return -int64(unix.EFAULT)
}

// check asserts that the run exited once, with TC_ACT_UNSPEC or, for the drop action callback, TC_ACT_SHOT
func (r *tcTestRun) check(code int32) {
	assert.NoError(r.t, errors.Join(r.errs...))
	if assert.Len(r.t, r.exits, 1, "programs: %v", r.ran) {
		assert.Equal(r.t, r.exits[0].code, code, "programs: %v", r.ran)
	}
	for _, exit := range r.exits {
		if exit.code != probes.TCActShot || !probes.IsProgAllowedToTCActShot(exit.prog) {
			assert.Equal(r.t, int32(probes.TCActUnspec), exit.code, "%s returned %d, failed tail call: %d, programs: %v", exit.prog, exit.code, r.failTailCall, r.ran)
		}
	}
}

func TestTCReturnCodes(t *testing.T) {
	spec := loadColSpec(t)
	packets := tcTestPackets(t)
	progs, returned := make(map[string]bool), make(map[string]bool)

	for _, env := range tcTestEnvs {
		tcSpec := tcTestSpec(t, spec, env)

		for _, entry := range probes.GetTCProbes(true, true) {
			prog, ingress := entry.EBPFFuncName, entry.NetworkDirection == manager.Ingress
			progs[prog] = true

			for _, packet := range packets {
				t.Run(fmt.Sprintf("%s/%s/%s", env.name, prog, packet.name), func(t *testing.T) {
					// every tail call succeeds first, then each of them fails in turn
					for fail, tailCalls := -1, 0; fail < tailCalls; fail++ {
						r := newTCTestRun(t, tcSpec, env, fail)
						code := r.run(prog, packet.data, ingress)
						r.check(code)

						if fail == -1 {
							tailCalls = r.tailCalls
							want := int32(probes.TCActUnspec)
							if packet.dropped && prog == "classifier_raw_packet_egress" && (env.cookiePid || env.features) {
								want = probes.TCActShot
							}
							assert.Equal(t, want, code, "programs: %v", r.ran)
						}

						for _, name := range r.progs {
							progs[name] = true
						}
						for _, exit := range r.exits {
							returned[exit.prog] = true
						}
					}
				})
			}
		}
	}

	for prog := range progs {
		assert.True(t, returned[prog], "%s never returned", prog)
	}
}
