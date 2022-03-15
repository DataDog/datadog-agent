// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/gopsutil/process"
	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	// ErrNoNetworkNamespaceHandle is used to indicate that we haven't resolved a handle for the requested network
	// namespace yet.
	ErrNoNetworkNamespaceHandle = fmt.Errorf("no network namespace handle")

	// lonelyTimeout is the timeout past which a lonely network namespace is expired
	lonelyTimeout = 30 * time.Second
	// flushNamespacesPeriod is the period at which the resolver checks if a namespace should be flushed
	flushNamespacesPeriod = 30 * time.Second
)

const (
	netStructHasProcINum uint64 = 0
	netStructHasNS       uint64 = 1
)

func getNetStructType(kv *kernel.Version) uint64 {
	if kv.IsRH7Kernel() {
		return netStructHasProcINum
	}
	return netStructHasNS
}

// NetworkNamespace is used to hold a handle to a network namespace
type NetworkNamespace struct {
	sync.RWMutex

	// nsID is the network namespace ID of the current network namespace.
	nsID uint32

	// handle is the network namespace handle that points to the current network namespace. This handle is used by the
	// manager to create a netlink socket inside the network namespace in which lives the network interfaces we want to
	// monitor.
	handle *os.File

	// networkDevicesQueue is the list of devices that we have detected at runtime, but to which we haven't been able
	// to attach a probe yet. These devices will be dequeued once we capture a network namespace handle, or when the
	// current network namespace expires (see the timeout below).
	networkDevicesQueue []model.NetDevice

	// lonelyTimeout indicates that we have been able to capture a handle for this namespace, but we are yet to see an
	// interface in this namespace. The handle of this namespace will be released if we don't see an interface by the
	// time this timeout expires.
	lonelyTimeout time.Time
}

// NewNetworkNamespace returns a new NetworkNamespace instance
func NewNetworkNamespace(nsID uint32) *NetworkNamespace {
	return &NetworkNamespace{
		nsID: nsID,
	}
}

// NewNetworkNamespaceWithPath returns a new NetworkNamespace instance from a path.
func NewNetworkNamespaceWithPath(nsID uint32, nsPath string) (*NetworkNamespace, error) {
	netns := NewNetworkNamespace(nsID)
	if err := netns.openHandle(nsPath); err != nil {
		return nil, err
	}
	return netns, nil
}

// openHandle tries to create a network namespace handle with the provided thread ID
func (nn *NetworkNamespace) openHandle(nsPath string) error {
	nn.Lock()
	defer nn.Unlock()

	// check that the handle matches the expected netns ID
	threadNetnsID, err := utils.GetProcessNetworkNamespace(nsPath)
	if err != nil {
		return err
	}
	if threadNetnsID != nn.nsID {
		// The reason why this can happen is that a process can hold a socket in a different network namespace. This is
		// the case for the Docker Embedded DNS server: a socket is created in the container namespace, but the thead
		// holding the socket jumps back to the host network namespace. Unfortunately this code is racy: ideally we'd
		// like to lock the network namespace of the thread in place until we fetch both the netns ID and the handle,
		// but afaik that's not possible (without freezing the process or its cgroup ...).
		return fmt.Errorf("the provided doesn't match the expected netns ID: got %d, expected %d", threadNetnsID, nn.nsID)
	}

	handle, err := os.Open(nsPath)
	if err != nil {
		return err
	}
	nn.handle = handle
	return nil
}

// GetNamespaceHandleDup duplicates the network namespace handle and returns it. WARNING: it is up to the caller of this
// function to close the duplicated network namespace handle. Failing to close a network namespace handle may lead to
// leaking the network namespace.
func (nn *NetworkNamespace) GetNamespaceHandleDup() (*os.File, error) {
	nn.Lock()
	defer nn.Unlock()

	return nn.getNamespaceHandleDup()
}

