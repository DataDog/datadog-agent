// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package offsetguess

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
)

// sizeof(struct nf_conntrack_tuple), see https://github.com/torvalds/linux/blob/master/include/net/netfilter/nf_conntrack_tuple.h
const sizeofNfConntrackTuple = 40

type conntrackOffsetGuesser struct {
	m      *manager.Manager
	status *netebpf.ConntrackStatus
}

func NewConntrackOffsetGuesser(consts []manager.ConstantEditor) (OffsetGuesser, error) {
	var offsetIno uint64
	for _, c := range consts {
		if c.Name == "offset_ino" {
			offsetIno = c.Value.(uint64)
			break
		}
	}

	if offsetIno == 0 {
		return nil, fmt.Errorf("ino offset is 0")
	}

	return &conntrackOffsetGuesser{
		m: &manager.Manager{
			Maps: []*manager.Map{
				{Name: probes.ConntrackStatusMap},
			},
			PerfMaps: []*manager.PerfMap{},
			Probes: []*manager.Probe{
				{ProbeIdentificationPair: idPair(probes.ConntrackHashInsert)},
				// have to add this for older kernels since loading
				// it twice in a process (once by the tracer offset guesser)
				// does not seem to work; this will be not be enabled,
				// so explicitly disabled, and the manager won't load it
				{ProbeIdentificationPair: idPair(probes.NetDevQueue)},
			},
		},
		status: &netebpf.ConntrackStatus{Offset_ino: offsetIno},
	}, nil
}

func (c *conntrackOffsetGuesser) Manager() *manager.Manager {
	return c.m
}

func (c *conntrackOffsetGuesser) Probes(cfg *config.Config) (map[probes.ProbeFuncName]struct{}, error) {
	p := map[probes.ProbeFuncName]struct{}{}
	enableProbe(p, probes.ConntrackHashInsert)
	return p, nil
}

func (c *conntrackOffsetGuesser) getConstantEditors() []manager.ConstantEditor {
	return []manager.ConstantEditor{
		{Name: "offset_ct_origin", Value: c.status.Offset_origin},
		{Name: "offset_ct_reply", Value: c.status.Offset_reply},
		{Name: "offset_ct_status", Value: c.status.Offset_status},
		{Name: "offset_ct_netns", Value: c.status.Offset_netns},
		{Name: "offset_ct_ino", Value: c.status.Offset_ino},
	}
}

// checkAndUpdateCurrentOffset checks the value for the current offset stored
// in the eBPF map against the expected value, incrementing the offset if it
// doesn't match, or going to the next field to guess if it does
func (c *conntrackOffsetGuesser) checkAndUpdateCurrentOffset(mp *ebpf.Map, expected *fieldValues, maxRetries *int, threshold uint64, protocolClassificationSupported bool) error {
	// get the updated map value so we can check if the current offset is
	// the right one
	if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(c.status)); err != nil {
		return fmt.Errorf("error reading conntrack_status: %v", err)
	}

	if netebpf.State(c.status.State) != netebpf.StateChecked {
		if *maxRetries == 0 {
			return fmt.Errorf("invalid guessing state while guessing %v, got %v expected %v",
				whatString[netebpf.GuessWhat(c.status.What)], stateString[netebpf.State(c.status.State)], stateString[netebpf.StateChecked])
		}
		*maxRetries--
		time.Sleep(10 * time.Millisecond)
		return nil
	}
	switch netebpf.GuessWhat(c.status.What) {
	case netebpf.GuessCtTupleOrigin:
		if c.status.Saddr == expected.saddr {
			// the reply tuple comes always after the origin tuple
			c.status.Offset_reply = c.status.Offset_origin + sizeofNfConntrackTuple
			c.logAndAdvance(c.status.Offset_origin, netebpf.GuessCtTupleReply)
			break
		}
		c.status.Offset_origin++
		c.status.Saddr = expected.saddr
	case netebpf.GuessCtTupleReply:
		if c.status.Saddr == expected.daddr {
			c.logAndAdvance(c.status.Offset_reply, netebpf.GuessCtStatus)
			break
		}
		c.status.Offset_reply++
		c.status.Saddr = expected.saddr
	case netebpf.GuessCtStatus:
		if c.status.Status == expected.ctStatus {
			c.status.Offset_netns = c.status.Offset_status + 1
			c.logAndAdvance(c.status.Offset_status, netebpf.GuessCtNet)
			break
		}
		c.status.Offset_status++
		c.status.Status = expected.ctStatus
	case netebpf.GuessCtNet:
		if c.status.Netns == expected.netns {
			c.logAndAdvance(c.status.Offset_netns, netebpf.GuessNotApplicable)
			return c.setReadyState(mp)
		}
		c.status.Offset_netns++
		c.status.Netns = expected.netns
	default:
		return fmt.Errorf("unexpected field to guess: %v", whatString[netebpf.GuessWhat(c.status.What)])
	}

	c.status.State = uint64(netebpf.StateChecking)
	// update the map with the new offset/field to check
	if err := mp.Put(unsafe.Pointer(&zero), unsafe.Pointer(c.status)); err != nil {
		return fmt.Errorf("error updating tracer_t.status: %v", err)
	}

	return nil

}

func (c *conntrackOffsetGuesser) setReadyState(mp *ebpf.Map) error {
	c.status.State = uint64(netebpf.StateReady)
	if err := mp.Put(unsafe.Pointer(&zero), unsafe.Pointer(c.status)); err != nil {
		return fmt.Errorf("error updating tracer_status: %v", err)
	}
	return nil
}

