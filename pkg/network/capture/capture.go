// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && pcap && cgo

package capture

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// ringBufMetaSize is the number of bytes of metadata that prefix every ring
// buffer record before the raw packet bytes.
//
// Layout (little-endian, packed):
//
//	+0  ktime_ns  uint64  — nanoseconds from bpf_ktime_get_ns()
//	+8  orig_len  uint32  — on-wire packet length (skb->len)
//	+12 ifindex   uint32  — skb->ifindex
//	+16 ingress   uint32  — 1 if TC ingress hook, 0 if egress
//	+20 _pad      uint32  — reserved, always 0
//
// Total: 24 bytes. Must match recXxxOffset constants in buildProgram().
const ringBufMetaSize = 24

// Register allocation for the generated TC programs (eBPF calling convention):
//
//	R0  — helper return value / temporary
//	R1–R5 — helper arguments (clobbered on every helper call)
//	R6  — skb (ctx), saved in prologue, survives all helper calls
//	R7  — ring buffer reservation pointer, set after bpf_ringbuf_reserve
//	R8  — skb->len snapshot, set in prologue, survives all helper calls
//	R9  — skb->ifindex snapshot, set in prologue, survives all helper calls
//
// cbpfc uses Working=[R1,R2,R3,R4] so R5–R9 are never clobbered by the filter.
// cbpfc's PacketStart=R1, PacketEnd=R2, Result=R3.

// capturer is the concrete implementation of the Capturer interface.
type capturer struct {
	cfg     CaptureConfig
	snapLen uint32

	// Compiled eBPF filter instructions (nil = match-all).
	filterInsts asm.Instructions

	// eBPF objects — loaded on Start.
	ringBufMap *ebpf.Map

	// We load up to two programs: one for ingress, one for egress.
	// When Direction == DirectionBoth both are non-nil.
	progIngress *ebpf.Program
	progEgress  *ebpf.Program

	// TC attachment state.
	netlinkHandle *netlink.Handle
	qdiscAdded    bool

	// Ring buffer reader.
	reader *ringbuf.Reader

	// Background goroutine lifecycle.
	stopCh        chan struct{}
	doneCh        chan struct{}
	// drainStarted is set to true just before the drain goroutine is launched
	// so that Stop() knows whether to wait on doneCh.
	drainStarted atomic.Bool

	// Statistics — all updated atomically.
	packetsCaptured atomic.Uint64
	packetsDropped  atomic.Uint64
	bytesCaptured   atomic.Uint64
	errCount        atomic.Uint64

	// startTime / endTime protected by mu.
	mu        sync.Mutex
	startTime time.Time
	endTime   time.Time

	// Guards against double-Start / double-Stop.
	started atomic.Bool
	stopped atomic.Bool

	// Serialises writes to cfg.Output.
	writeMu sync.Mutex
}

// newCapturer validates cfg, applies defaults, and compiles the BPF filter.
// It does not allocate any kernel resources.
func newCapturer(cfg CaptureConfig) (*capturer, error) {
	if cfg.Iface == nil {
		return nil, errors.New("capture: Iface must not be nil")
	}
	if cfg.Output == nil {
		return nil, errors.New("capture: Output must not be nil")
	}

	cfg.applyDefaults()

	var filterInsts asm.Instructions
	if cfg.Filter != "" {
		raw, err := compileBPFFilter(cfg.Filter, cfg.SnapLen)
		if err != nil {
			return nil, fmt.Errorf("capture: %w", err)
		}
		if len(raw) > 0 {
			filterInsts, err = bpfToEBPF(raw)
			if err != nil {
				return nil, fmt.Errorf("capture: %w", err)
			}
		}
	}

	return &capturer{
		cfg:         cfg,
		snapLen:     cfg.SnapLen,
		filterInsts: filterInsts,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}, nil
}