// getNamespaceHandleDup is an internal function (see GetNamespaceHandleDup)
func (nn *NetworkNamespace) getNamespaceHandleDup() (*os.File, error) {
	if nn.handle == nil {
		return nil, ErrNoNetworkNamespaceHandle
	}

	// duplicate the file descriptor to avoid race conditions with the resync
	dup, err := unix.Dup(int(nn.handle.Fd()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(dup), nn.handle.Name()), nil
}

// dequeueNetworkDevices dequeues the devices in the current network devices queue.
func (nn *NetworkNamespace) dequeueNetworkDevices(probe *Probe) {
	nn.Lock()
	defer nn.Unlock()

	if len(nn.networkDevicesQueue) == 0 {
		return
	}

	// make a copy of the network namespace handle to make sure we don't poison our internal cache if the eBPF library
	// modifies the handle.
	handle, err := nn.getNamespaceHandleDup()
	if err != nil {
		return
	}
	defer handle.Close()

	for _, queuedDevice := range nn.networkDevicesQueue {
		_ = probe.setupNewTCClassifierWithNetNSHandle(queuedDevice, handle)
	}
	nn.flushNetworkDevicesQueue()
}

func (nn *NetworkNamespace) queueNetworkDevice(device model.NetDevice) {
	nn.Lock()
	defer nn.Unlock()

	nn.networkDevicesQueue = append(nn.networkDevicesQueue, device)
}

func (nn *NetworkNamespace) flushNetworkDevicesQueue() {
	// flush the network devices queue
	nn.networkDevicesQueue = nil
}

func (nn *NetworkNamespace) close() error {
	return nn.handle.Close()
}

func (nn *NetworkNamespace) hasValidHandle() bool {
	return nn.handle != nil
}

// NamespaceResolver is used to store namespace handles
type NamespaceResolver struct {
	sync.RWMutex
	state  int64
	probe  *Probe
	client *statsd.Client

	networkNamespaces *simplelru.LRU
}

// NewNamespaceResolver returns a new instance of NamespaceResolver
func NewNamespaceResolver(probe *Probe) (*NamespaceResolver, error) {
	nr := &NamespaceResolver{
		probe:  probe,
		client: probe.statsdClient,
	}

	lru, err := simplelru.NewLRU(1024, func(key interface{}, value interface{}) {
		nr.flushNetworkNamespace(value.(*NetworkNamespace))
	})
	if err != nil {
		return nil, err
	}

	nr.networkNamespaces = lru
	return nr, nil
}

// SetState sets state of the namespace resolver
func (nr *NamespaceResolver) SetState(state int64) {
	atomic.StoreInt64(&nr.state, state)
}

// GetState returns the state of the namespace resolver
func (nr *NamespaceResolver) GetState() int64 {
	return atomic.LoadInt64(&nr.state)
}

// SaveNetworkNamespaceHandle inserts the provided process network namespace in the list of tracked network. Returns
// true if a new entry was added.
func (nr *NamespaceResolver) SaveNetworkNamespaceHandle(nsID uint32, nsPath string) (*NetworkNamespace, bool) {
	if !nr.probe.config.NetworkEnabled || nsID == 0 || nsPath == "" {
		return nil, false
	}

	nr.Lock()
	defer nr.Unlock()

	var netns *NetworkNamespace
	value, found := nr.networkNamespaces.Get(nsID)
	if !found {
		var err error
		netns, err = NewNetworkNamespaceWithPath(nsID, nsPath)
		if err != nil {
			// we'll get this namespace another time, ignore
			return nil, false
		}
		nr.networkNamespaces.Add(nsID, netns)
	} else {
		netns = value.(*NetworkNamespace)
		if netns.hasValidHandle() {
			// we already have a handle for this network namespace, ignore
			return netns, false
		}
		if err := netns.openHandle(nsPath); err != nil {
			// we'll get this namespace another time, ignore
			return nil, false
		}
	}

	// dequeue devices
	netns.dequeueNetworkDevices(nr.probe)

	// if the snapshot process is still going on, we need to snapshot the namespace now, otherwise we'll miss it
	if nr.GetState() == snapshotting {
		_ = nr.snapshotNetworkDevices(netns)
	}
	return netns, true
}

// ResolveNetworkNamespace returns a file descriptor to the network namespace. WARNING: it is up to the caller to
// close this file descriptor when it is done using it. Do not forget to close this file descriptor, otherwise we might
// exhaust the host IPs by keeping all network namespaces alive.
func (nr *NamespaceResolver) ResolveNetworkNamespace(nsID uint32) *NetworkNamespace {
	if !nr.probe.config.NetworkEnabled || nsID == 0 {
		return nil
	}

	nr.RLock()
	defer nr.RUnlock()

	if ns, found := nr.networkNamespaces.Get(nsID); found {
		return ns.(*NetworkNamespace)
	}

	return nil
}

// snapshotNetworkDevicesWithHandle snapshots the network devices of the provided network namespace. This function returns the
// number of non-loopback network devices to which egress and ingress TC classifiers were successfully attached.
func (nr *NamespaceResolver) snapshotNetworkDevices(netns *NetworkNamespace) int {
	handle, err := netns.getNamespaceHandleDup()
	if err != nil {
		return 0
	}
	defer handle.Close()

	ntl, err := nr.probe.manager.GetNetlinkSocket(uint64(handle.Fd()), netns.nsID)
	if err != nil {
		seclog.Errorf("couldn't open netlink socket: %s", err)
		return 0
	}

	links, err := ntl.Sock.LinkList()
	if err != nil {
		seclog.Errorf("couldn't list network interfaces in namespace %d: %s", netns.nsID, err)
		return 0
	}

	var attachedDeviceCountNoLazyDeletion int
	for _, link := range links {
		attrs := link.Attrs()
		if attrs == nil {
			continue
		}

		device := model.NetDevice{
			Name:    attrs.Name,
			IfIndex: uint32(attrs.Index),
			NetNS:   netns.nsID,
		}

		if err = nr.probe.setupNewTCClassifierWithNetNSHandle(device, handle); err == nil {
			// ignore interfaces that are lazily deleted
			if !nr.IsLazyDeletionInterface(device.Name) && attrs.HardwareAddr.String() != "" {
				attachedDeviceCountNoLazyDeletion++
			}
		}
	}
	return attachedDeviceCountNoLazyDeletion
}

// IsLazyDeletionInterface returns true if an interface name is in the list of interfaces that aren't explicitly deleted by the
// container runtime when a container is deleted.
func (nr *NamespaceResolver) IsLazyDeletionInterface(name string) bool {
	for _, lazyPrefix := range nr.probe.config.NetworkLazyInterfacePrefixes {
		if strings.HasPrefix(name, lazyPrefix) {
			return true
		}
	}
	return false
}

// SyncCache snapshots /proc for the provided pid. This method returns true if it updated the namespace cache.
func (nr *NamespaceResolver) SyncCache(proc *process.Process) bool {
	if !nr.probe.config.NetworkEnabled {
		return false
	}

	nsPath := utils.NetNSPathFromPid(uint32(proc.Pid))
	nsID, err := utils.GetProcessNetworkNamespace(nsPath)
	if err != nil {
		return false
	}

	_, isNewEntry := nr.SaveNetworkNamespaceHandle(nsID, nsPath)
	if !isNewEntry {
		return false
	}
	return true
}

// QueueNetworkDevice adds the input device to the map of queued network devices. Once a handle for the network namespace
// of the device is resolved, a new TC classifier will automatically be added to the device. The queue is cleaned up
// periodically if a namespace do not own any process.
func (nr *NamespaceResolver) QueueNetworkDevice(device model.NetDevice) {
	if !nr.probe.config.NetworkEnabled {
		return
	}

	if device.NetNS == 0 {
		return
	}

	nr.Lock()
	defer nr.Unlock()

	var netns *NetworkNamespace
	value, found := nr.networkNamespaces.Get(device.NetNS)
	if !found {
		netns = NewNetworkNamespace(device.NetNS)
		nr.networkNamespaces.Add(device.NetNS, netns)
	} else {
		netns = value.(*NetworkNamespace)
	}

	netns.queueNetworkDevice(device)
}

// Start starts the namespace flush goroutine
func (nr *NamespaceResolver) Start(ctx context.Context) error {
	if !nr.probe.config.NetworkEnabled {
		return nil
	}

	go nr.flushNamespaces(ctx)
	return nil
}

func (nr *NamespaceResolver) flushNamespaces(ctx context.Context) {
	ticker := time.NewTicker(flushNamespacesPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			probesCount := nr.probe.flushInactiveProbes()

			// There is a possible race condition if we lose all network device creations but do notice the new network
			// namespace: we will create a handle that will never be flushed by `nr.probe.flushInactiveNamespaces()`.
			// To detect this race, compute the list of namespaces that are in cache, but for which we do not have any
			// device. Defer a snapshot process for each of those namespaces, and delete them if the snapshot yields
			// no new device.
			nr.preventNetworkNamespaceDrift(probesCount)
		}
	}
}

