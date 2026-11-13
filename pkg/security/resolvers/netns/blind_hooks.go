// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// maxListedBlindHooks caps the number of hooks listed by the blind hooks warning
const maxListedBlindHooks = 10

// BlindHook is a hook of a host interface where a foreign program runs before the classifiers
type BlindHook struct {
	Device    string
	Direction manager.TrafficType
	Program   tc.ProgramAhead
}

// tcxQuerier lists the TCX programs of hooks, until the kernel turns out not to support TCX
type tcxQuerier struct {
	unsupported bool
	query       func(ifindex int, direction manager.TrafficType) ([]tc.TCXProgram, error)
}

// programs returns the TCX programs of a hook, in run order
func (q *tcxQuerier) programs(ifindex int, direction manager.TrafficType) ([]tc.TCXProgram, error) {
	if q.unsupported {
		return nil, nil
	}

	programs, err := q.query(ifindex, direction)
	switch {
	// kernels without TCX (below 6.6) reject the attach type, and older ones the query itself
	case errors.Is(err, unix.EINVAL), errors.Is(err, ebpf.ErrNotSupported):
		seclog.Debugf("TCX isn't supported, TCX programs won't be listed: %v", err)
		q.unsupported = true
		return nil, nil
	case errors.Is(err, unix.ENODEV):
		return nil, nil
	}
	return programs, err
}

// hostHook is a hook of a host interface, with the programs attached to it
type hostHook struct {
	device    string
	ifindex   int
	direction manager.TrafficType
	programs  tc.HookPrograms
}

// clsactParent returns the clsact parent of the filters of a hook
func clsactParent(direction manager.TrafficType) uint32 {
	if direction == manager.Ingress {
		return netlink.HANDLE_MIN_INGRESS
	}
	return netlink.HANDLE_MIN_EGRESS
}

// hasPeerInOtherNetNS returns true for the ends of pairs of the provided kinds, such as veth or netkit, plugged into
// another network namespace: netlink only reports the namespace of the peer when it isn't the namespace of the device
func hasPeerInOtherNetNS(device netlink.Link, pairKinds ...string) bool {
	return slices.Contains(pairKinds, device.Type()) && device.Attrs().NetNsID >= 0
}

func bpfFilters(filters []netlink.Filter) []tc.Filter {
	var out []tc.Filter
	for _, filter := range filters {
		bpfFilter, ok := filter.(*netlink.BpfFilter)
		if !ok {
			continue
		}

		var chain uint32
		if bpfFilter.Chain != nil {
			chain = *bpfFilter.Chain
		}

		out = append(out, tc.Filter{
			Name:         bpfFilter.Name,
			ProgramID:    uint32(bpfFilter.Id),
			Priority:     bpfFilter.Priority,
			Chain:        chain,
			DirectAction: bpfFilter.DirectAction,
		})
	}
	return out
}

// DetectBlindHooks returns the hooks of the host interfaces where a foreign program runs before the classifiers
func (nr *Resolver) DetectBlindHooks() []BlindHook {
	if !nr.config.NetworkEnabled {
		return nil
	}

	mode := nr.tcResolver.AttachMode()
	hooks, own, err := nr.listHostHooks()
	if err != nil {
		seclog.Debugf("couldn't list the hooks of the host interfaces: %v", err)
		return nil
	}

	isOwn := func(programID uint32) bool {
		_, ok := own[programID]
		return ok
	}
	filterPriority := nr.tcResolver.FilterPriority()

	var blind []BlindHook
	for _, hook := range hooks {
		if prog, found := tc.FindProgramAhead(mode, hook.programs, filterPriority, isOwn); found {
			blind = append(blind, BlindHook{Device: hook.device, Direction: hook.direction, Program: prog})
		}
	}
	return blind
}

// warnAboutBlindHooks logs the hooks of the host interfaces where a foreign program runs before the classifiers
func (nr *Resolver) warnAboutBlindHooks() {
	if warning := blindHooksWarning(nr.DetectBlindHooks()); warning != "" {
		seclog.Warnf("%s", warning)
	}
}

