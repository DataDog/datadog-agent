// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/link"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// These helpers act on the devices of the network namespace the test runs in.

// createTestDevice creates a dummy interface, deleted at the end of the test
func createTestDevice(tb testing.TB, name string) netlink.Link {
	tb.Helper()

	dummy := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: name}}
	_ = netlink.LinkDel(dummy)
	if err := netlink.LinkAdd(dummy); err != nil {
		tb.Fatalf("couldn't create %s: %v", name, err)
	}
	tb.Cleanup(func() {
		_ = netlink.LinkDel(dummy)
	})

	device, err := netlink.LinkByName(name)
	if err != nil {
		tb.Fatalf("couldn't resolve %s: %v", name, err)
	}
	return device
}

// loadPassProgram loads a SCHED_CLS program returning 0: TC_ACT_OK for a direct-action filter, TCX_PASS for a
// TCX program. Both end the hook before the programs that follow.
func loadPassProgram(tb testing.TB, name string) *ebpf.Program {
	tb.Helper()

	prog, err := ebpf.NewProgram(&ebpf.ProgramSpec{
		Name:    name,
		Type:    ebpf.SchedCLS,
		License: "Apache-2.0",
		Instructions: asm.Instructions{
			asm.Mov.Imm(asm.R0, 0),
			asm.Return(),
		},
	})
	if err != nil {
		tb.Fatalf("couldn't load program %s: %v", name, err)
	}
	tb.Cleanup(func() {
		_ = prog.Close()
	})
	return prog
}

// attachForeignTCXProgram attaches a program returning TCX_PASS at the tail of a TCX hook, as Cilium does in
// TCX mode. It skips the test on kernels without TCX.
func attachForeignTCXProgram(tb testing.TB, device netlink.Link, direction manager.TrafficType, name string) {
	tb.Helper()

	attach := ebpf.AttachTCXEgress
	if direction == manager.Ingress {
		attach = ebpf.AttachTCXIngress
	}

	tcxLink, err := link.AttachTCX(link.TCXOptions{
		Interface: device.Attrs().Index,
		Program:   loadPassProgram(tb, name),
		Attach:    attach,
		Anchor:    link.Tail(),
	})
	if errors.Is(err, ebpf.ErrNotSupported) {
		tb.Skipf("TCX isn't supported: %v", err)
	}
	if err != nil {
		tb.Fatalf("couldn't attach %s to the %s TCX hook of %s: %v", name, direction, device.Attrs().Name, err)
	}
	tb.Cleanup(func() {
		_ = tcxLink.Close()
	})
}

// addForeignTCFilter adds a direct-action filter returning TC_ACT_OK, as Cilium does in legacy mode at
// priority 1. Its cleanup only removes this filter.
func addForeignTCFilter(tb testing.TB, device netlink.Link, direction manager.TrafficType, priority uint16, name string) {
	tb.Helper()

	qdisc := &netlink.GenericQdisc{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: device.Attrs().Index,
			Handle:    netlink.MakeHandle(0xffff, 0),
			Parent:    netlink.HANDLE_CLSACT,
		},
		QdiscType: "clsact",
	}
	if err := netlink.QdiscAdd(qdisc); err != nil && !errors.Is(err, unix.EEXIST) {
		tb.Fatalf("couldn't add a clsact qdisc to %s: %v", device.Attrs().Name, err)
	}

	filter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: device.Attrs().Index,
			Parent:    clsactParent(direction),
			Handle:    1,
			Priority:  priority,
			Protocol:  unix.ETH_P_ALL,
		},
		Fd:           loadPassProgram(tb, "tc_act_ok").FD(),
		Name:         name,
		DirectAction: true,
	}
	if err := netlink.FilterAdd(filter); err != nil {
		tb.Fatalf("couldn't add filter %s to the %s hook of %s: %v", name, direction, device.Attrs().Name, err)
	}
	tb.Cleanup(func() {
		if err := netlink.FilterDel(filter); err != nil && !errors.Is(err, unix.ENOENT) && !errors.Is(err, unix.ENODEV) {
			tb.Errorf("couldn't delete filter %s from the %s hook of %s: %v", name, direction, device.Attrs().Name, err)
		}
	})
}

// deleteTCFilters deletes every filter of a hook, the classifiers included, as Cilium does after it attaches
// in TCX mode
func deleteTCFilters(tb testing.TB, device netlink.Link, direction manager.TrafficType) {
	tb.Helper()

	filters, err := netlink.FilterList(device, clsactParent(direction))
	if err != nil {
		tb.Fatalf("couldn't list the filters of the %s hook of %s: %v", direction, device.Attrs().Name, err)
	}
	for _, filter := range filters {
		if err := netlink.FilterDel(filter); err != nil && !errors.Is(err, unix.ENOENT) {
			tb.Fatalf("couldn't delete a filter of the %s hook of %s: %v", direction, device.Attrs().Name, err)
		}
	}
}

func clsactParent(direction manager.TrafficType) uint32 {
	if direction == manager.Ingress {
		return netlink.HANDLE_MIN_INGRESS
	}
	return netlink.HANDLE_MIN_EGRESS
}