// FlushNetworkNamespace flushes the cached entries for the provided network namespace.
func (nr *NamespaceResolver) FlushNetworkNamespace(netns *NetworkNamespace) {
	nr.Lock()
	defer nr.Unlock()

	nr.flushNetworkNamespace(netns)
}

// flushNetworkNamespace flushes the cached entries for the provided network namespace.
func (nr *NamespaceResolver) flushNetworkNamespace(netns *NetworkNamespace) {

	// if we can, make sure the manager has a valid netlink socket to this handle before removing everything
	handle, err := netns.getNamespaceHandleDup()
	if err == nil {
		defer handle.Close()
		_, _ = nr.probe.manager.GetNetlinkSocket(uint64(handle.Fd()), netns.nsID)
	}

	// close network namespace handle to release the namespace
	netns.close()

	// delete map entry
	nr.networkNamespaces.Remove(netns.nsID)

	// remove all references to this network namespace from the manager
	_ = nr.probe.manager.CleanupNetworkNamespace(netns.nsID)
}

// preventNetworkNamespaceDrift ensures that we do not keep network namespace handles indefinitely
func (nr *NamespaceResolver) preventNetworkNamespaceDrift(probesCount map[uint32]int) {
	nr.Lock()
	defer nr.Unlock()

	now := time.Now()
	timeout := now.Add(lonelyTimeout)

	// compute the list of network namespaces without any probe
	for nsID := range nr.networkNamespaces.Keys() {
		value, _ := nr.networkNamespaces.Peek(nsID)
		netns := value.(*NetworkNamespace)

		netns.Lock()
		netnsCount := probesCount[netns.nsID]

		// is this network namespace lonely ?
		if !netns.lonelyTimeout.IsZero() && netnsCount == 0 {
			// snapshot lonely namespace and delete it if it is all alone on earth
			if now.After(netns.lonelyTimeout) {
				netns.lonelyTimeout = time.Time{}
				deviceCountNoLoopbackNoDummy := nr.snapshotNetworkDevices(netns)
				if deviceCountNoLoopbackNoDummy == 0 {
					nr.flushNetworkNamespace(netns)
					netns.Unlock()
					continue
				}
			}
		} else {
			if netnsCount == 0 {
				netns.lonelyTimeout = timeout
			} else {
				netns.lonelyTimeout = time.Time{}
			}
		}

		netns.Unlock()
	}
}

