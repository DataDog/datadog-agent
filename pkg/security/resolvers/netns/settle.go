// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"net"
	"os"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/vishvananda/netlink"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// settlesBeforeAttach returns true for the kinds of devices that container runtimes and CNI plugins create in one
// network namespace and then move to another one. A TCX link follows a moved device, so these are only attached
// once they had time to reach their final namespace.
func settlesBeforeAttach(device netlink.Link) bool {
	switch device.Type() {
	case "netkit", "macvlan", "macvtap", "ipvlan", "ipvtap", "vlan", "veth":
		return true
	case "device":
		// physical devices and VFs report no kind, like lo, which can't move
		return device.Attrs().Flags&net.FlagLoopback == 0
	default:
		return false
	}
}

type deviceKey struct {
	netNS   uint32
	ifIndex uint32
}

type settlingDevice struct {
	device   model.NetDevice
	deadline time.Time
	// fromHostSnapshot devices were found by the periodic host snapshot rather than by an event
	fromHostSnapshot bool
}

// failedDevice identifies a device that a new name or a reused ifindex gives another try
type failedDevice struct {
	ifIndex uint32
	name    string
}

// deviceSettler holds the TCX devices waiting to be attached, and the host devices whose attach failed. Only the TC
// classifier loop uses it.
type deviceSettler struct {
	delay     time.Duration
	hostNetNS uint32

	// pending is in deadline order, since every device waits for the same delay
	pending    []settlingDevice
	pendingSet map[deviceKey]struct{}
	timer      *time.Timer

	failed map[failedDevice]struct{}
}

func newDeviceSettler(delay time.Duration) *deviceSettler {
	return &deviceSettler{
		delay:      delay,
		pendingSet: make(map[deviceKey]struct{}),
		failed:     make(map[failedDevice]struct{}),
	}
}

func (s *deviceSettler) isPending(device model.NetDevice) bool {
	_, ok := s.pendingSet[deviceKey{netNS: device.NetNS, ifIndex: device.IfIndex}]
	return ok
}

// push queues a device until now + delay, unless it's queued already
func (s *deviceSettler) push(device model.NetDevice, fromHostSnapshot bool, now time.Time) {
	key := deviceKey{netNS: device.NetNS, ifIndex: device.IfIndex}
	if _, ok := s.pendingSet[key]; ok {
		return
	}
	s.pendingSet[key] = struct{}{}
	s.pending = append(s.pending, settlingDevice{device: device, deadline: now.Add(s.delay), fromHostSnapshot: fromHostSnapshot})
}

// popDue dequeues the devices whose deadline is past
func (s *deviceSettler) popDue(now time.Time) []settlingDevice {
	var due int
	for due < len(s.pending) && !s.pending[due].deadline.After(now) {
		delete(s.pendingSet, deviceKey{netNS: s.pending[due].device.NetNS, ifIndex: s.pending[due].device.IfIndex})
		due++
	}
	devices := s.pending[:due:due]
	s.pending = s.pending[due:]
	return devices
}

// arm sets the timer to the next deadline, and returns its channel, nil when no device is pending
func (s *deviceSettler) arm(now time.Time) <-chan time.Time {
	if len(s.pending) == 0 {
		if s.timer != nil {
			s.timer.Stop()
		}
		return nil
	}

	wait := max(s.pending[0].deadline.Sub(now), 0)
	if s.timer == nil {
		s.timer = time.NewTimer(wait)
	} else {
		s.timer.Reset(wait)
	}
	return s.timer.C
}

// resolveHostNetNS returns false when the host namespace can't be resolved yet
func (s *deviceSettler) resolveHostNetNS() bool {
	if s.hostNetNS != 0 {
		return true
	}
	nsID, err := utils.NewNSPathFromPid(1, utils.NetNsType).GetNSID()
	if err != nil {
		seclog.Debugf("couldn't resolve the host network namespace: %v", err)
		return false
	}
	s.hostNetNS = nsID
	return true
}

func (s *deviceSettler) stop() {
	if s.timer != nil {
		s.timer.Stop()
	}
}

func (s *deviceSettler) hasFailed(device netlink.Link) bool {
	_, ok := s.failed[failedDevice{ifIndex: uint32(device.Attrs().Index), name: device.Attrs().Name}]
	return ok
}

func (s *deviceSettler) addFailed(device model.NetDevice) {
	if device.NetNS == s.hostNetNS {
		s.failed[failedDevice{ifIndex: device.IfIndex, name: device.Name}] = struct{}{}
	}
}

// pruneFailed forgets the failed devices that aren't in the provided list anymore
func (s *deviceSettler) pruneFailed(devices []netlink.Link) {
	present := make(map[failedDevice]struct{}, len(s.failed))
	for _, device := range devices {
		key := failedDevice{ifIndex: uint32(device.Attrs().Index), name: device.Attrs().Name}
		if _, ok := s.failed[key]; ok {
			present[key] = struct{}{}
		}
	}
	s.failed = present
}