// Start implements Capturer. It allocates kernel resources, attaches TC hooks,
// writes the PCAP global header, and launches the drain goroutine.
func (c *capturer) Start(ctx context.Context) error {
	if !c.started.CompareAndSwap(false, true) {
		return errors.New("capture: already started")
	}

	c.mu.Lock()
	c.startTime = time.Now()
	c.mu.Unlock()

	// All cleanup paths use closeResources to avoid repetition.
	var (
		rbMap       *ebpf.Map
		progI       *ebpf.Program
		progE       *ebpf.Program
		nlh         *netlink.Handle
		rbReader    *ringbuf.Reader
		tcAttached  bool
	)

	cleanup := func() {
		// Immediately close the doneCh so that any concurrent Stop() call will
		// not block on <-doneCh. Since the drain goroutine was never started,
		// nobody else will close doneCh.
		// We also mark stopped=true so that Stop() returns immediately.
		c.stopped.Store(true)

		if tcAttached {
			c.detachTC()
		}
		if nlh != nil {
			nlh.Close()
		}
		if rbReader != nil {
			rbReader.Close()
		}
		if progI != nil {
			progI.Close()
		}
		if progE != nil {
			progE.Close()
		}
		if rbMap != nil {
			rbMap.Close()
		}
	}

	var err error

	// 1. Create ring buffer map.
	rbMap, err = ebpf.NewMap(&ebpf.MapSpec{
		Name:       "dd_pcap_rb",
		Type:       ebpf.RingBuf,
		MaxEntries: uint32(c.cfg.RingBufferSize),
	})
	if err != nil {
		cleanup()
		return fmt.Errorf("capture: creating ring buffer map: %w", err)
	}
	c.ringBufMap = rbMap

	// 2. Build and load eBPF programs.
	needIngress := c.cfg.Direction == DirectionBoth || c.cfg.Direction == DirectionIngress
	needEgress := c.cfg.Direction == DirectionBoth || c.cfg.Direction == DirectionEgress

	if needIngress {
		progI, err = c.buildProgram(true /* ingress */)
		if err != nil {
			cleanup()
			return fmt.Errorf("capture: loading ingress eBPF program: %w", err)
		}
		c.progIngress = progI
	}
	if needEgress {
		progE, err = c.buildProgram(false /* egress */)
		if err != nil {
			cleanup()
			return fmt.Errorf("capture: loading egress eBPF program: %w", err)
		}
		c.progEgress = progE
	}

	// 3. Open netlink handle.
	nlh, err = netlink.NewHandle()
	if err != nil {
		cleanup()
		return fmt.Errorf("capture: opening netlink handle: %w", err)
	}
	c.netlinkHandle = nlh

	// 4. Attach TC hooks.
	if err = c.attachTC(); err != nil {
		cleanup()
		return fmt.Errorf("capture: attaching TC hook: %w", err)
	}
	tcAttached = true

	// 5. Create ring buffer reader.
	rbReader, err = ringbuf.NewReader(c.ringBufMap)
	if err != nil {
		cleanup()
		return fmt.Errorf("capture: creating ring buffer reader: %w", err)
	}
	c.reader = rbReader

	// 6. Write PCAP global header.
	c.writeMu.Lock()
	writeErr := writePCAPHeader(c.cfg.Output, c.snapLen)
	c.writeMu.Unlock()
	if writeErr != nil {
		cleanup()
		return fmt.Errorf("capture: writing PCAP header: %w", writeErr)
	}

	// 7. Spawn drain goroutine.
	c.drainStarted.Store(true)
	go c.drainLoop(ctx)

	return nil
}