// SendStats sends metrics about the current state of the namespace resolver
func (nr *NamespaceResolver) SendStats() error {
	nr.RLock()
	defer nr.RUnlock()

	networkNamespacesCount := float64(nr.networkNamespaces.Len())
	if networkNamespacesCount > 0 {
		_ = nr.client.Gauge(metrics.MetricNamespaceResolverNetNSHandle, networkNamespacesCount, []string{}, 1.0)
	}

	var queuedNetworkDevicesCount float64
	var lonelyNetworkNamespacesCount float64

	for nsID := range nr.networkNamespaces.Keys() {
		value, _ := nr.networkNamespaces.Peek(nsID)
		netns := value.(*NetworkNamespace)

		if count := len(netns.networkDevicesQueue); count > 0 {
			queuedNetworkDevicesCount += float64(count)
		}
		if !netns.lonelyTimeout.IsZero() {
			lonelyNetworkNamespacesCount++
		}
	}

	if queuedNetworkDevicesCount > 0 {
		_ = nr.client.Gauge(metrics.MetricNamespaceResolverQueuedNetworkDevice, queuedNetworkDevicesCount, []string{}, 1.0)
	}
	if lonelyNetworkNamespacesCount > 0 {
		_ = nr.client.Gauge(metrics.MetricNamespaceResolverLonelyNetworkNamespace, lonelyNetworkNamespacesCount, []string{}, 1.0)
	}
	return nil
}