func (c *conntrackOffsetGuesser) logAndAdvance(offset uint64, next netebpf.GuessWhat) {
	guess := netebpf.GuessWhat(c.status.What)
	if offset != notApplicable {
		log.Debugf("Successfully guessed %v with offset of %d bytes", whatString[guess], offset)
	} else {
		log.Debugf("Could not guess offset for %v", whatString[guess])
	}
	if next != netebpf.GuessNotApplicable {
		log.Debugf("Started offset guessing for %v", whatString[next])
		c.status.What = uint64(next)
	}
}

func (c *conntrackOffsetGuesser) Guess(cfg *config.Config) ([]manager.ConstantEditor, error) {
	mp, _, err := c.m.GetMap(probes.ConntrackStatusMap)
	if err != nil {
		return nil, fmt.Errorf("unable to find map %s: %s", probes.ConntrackStatusMap, err)
	}

	// When reading kernel structs at different offsets, don't go over the set threshold
	// Defaults to 400, with a max of 3000. This is an arbitrary choice to avoid infinite loops.
	threshold := cfg.OffsetGuessThreshold

	// pid & tid must not change during the guessing work: the communication
	// between ebpf and userspace relies on it
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	processName := filepath.Base(os.Args[0])
	if len(processName) > netebpf.ProcCommMaxLen { // Truncate process name if needed
		processName = processName[:netebpf.ProcCommMaxLen]
	}

	cProcName := [netebpf.ProcCommMaxLen + 1]int8{} // Last char has to be null character, so add one
	for i, ch := range processName {
		cProcName[i] = int8(ch)
	}

	c.status.State = uint64(netebpf.StateChecking)
	c.status.Proc = netebpf.Proc{Comm: cProcName}
	c.status.What = uint64(netebpf.GuessCtTupleOrigin)

	// if we already have the offsets, just return
	err = mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(c.status))
	if err == nil && netebpf.State(c.status.State) == netebpf.StateReady {
		return c.getConstantEditors(), nil
	}

	eventGenerator, err := newConntrackEventGenerator()
	if err != nil {
		return nil, err
	}
	defer eventGenerator.Close()

	// initialize map
	if err := mp.Put(unsafe.Pointer(&zero), unsafe.Pointer(c.status)); err != nil {
		return nil, fmt.Errorf("error initializing conntrack_c.status map: %v", err)
	}

	// If the kretprobe for tcp_v4_connect() is configured with a too-low maxactive, some kretprobe might be missing.
	// In this case, we detect it and try again. See: https://github.com/weaveworks/tcptracer-bpf/issues/24
	maxRetries := 100

	log.Debugf("Checking for offsets with threshold of %d", threshold)
	expected := &fieldValues{}
	for netebpf.State(c.status.State) != netebpf.StateReady {
		if err := eventGenerator.Generate(netebpf.GuessWhat(c.status.What), expected); err != nil {
			return nil, err
		}

		if err := c.checkAndUpdateCurrentOffset(mp, expected, &maxRetries, threshold, false); err != nil {
			return nil, err
		}

		// Stop at a reasonable offset so we don't run forever.
		// Reading too far away in kernel memory is not a big deal:
		// probe_kernel_read() handles faults gracefully.
		if c.status.Offset_netns >= threshold || c.status.Offset_status >= threshold ||
			c.status.Offset_origin >= threshold || c.status.Offset_reply >= threshold {
			return nil, fmt.Errorf("overflow while guessing %v, bailing out", whatString[netebpf.GuessWhat(c.status.What)])
		}
	}

	return c.getConstantEditors(), nil
}

type conntrackEventGenerator struct {
	udpAddr string
	udpDone func()
}

func newConntrackEventGenerator() (*conntrackEventGenerator, error) {
	eg := &conntrackEventGenerator{}

	// port 0 means we let the kernel choose a free port
	var err error
	addr := fmt.Sprintf("%s:0", listenIPv4)
	// Spin up UDP server
	eg.udpAddr, eg.udpDone, err = newUDPServer(addr)
	if err != nil {
		eg.Close()
		return nil, err
	}

	return eg, nil
}

// Generate an event for offset guessing
func (e *conntrackEventGenerator) Generate(status netebpf.GuessWhat, expected *fieldValues) error {
	if status >= netebpf.GuessCtTupleOrigin &&
		status <= netebpf.GuessCtNet {
		c, err := net.DialTimeout("udp4", e.udpAddr, 500*time.Millisecond)
		if err != nil {
			return err
		}

		if err = e.populateUDPExpectedValues(c, expected); err != nil {
			return err
		}

		_, err = c.Write([]byte("foo"))
		return err
	}

	return fmt.Errorf("invalid status %v", status)
}

func (e *conntrackEventGenerator) populateUDPExpectedValues(c net.Conn, expected *fieldValues) error {
	saddr, daddr, _, _, err := extractIPsAndPorts(c)
	if err != nil {
		return err
	}

	expected.saddr = saddr
	expected.daddr = daddr
	// IPS_CONFIRMED | IPS_SRC_NAT_DONE | IPS_DST_NAT_DONE
	// see https://elixir.bootlin.com/linux/v5.19.17/source/include/uapi/linux/netfilter/nf_conntrack_common.h#L42
	expected.ctStatus = 0x188
	expected.netns, err = util.GetCurrentIno()
	if err != nil {
		return err
	}

	return nil
}

func (e *conntrackEventGenerator) Close() {
	if e.udpDone != nil {
		e.udpDone()
	}
}