// Stop implements Capturer. Closes the reader (unblocking drainLoop), waits
// for it to exit, then detaches TC hooks and frees kernel resources.
func (c *capturer) Stop() error {
	if !c.stopped.CompareAndSwap(false, true) {
		return nil
	}

	// Signal drain goroutine to stop.
	close(c.stopCh)

	// Unblock any in-progress reader.Read() call.
	var readerErr error
	if c.reader != nil {
		readerErr = c.reader.Close()
	}

	// Wait for the drain goroutine to exit (only if it was launched).
	if c.drainStarted.Load() {
		<-c.doneCh
	}

	// Detach TC filters (after goroutine exits to avoid writing to a detached
	// program's ring buffer after cleanup).
	c.detachTC()

	if c.netlinkHandle != nil {
		c.netlinkHandle.Close()
	}
	if c.progIngress != nil {
		c.progIngress.Close()
	}
	if c.progEgress != nil {
		c.progEgress.Close()
	}
	if c.ringBufMap != nil {
		c.ringBufMap.Close()
	}

	c.mu.Lock()
	c.endTime = time.Now()
	c.mu.Unlock()

	return readerErr
}

// Stats implements Capturer.
func (c *capturer) Stats() CaptureStats {
	c.mu.Lock()
	start := c.startTime
	end := c.endTime
	c.mu.Unlock()

	return CaptureStats{
		PacketsCaptured: c.packetsCaptured.Load(),
		PacketsDropped:  c.packetsDropped.Load(),
		BytesCaptured:   c.bytesCaptured.Load(),
		StartTime:       start,
		EndTime:         end,
		Errors:          c.errCount.Load(),
	}
}

// drainLoop is the background goroutine that reads ring buffer records and
// writes PCAP packets. Terminates on: stop signal, context cancellation,
// reader close, MaxPackets limit, or Duration expiry.
func (c *capturer) drainLoop(ctx context.Context) {
	defer close(c.doneCh)

	var deadline <-chan time.Time
	if c.cfg.Duration > 0 {
		t := time.NewTimer(c.cfg.Duration)
		defer t.Stop()
		deadline = t.C
	}

	for {
		// Non-blocking check for stop conditions before blocking on Read.
		select {
		case <-c.stopCh:
			return
		case <-ctx.Done():
			return
		case <-deadline:
			return
		default:
		}

		rec, err := c.reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return
			}
			c.errCount.Add(1)
			continue
		}

		// LostSamples > 0 means the ring buffer overflowed before this record.
		if rec.LostSamples > 0 {
			c.packetsDropped.Add(rec.LostSamples)
		}

		if len(rec.RawSample) < ringBufMetaSize {
			c.errCount.Add(1)
			continue
		}

		pkt, ok := parseRecord(rec.RawSample)
		if !ok {
			c.errCount.Add(1)
			continue
		}

		// Direction filter: the eBPF program already filters by direction at
		// submission time, but we double-check here for correctness.
		if !c.directionAllowed(pkt.Ingress) {
			continue
		}

		c.writeMu.Lock()
		writeErr := writePCAPPacket(c.cfg.Output, pkt)
		c.writeMu.Unlock()
		if writeErr != nil {
			c.errCount.Add(1)
			continue
		}

		c.packetsCaptured.Add(1)
		c.bytesCaptured.Add(uint64(len(pkt.Data)))

		if c.cfg.MaxPackets > 0 && c.packetsCaptured.Load() >= c.cfg.MaxPackets {
			return
		}
	}
}

// parseRecord decodes a raw ring buffer sample into a RawPacket.
//
// Record layout (must match buildProgram's metadata writes):
//
//	[0:8]   ktime_ns uint64
//	[8:12]  orig_len uint32
//	[12:16] ifindex  uint32
//	[16:20] ingress  uint32  (1=ingress, 0=egress)
//	[20:24] _pad     uint32
//	[24:]   packet data bytes
func parseRecord(raw []byte) (RawPacket, bool) {
	if len(raw) < ringBufMetaSize {
		return RawPacket{}, false
	}

	le := binary.LittleEndian
	origLen := le.Uint32(raw[8:12])
	ifindex := le.Uint32(raw[12:16])
	ingress := le.Uint32(raw[16:20]) != 0

	data := make([]byte, len(raw)-ringBufMetaSize)
	copy(data, raw[ringBufMetaSize:])

	// We use the wall-clock time at read time as the packet timestamp.
	// bpf_ktime_get_ns() returns boot-relative monotonic time; converting to
	// wall-clock accurately requires reading the boot time offset from
	// /proc/stat or clock_gettime(CLOCK_BOOTTIME). Using time.Now() is a
	// reasonable approximation for capture files.
	return RawPacket{
		Timestamp: time.Now(),
		Data:      data,
		OrigLen:   origLen,
		IfIndex:   ifindex,
		Ingress:   ingress,
	}, true
}