func newTmpFile(prefix string) (*os.File, error) {
	f, err := os.CreateTemp("/tmp", prefix)
	if err != nil {
		return nil, err
	}

	if err = os.Chmod(f.Name(), 0400); err != nil {
		return nil, err
	}
	return f, err
}

// NetworkDeviceDump is used to dump a network namespace
type NetworkDeviceDump struct {
	IfName  string
	IfIndex int
}

// NetworkNamespaceDump is used to dump a network namespce
type NetworkNamespaceDump struct {
	NsID           uint32
	HandleFD       int
	HandlePath     string
	LonelyTimeout  time.Time
	Devices        []NetworkDeviceDump
	DevicesInQueue []NetworkDeviceDump
}

func (nr *NamespaceResolver) dump(params *api.DumpNetworkNamespaceParams) []NetworkNamespaceDump {
	nr.RLock()
	defer nr.RUnlock()

	var handle *os.File
	var ntl *manager.NetlinkSocket
	var links []netlink.Link
	var dump []NetworkNamespaceDump
	var err error

	// iterate over the list of network namespaces
	for nsID := range nr.networkNamespaces.Keys() {
		value, _ := nr.networkNamespaces.Peek(nsID)
		netns := value.(*NetworkNamespace)

		netns.Lock()

		netnsDump := NetworkNamespaceDump{
			NsID:          netns.nsID,
			HandleFD:      int(netns.handle.Fd()),
			HandlePath:    netns.handle.Name(),
			LonelyTimeout: netns.lonelyTimeout,
		}

		for _, dev := range netns.networkDevicesQueue {
			netnsDump.DevicesInQueue = append(netnsDump.DevicesInQueue, NetworkDeviceDump{
				IfName:  dev.Name,
				IfIndex: int(dev.IfIndex),
			})
		}

		if params.GetSnapshotInterfaces() {
			handle, err = netns.getNamespaceHandleDup()
			if err != nil {
				netns.Unlock()
				continue
			}

			ntl, err = nr.probe.manager.GetNetlinkSocket(uint64(handle.Fd()), netns.nsID)
			if err == nil {
				links, err = ntl.Sock.LinkList()
				if err == nil {
					for _, link := range links {
						netnsDump.Devices = append(netnsDump.Devices, NetworkDeviceDump{
							IfName:  link.Attrs().Name,
							IfIndex: link.Attrs().Index,
						})
					}
				}
			}

			handle.Close()
		}

		netns.Unlock()
		dump = append(dump, netnsDump)
	}

	return dump
}

// DumpNetworkNamespaces dumps the network namespaces held by the namespace resolver
func (nr *NamespaceResolver) DumpNetworkNamespaces(params *api.DumpNetworkNamespaceParams) *api.DumpNetworkNamespaceMessage {
	resp := &api.DumpNetworkNamespaceMessage{}
	dump := nr.dump(params)

	// create the dump file
	dumpFile, err := newTmpFile("network-namespace-dump-*.json")
	if err != nil {
		resp.Error = fmt.Sprintf("couldn't create temporary file: %v", err)
		seclog.Warnf(resp.Error)
		return resp
	}
	defer dumpFile.Close()
	resp.DumpFilename = dumpFile.Name()

	// dump to JSON file
	encoder := json.NewEncoder(dumpFile)
	if err = encoder.Encode(dump); err != nil {
		resp.Error = fmt.Sprintf("couldn't encode list of network namespace: %v", err)
		seclog.Warnf(resp.Error)
		return resp
	}

	// create graph file
	graphFile, err := newTmpFile("network-namespace-graph-*.dot")
	if err != nil {
		resp.Error = fmt.Sprintf("couldn't create temporary file: %v", err)
		seclog.Warnf(resp.Error)
		return resp
	}
	defer graphFile.Close()
	resp.GraphFilename = graphFile.Name()

	// generate dot graph
	if err = nr.generateGraph(dump, graphFile); err != nil {
		resp.Error = fmt.Sprintf("couldn't generate dot graph: %v", err)
		seclog.Warnf(resp.Error)
		return resp
	}

	return resp
}