// listHostHooks returns the hooks of the host interfaces, along with the IDs of the classifiers attached in
// the host network namespace
func (nr *Resolver) listHostHooks() ([]hostHook, map[uint32]struct{}, error) {
	hostNetNSID, err := utils.NewNSPathFromPid(1, utils.NetNsType).GetNSID()
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't resolve the host network namespace: %w", err)
	}

	hostNetNS := nr.ResolveNetworkNamespace(hostNetNSID)
	if hostNetNS == nil {
		return nil, nil, fmt.Errorf("the host network namespace %d isn't tracked", hostNetNSID)
	}

	handle, err := hostNetNS.GetNamespaceHandleDup()
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't get a handle on the host network namespace %d: %w", hostNetNSID, err)
	}
	defer func() {
		if cerr := handle.Close(); cerr != nil {
			seclog.Warnf("could not close file [%s]: %s", handle.Name(), cerr)
		}
	}()

	ntl, err := nr.manager.GetNetlinkSocket(uint64(handle.Fd()), hostNetNSID)
	if err != nil {
		nr.countError(errorClassNetlinkSocket)
		return nil, nil, err
	}

	devices, err := listLinks(ntl.Sock)
	if err != nil {
		nr.countError(errorClassLinkList)
		return nil, nil, err
	}

	directions := nr.tcResolver.Directions()
	var hooks []hostHook
	for _, device := range devices {
		attrs := device.Attrs()
		if attrs == nil || hasPeerInOtherNetNS(device, "veth", "netkit") {
			continue
		}

		for _, direction := range directions {
			filters, err := ntl.Sock.FilterList(device, clsactParent(direction))
			if err != nil {
				if !errors.Is(err, unix.ENODEV) {
					nr.countError(errorClassFilterList)
					seclog.Debugf("couldn't list the tc filters of %s %s: %v", attrs.Name, direction, err)
				}
				continue
			}

			hooks = append(hooks, hostHook{
				device:    attrs.Name,
				ifindex:   attrs.Index,
				direction: direction,
				programs:  tc.HookPrograms{Filters: bpfFilters(filters)},
			})
		}
	}

	own := nr.tcResolver.ProgramIDs(hostNetNSID)
	nr.listTCXPrograms(handle, hooks, own)

	return hooks, own, nil
}

// listTCXPrograms adds their TCX programs to the provided hooks. The kernel resolves the interface index of a
// TCX query in the network namespace of the caller, so the queries run from the host network namespace.
func (nr *Resolver) listTCXPrograms(hostNetNS *os.File, hooks []hostHook, own map[uint32]struct{}) {
	if len(hooks) == 0 {
		return
	}

	tcx := tcxQuerier{query: tc.QueryTCXPrograms}
	programs := make([][]tc.TCXProgram, len(hooks))
	err := tc.RunInNetNS(hostNetNS, func() error {
		for i, hook := range hooks {
			hookPrograms, err := tcx.programs(hook.ifindex, hook.direction)
			if err != nil {
				nr.countError(errorClassTCXQuery)
				seclog.Debugf("couldn't list the TCX programs of %s %s: %v", hook.device, hook.direction, err)
				continue
			}
			programs[i] = hookPrograms
		}
		return nil
	})
	if err != nil {
		nr.countError(errorClassTCXQuery)
		seclog.Debugf("couldn't list TCX programs from the host network namespace: %v", err)
		return
	}

	for i := range hooks {
		for _, prog := range programs[i] {
			if _, ok := own[prog.ID]; !ok {
				name, err := programName(prog.ID)
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				if err != nil {
					nr.countError(errorClassTCXQuery)
					seclog.Debugf("couldn't get the name of the TCX program %d of %s %s: %v", prog.ID, hooks[i].device, hooks[i].direction, err)
				}
				prog.Name = name
			}
			hooks[i].programs.TCX = append(hooks[i].programs.TCX, prog)
		}
	}
}

func programName(id uint32) (string, error) {
	prog, err := ebpf.NewProgramFromID(ebpf.ProgramID(id))
	if err != nil {
		return "", err
	}
	defer prog.Close()

	info, err := prog.Info()
	if err != nil {
		return "", err
	}
	return info.Name, nil
}

// blindHooksWarning returns a warning listing the provided hooks, or an empty string if there are none
func blindHooksWarning(hooks []BlindHook) string {
	if len(hooks) == 0 {
		return ""
	}

	listedCount := min(len(hooks), maxListedBlindHooks)
	listed := make([]string, 0, listedCount+1)
	for _, hook := range hooks[:listedCount] {
		listed = append(listed, fmt.Sprintf("%s %s (%s, %s)", hook.Device, hook.Direction, hook.Program.Name, hook.Program.Reason))
	}
	if len(hooks) > listedCount {
		listed = append(listed, fmt.Sprintf("and %d more", len(hooks)-listedCount))
	}

	hookWord := "hooks"
	if len(hooks) == 1 {
		hookWord = "hook"
	}
	return fmt.Sprintf("network events may be missed on %d host interface %s, where foreign programs run before the Workload Protection classifiers: %s", len(hooks), hookWord, strings.Join(listed, ", "))
}