// directionAllowed returns true if pkt direction should be written to output.
func (c *capturer) directionAllowed(ingress bool) bool {
	switch c.cfg.Direction {
	case DirectionIngress:
		return ingress
	case DirectionEgress:
		return !ingress
	default:
		return true
	}
}

// buildProgram generates a TC SchedCLS eBPF program for one traffic direction.
//
// Program structure:
//
//  1. Prologue: save ctx in R6, snapshot skb->len in R8, skb->ifindex in R9.
//  2. Reserve ring buffer slot (ringBufMetaSize + snapLen bytes).
//  3. bpf_skb_load_bytes → packet data into reservation[recDataOffset:].
//  4. Set R1=data_start, R2=data_end; run cbpfc filter (result in R3).
//  5. If filter miss: bpf_ringbuf_discard; return TC_ACT_UNSPEC.
//  6. If filter hit: write metadata (ktime, orig_len, ifindex, ingress).
//  7. bpf_ringbuf_submit; return TC_ACT_UNSPEC.
//
// Register allocation (across helper calls, R6–R9 survive):
//
//	R6  skb (ctx)
//	R7  ring buffer reservation pointer
//	R8  skb->len  (original packet length)
//	R9  skb->ifindex
//
// cbpfc is configured with Working=[R1,R2,R3,R4] so R5–R9 are untouched by the
// filter. cbpfc's PacketStart=R1, PacketEnd=R2, Result=R3.
func (c *capturer) buildProgram(ingress bool) (*ebpf.Program, error) {
	const (
		tcActUnspec = -1

		// Accessible __sk_buff field offsets for TC programs (linux/bpf.h).
		// These are the offsets used by bpf_load_mem in SchedCLS context.
		skbLenOffset     int16 = 0
		skbIfindexOffset int16 = 40

		// Ring buffer record metadata offsets (must match parseRecord).
		recKtimeOffset   int16 = 0
		recOrigLenOffset int16 = 8
		recIfindexOffset int16 = 12
		recIngressOffset int16 = 16
		recDataOffset    int16 = ringBufMetaSize
	)

	rbFd := c.ringBufMap.FD()
	recSize := int32(ringBufMetaSize) + int32(c.snapLen)

	ingressVal := int32(0)
	if ingress {
		ingressVal = 1
	}

	suffix := "egress"
	if ingress {
		suffix = "ingress"
	}

	var insts asm.Instructions

	// ── 1. Prologue ──────────────────────────────────────────────────────────
	insts = append(insts,
		asm.Mov.Reg(asm.R6, asm.R1),                                        // R6 = skb
		asm.LoadMem(asm.R8, asm.R6, int16(skbLenOffset), asm.Word),         // R8 = skb->len
		asm.LoadMem(asm.R9, asm.R6, int16(skbIfindexOffset), asm.Word),     // R9 = skb->ifindex
	)

	// ── 2. Reserve ring buffer slot ───────────────────────────────────────────
	// bpf_ringbuf_reserve(map, size, flags) → ptr or NULL
	insts = append(insts,
		asm.LoadMapPtr(asm.R1, rbFd),
		asm.Mov.Imm(asm.R2, recSize),
		asm.Mov.Imm(asm.R3, 0), // flags = 0
		asm.FnRingbufReserve.Call(),
		// NULL return → no space, pass packet and skip capture.
		asm.JNE.Imm(asm.R0, 0, "rb_reserved"),
		asm.Mov.Imm(asm.R0, tcActUnspec),
		asm.Return(),
		asm.Mov.Reg(asm.R7, asm.R0).WithSymbol("rb_reserved"), // R7 = reservation
	)

	// ── 3. Load packet data into reservation[recDataOffset:] ─────────────────
	// bpf_skb_load_bytes(skb, offset, to, len) → 0 on success, negative on error
	insts = append(insts,
		asm.Mov.Reg(asm.R1, asm.R6),                   // R1 = skb
		asm.Mov.Imm(asm.R2, 0),                         // R2 = byte offset = 0
		asm.Mov.Reg(asm.R3, asm.R7),                    // R3 = reservation base
		asm.Add.Imm(asm.R3, int32(recDataOffset)),       // R3 = &reservation[recDataOffset]
		asm.Mov.Imm(asm.R4, int32(c.snapLen)),           // R4 = length to load
		asm.FnSkbLoadBytes.Call(),                        // R0 = 0 or <0
		// Non-zero (error) → discard reservation and pass.
		asm.JEq.Imm(asm.R0, 0, "skb_load_ok"),
		asm.Mov.Reg(asm.R1, asm.R7),
		asm.Mov.Imm(asm.R2, 0),
		asm.FnRingbufDiscard.Call(),
		asm.Mov.Imm(asm.R0, tcActUnspec),
		asm.Return(),
	)

	// ── 4. Run cbpfc filter (optional) ───────────────────────────────────────
	// Set R1 = data start pointer, R2 = data end pointer for cbpfc.
	insts = append(insts,
		asm.Mov.Reg(asm.R1, asm.R7).WithSymbol("skb_load_ok"), // R1 = reservation
		asm.Add.Imm(asm.R1, int32(recDataOffset)),               // R1 = data start
		asm.Mov.Reg(asm.R2, asm.R1),
		asm.Add.Imm(asm.R2, int32(c.snapLen)),                   // R2 = data end
	)

	if len(c.filterInsts) > 0 {
		// filterInsts: uses R1(start), R2(end), R3(result), R4 scratch.
		// R6–R9 are untouched (cbpfc Working=[R1,R2,R3,R4]).
		insts = append(insts, c.filterInsts...)

		// R3 = 0 → no match.
		insts = append(insts,
			asm.JNE.Imm(asm.R3, 0, "filter_matched"),
			asm.Mov.Reg(asm.R1, asm.R7),
			asm.Mov.Imm(asm.R2, 0),
			asm.FnRingbufDiscard.Call(),
			asm.Mov.Imm(asm.R0, tcActUnspec),
			asm.Return(),
			// Anchor label — must be a real instruction.
			asm.Mov.Reg(asm.R0, asm.R0).WithSymbol("filter_matched"),
		)
	}

	// ── 5. Write metadata ─────────────────────────────────────────────────────
	// bpf_ktime_get_ns() — R0 = ktime_ns. Clobbers R1–R5; R6–R9 survive.
	insts = append(insts,
		asm.FnKtimeGetNs.Call(), // R0 = ktime_ns
		asm.StoreMem(asm.R7, recKtimeOffset, asm.R0, asm.DWord),   // rec[0]  = ktime_ns
		asm.StoreMem(asm.R7, recOrigLenOffset, asm.R8, asm.Word),   // rec[8]  = orig_len
		asm.StoreMem(asm.R7, recIfindexOffset, asm.R9, asm.Word),   // rec[12] = ifindex
		asm.StoreImm(asm.R7, recIngressOffset, ingressVal, asm.Word), // rec[16] = ingress (constant)
		asm.StoreImm(asm.R7, recIngressOffset+4, 0, asm.Word),      // rec[20] = pad = 0
	)

	// ── 6. Submit and return ──────────────────────────────────────────────────
	insts = append(insts,
		asm.Mov.Reg(asm.R1, asm.R7),
		asm.Mov.Imm(asm.R2, 0), // flags = 0
		asm.FnRingbufSubmit.Call(),
		asm.Mov.Imm(asm.R0, tcActUnspec),
		asm.Return(),
	)

	spec := &ebpf.ProgramSpec{
		Name:         progPrefix + suffix,
		Type:         ebpf.SchedCLS,
		License:      "GPL",
		Instructions: insts,
	}

	prog, err := ebpf.NewProgram(spec)
	if err != nil {
		return nil, fmt.Errorf("loading %s eBPF program: %w", suffix, err)
	}
	return prog, nil
}

