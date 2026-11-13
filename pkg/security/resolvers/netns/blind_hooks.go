// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	// blindHooksScanPeriod is the period at which the hooks of the host interfaces are checked
	blindHooksScanPeriod = 5 * time.Minute
)

// maxListedBlindHooks caps the number of hooks listed by the blind hooks warning
const maxListedBlindHooks = 10

// BlindHook is a hook of a host interface where a foreign program runs before the classifiers
type BlindHook struct {
	Device    string
	Direction manager.TrafficType
	Program   tc.ProgramAhead
}

type blindHookKey struct {
	direction manager.TrafficType
	reason    tc.BlindReason
}

// blindHooks holds the state of the detection of blind hooks
type blindHooks struct {
	// scanLock serializes the scans and guards the fields that only the scans use
	scanLock    sync.Mutex
	hostNetNSID uint32
	tcx         tcxQuerier
	warned      bool

	lock  sync.RWMutex
	mode  manager.TCAttachMode
	hooks []BlindHook

	scanRequests chan struct{}
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

// DetectBlindHooks looks for foreign programs that run before the classifiers on the hooks of the host
// interfaces, and updates the blind hooks report with what it finds
func (nr *Resolver) DetectBlindHooks() {
	if !nr.config.NetworkEnabled {
		return
	}

	nr.blindHooks.scanLock.Lock()
	defer nr.blindHooks.scanLock.Unlock()

	mode := nr.tcResolver.AttachMode()
	hooks, own, err := nr.listHostHooks()
	if err != nil {
		seclog.Debugf("couldn't list the hooks of the host interfaces: %v", err)
		return
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

	nr.publishBlindHooks(mode, blind)
}

// listHostHooks returns the hooks of the host interfaces, along with the IDs of the classifiers attached in
// the host network namespace
func (nr *Resolver) listHostHooks() ([]hostHook, map[uint32]struct{}, error) {
	if nr.blindHooks.hostNetNSID == 0 {
		nsID, err := utils.NewNSPathFromPid(1, utils.NetNsType).GetNSID()
		if err != nil {
			return nil, nil, fmt.Errorf("couldn't resolve the host network namespace: %w", err)
		}
		nr.blindHooks.hostNetNSID = nsID
	}
	hostNetNSID := nr.blindHooks.hostNetNSID

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
	tcx := &nr.blindHooks.tcx
	if tcx.unsupported || len(hooks) == 0 {
		return
	}

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

func (nr *Resolver) publishBlindHooks(mode manager.TCAttachMode, hooks []BlindHook) {
	nr.blindHooks.lock.Lock()
	nr.blindHooks.mode = mode
	nr.blindHooks.hooks = hooks
	nr.blindHooks.lock.Unlock()

	// every series is set, so that the hooks that aren't blind anymore go back to 0
	counts := countBlindHooks(hooks)
	for _, direction := range []manager.TrafficType{manager.Ingress, manager.Egress} {
		for _, reason := range tc.BlindReasons {
			count := counts[blindHookKey{direction: direction, reason: reason}]
			blindHooksTelemetry.Set(float64(count), withPlatformTags(mode.String(), string(reason), direction.String())...)
		}
	}

	if warning := blindHooksWarning(hooks); warning != "" && !nr.blindHooks.warned {
		seclog.Warnf("%s", warning)
		nr.blindHooks.warned = true
	}
}

func countBlindHooks(hooks []BlindHook) map[blindHookKey]int {
	counts := make(map[blindHookKey]int)
	for _, hook := range hooks {
		counts[blindHookKey{direction: hook.Direction, reason: hook.Program.Reason}]++
	}
	return counts
}

func (nr *Resolver) sendBlindHookStats() {
	nr.blindHooks.lock.RLock()
	mode, hooks := nr.blindHooks.mode, nr.blindHooks.hooks
	nr.blindHooks.lock.RUnlock()

	for key, count := range countBlindHooks(hooks) {
		tags := []string{"direction:" + key.direction.String(), "reason:" + string(key.reason), "attach_mode:" + mode.String()}
		_ = nr.client.Gauge(metrics.MetricTCBlindHooks, float64(count), tags, 1.0)
	}
}

// BlindHooks returns the blind hooks found by the last scan
func (nr *Resolver) BlindHooks() []BlindHook {
	nr.blindHooks.lock.RLock()
	defer nr.blindHooks.lock.RUnlock()

	return slices.Clone(nr.blindHooks.hooks)
}

// BlindHooksWarning returns a warning listing the hooks of host interfaces where a program known to end the tc
// chain runs before the classifiers, or an empty string if there are none
func (nr *Resolver) BlindHooksWarning() string {
	nr.blindHooks.lock.RLock()
	defer nr.blindHooks.lock.RUnlock()

	return blindHooksWarning(nr.blindHooks.hooks)
}

func blindHooksWarning(hooks []BlindHook) string {
	var count int
	var listed []string
	for _, hook := range hooks {
		if !hook.Program.Reason.EndsChain() {
			continue
		}

		count++
		if len(listed) < maxListedBlindHooks {
			listed = append(listed, fmt.Sprintf("%s %s (%s)", hook.Device, hook.Direction, hook.Program.Name))
		}
	}

	if count == 0 {
		return ""
	}
	if count > len(listed) {
		listed = append(listed, fmt.Sprintf("and %d more", count-len(listed)))
	}

	hookWord := "hooks"
	if count == 1 {
		hookWord = "hook"
	}
	return fmt.Sprintf("network events are missed on %d host interface %s, where Cilium programs end the tc chain before the Workload Protection classifiers: %s", count, hookWord, strings.Join(listed, ", "))
}

// requestBlindHooksScan asks for a scan of the hooks of the host interfaces, without waiting for it
func (nr *Resolver) requestBlindHooksScan() {
	select {
	case nr.blindHooks.scanRequests <- struct{}{}:
	default:
	}
}

func (nr *Resolver) detectBlindHooksPeriodically(ctx context.Context) {
	ticker := time.NewTicker(blindHooksScanPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-nr.blindHooks.scanRequests:
		case <-ticker.C:
		}
		nr.DetectBlindHooks()
	}
}