// handleRegisteredDevice attaches a device that just registered, once it settled if its kind may move
func (nr *Resolver) handleRegisteredDevice(device model.NetDevice, now time.Time) error {
	netns := nr.ResolveNetworkNamespace(device.NetNS)
	if netns == nil {
		return nr.setupNewTCClassifier(device)
	}
	handle, err := netns.GetNamespaceHandleDup()
	if err != nil || handle == nil {
		return nr.setupNewTCClassifier(device)
	}
	defer closeHandle(handle)

	link, err := nr.resolveLink(netns.nsID, handle, device.IfIndex)
	if err != nil {
		return err
	}
	if settlesBeforeAttach(link) {
		nr.settler.push(device, false, now)
		return nil
	}
	if err = nr.tcResolver.SetupNewTCClassifierWithNetNSHandle(device, handle, nr.manager); err != nil {
		nr.settler.addFailed(device)
	}
	return err
}

// attachSettledDevices attaches the devices that are still in their namespace once settled
func (nr *Resolver) attachSettledDevices(now time.Time) {
	for _, settling := range nr.settler.popDue(now) {
		nr.attachSettledDevice(settling)
	}
}

func (nr *Resolver) attachSettledDevice(settling settlingDevice) {
	device := settling.device

	netns := nr.ResolveNetworkNamespace(device.NetNS)
	if netns == nil {
		return
	}
	handle, err := netns.GetNamespaceHandleDup()
	if err != nil {
		return
	}
	defer closeHandle(handle)

	// by ifindex only: devices get renamed in place
	link, err := nr.resolveLink(netns.nsID, handle, device.IfIndex)
	if err != nil {
		nr.reportTCClassifierError(err, device)
		return
	}
	if settling.fromHostSnapshot && hasPeerInOtherNetNS(link, "veth") {
		return
	}
	device.Name = link.Attrs().Name

	if err = nr.tcResolver.SetupNewTCClassifierWithNetNSHandle(device, handle, nr.manager); err != nil {
		nr.settler.addFailed(device)
		nr.reportTCClassifierError(err, device)
	}
}

// handleMovedDevice detaches what's left of a device in its previous namespace, then attaches it in its new one
func (nr *Resolver) handleMovedDevice(device model.NetDevice) error {
	countStaleLinks(nr.tcResolver.DetachMovedDevice(device.IfIndex, device.NetNS, nr.manager))
	return nr.setupNewTCClassifier(device)
}

// requestHostSnapshot asks the TC classifier loop to look for new host devices
func (nr *Resolver) requestHostSnapshot() {
	if nr.settler == nil || nr.ctx.Err() != nil {
		return
	}
	select {
	case nr.hostSnapshotRequests <- struct{}{}:
	default:
	}
}

// snapshotHostDevices attaches the new devices of the host namespace that no event reported, such as the veth pairs
// created and kept in the host namespace. Veth ends plugged into another namespace are left out, as in legacy mode.
func (nr *Resolver) snapshotHostDevices(now time.Time) {
	if !nr.settler.resolveHostNetNS() {
		return
	}

	netns := nr.ResolveNetworkNamespace(nr.settler.hostNetNS)
	if netns == nil {
		return
	}
	handle, err := netns.GetNamespaceHandleDup()
	if err != nil {
		return
	}
	defer closeHandle(handle)

	ntl, err := nr.manager.GetNetlinkSocket(uint64(handle.Fd()), netns.nsID)
	if err != nil {
		nr.countError(errorClassNetlinkSocket)
		seclog.Debugf("couldn't open netlink socket: %s", err)
		return
	}
	links, err := listLinks(ntl.Sock)
	if err != nil {
		nr.countError(errorClassLinkList)
		seclog.Debugf("couldn't list network interfaces in namespace %d: %s", netns.nsID, err)
		return
	}
	nr.settler.pruneFailed(links)

	attached := nr.tcResolver.AttachedIfIndexes(netns.nsID)
	for _, link := range links {
		attrs := link.Attrs()
		if attrs == nil {
			continue
		}
		device := model.NetDevice{Name: attrs.Name, IfIndex: uint32(attrs.Index), NetNS: netns.nsID}

		// unlike the blind hooks scan, netkit primaries are attached: legacy mode attaches them on registration
		if _, ok := attached[device.IfIndex]; ok || hasPeerInOtherNetNS(link, "veth") || nr.settler.isPending(device) || nr.settler.hasFailed(link) {
			continue
		}
		if settlesBeforeAttach(link) {
			nr.settler.push(device, true, now)
			continue
		}
		if err = nr.tcResolver.SetupNewTCClassifierWithNetNSHandle(device, handle, nr.manager); err != nil {
			nr.settler.addFailed(device)
			nr.reportTCClassifierError(err, device)
		}
	}
}

// resolveLink looks a device up by ifindex in its namespace, with the netlink socket of the eBPF manager
func (nr *Resolver) resolveLink(nsID uint32, handle *os.File, ifIndex uint32) (netlink.Link, error) {
	ntl, err := nr.manager.GetNetlinkSocket(uint64(handle.Fd()), nsID)
	if err != nil {
		return nil, err
	}
	return ntl.Sock.LinkByIndex(int(ifIndex))
}

func closeHandle(handle *os.File) {
	if cerr := handle.Close(); cerr != nil {
		seclog.Warnf("could not close file [%s]: %s", handle.Name(), cerr)
	}
}

// usesTCX returns true when the classifiers are attached with TCX links, which follow moved devices
func (nr *Resolver) usesTCX() bool {
	// some probe tests build the resolver without a TC resolver
	return nr.tcResolver != nil && nr.tcResolver.AttachMode() == manager.TCAttachModeTCX
}