// attachTC adds a clsact qdisc and attaches BPF TC filters to the interface.
func (c *capturer) attachTC() error {
	iface := c.cfg.Iface

	// clsact qdisc — required for ingress and egress TC hooks.
	// EEXIST means it's already present (e.g. from another tool); that's fine.
	qdisc := &netlink.GenericQdisc{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: iface.Index,
			Handle:    netlink.MakeHandle(0xffff, 0),
			Parent:    netlink.HANDLE_CLSACT,
		},
		QdiscType: "clsact",
	}
	if err := c.netlinkHandle.QdiscAdd(qdisc); err != nil {
		if !errors.Is(err, unix.EEXIST) {
			return fmt.Errorf("adding clsact qdisc on %s: %w", iface.Name, err)
		}
		// Already exists — do not remove it on Stop.
	} else {
		c.qdiscAdded = true
	}

	attachFilter := func(prog *ebpf.Program, parent uint32, suffix string) error {
		f := &netlink.BpfFilter{
			FilterAttrs: netlink.FilterAttrs{
				LinkIndex: iface.Index,
				Parent:    parent,
				Handle:    netlink.MakeHandle(0, 1),
				Protocol:  unix.ETH_P_ALL,
				// Priority 49152 (0xC000) is in the upper half of the u16 range.
				// Existing tools (e.g. the Security Agent) typically use lower
				// priorities, so we are unlikely to conflict.
				Priority: 49152,
			},
			Fd:           prog.FD(),
			Name:         progPrefix + suffix,
			DirectAction: true,
		}
		if err := c.netlinkHandle.FilterAdd(f); err != nil {
			return fmt.Errorf("adding BPF TC %s filter on %s: %w", suffix, iface.Name, err)
		}
		return nil
	}

	if c.progIngress != nil {
		if err := attachFilter(c.progIngress, netlink.HANDLE_MIN_INGRESS, "ingress"); err != nil {
			return err
		}
	}
	if c.progEgress != nil {
		if err := attachFilter(c.progEgress, netlink.HANDLE_MIN_EGRESS, "egress"); err != nil {
			return err
		}
	}
	return nil
}

// detachTC removes the BPF TC filters we attached and, if we created the clsact
// qdisc, removes it. Errors are silently ignored — cleanup is best-effort.
func (c *capturer) detachTC() {
	if c.netlinkHandle == nil || c.cfg.Iface == nil {
		return
	}

	iface := c.cfg.Iface

	removeFilter := func(parent uint32, suffix string) {
		f := &netlink.BpfFilter{
			FilterAttrs: netlink.FilterAttrs{
				LinkIndex: iface.Index,
				Parent:    parent,
				Handle:    netlink.MakeHandle(0, 1),
				Protocol:  unix.ETH_P_ALL,
				Priority:  49152,
			},
			Name: progPrefix + suffix,
		}
		_ = c.netlinkHandle.FilterDel(f)
	}

	if c.progIngress != nil {
		removeFilter(netlink.HANDLE_MIN_INGRESS, "ingress")
	}
	if c.progEgress != nil {
		removeFilter(netlink.HANDLE_MIN_EGRESS, "egress")
	}

	if c.qdiscAdded {
		qdisc := &netlink.GenericQdisc{
			QdiscAttrs: netlink.QdiscAttrs{
				LinkIndex: iface.Index,
				Handle:    netlink.MakeHandle(0xffff, 0),
				Parent:    netlink.HANDLE_CLSACT,
			},
			QdiscType: "clsact",
		}
		_ = c.netlinkHandle.QdiscDel(qdisc)
		c.qdiscAdded = false
	}
}
