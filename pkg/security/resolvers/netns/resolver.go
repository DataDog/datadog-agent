// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package netns holds netns related files
package netns

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/vishvananda/netlink"
)

var (
	// lonelyNamespaceTimeout is the timeout past which a lonely network namespace is expired
	lonelyNamespaceTimeout = 30 * time.Second
	// flushNamespacesPeriod is the period at which the resolver checks if a namespace should be flushed
	flushNamespacesPeriod = 30 * time.Second
)

// Resolver is used to store namespace handles
type Resolver struct {
	sync.RWMutex
	tcResolver *tc.Resolver
	client     statsd.ClientInterface
	config     *config.Config
	manager    *manager.Manager

	networkNamespaces *simplelru.LRU[uint32, *NetworkNamespace]
	tcRequests        chan TcClassifierRequest
	tcRequestsMu      sync.Mutex
	tcRequestsActive  map[tcDeviceKey]bool
	ctx               context.Context
	wg                sync.WaitGroup
}

// NewResolver returns a new instance of Resolver
func NewResolver(config *config.Config, manager *manager.Manager, statsdClient statsd.ClientInterface, tcResolver *tc.Resolver) (*Resolver, error) {
	nr := &Resolver{
		client:           statsdClient,
		config:           config,
		manager:          manager,
		tcResolver:       tcResolver,
		tcRequests:       make(chan TcClassifierRequest, 16),
		tcRequestsActive: make(map[tcDeviceKey]bool),
		ctx:              context.Background(),
	}

	lru, err := simplelru.NewLRU(1024, func(_ uint32, value *NetworkNamespace) {
		nr.flushNetworkNamespace(value)
		tcResolver.FlushNetworkNamespaceID(value.nsID, manager)
	})
	if err != nil {
		return nil, err
	}

	nr.networkNamespaces = lru
	return nr, nil
}

// SaveNetworkNamespaceHandle inserts the provided process network namespace in the list of tracked network. Returns
// true if a new entry was added.
func (nr *Resolver) SaveNetworkNamespaceHandle(nsID uint32, nsPath *utils.NSPath) (*NetworkNamespace, bool) {
	if !nr.config.NetworkEnabled || nsID == 0 || nsPath == nil {
		return nil, false
	}

	nr.Lock()
	netns, isNew := nr.insertNetworkNamespaceHandleLazy(nsID, func() *utils.NSPath {
		return nsPath
	})
	nr.Unlock()

	if isNew && netns != nil {
		netns.dequeueNetworkDevices(nr.tcResolver, nr.manager)
		nr.snapshotNetworkDevices(netns)
	}

	return netns, isNew
}

// SaveNetworkNamespaceHandleLazy inserts the provided process network namespace in the list of tracked network. Returns
// true if a new entry was added.
func (nr *Resolver) SaveNetworkNamespaceHandleLazy(nsID uint32, nsPathFunc func() *utils.NSPath) (*NetworkNamespace, bool) {
	if !nr.config.NetworkEnabled || nsID == 0 || nsPathFunc == nil {
		return nil, false
	}

	nr.Lock()
	netns, isNew := nr.insertNetworkNamespaceHandleLazy(nsID, nsPathFunc)
	nr.Unlock()

	if isNew && netns != nil {
		netns.dequeueNetworkDevices(nr.tcResolver, nr.manager)
		nr.snapshotNetworkDevices(netns)
	}

	return netns, isNew
}

// insertNetworkNamespaceHandleLazy inserts/updates the namespace in the LRU cache.
func (nr *Resolver) insertNetworkNamespaceHandleLazy(nsID uint32, nsPathFunc func() *utils.NSPath) (*NetworkNamespace, bool) {
	if !nr.config.NetworkEnabled || nsID == 0 || nsPathFunc == nil {
		return nil, false
	}

	netns, found := nr.networkNamespaces.Get(nsID)
	if !found {
		nsPath := nsPathFunc()
		if nsPath == nil {
			return nil, false
		}

		var err error
		netns, err = NewNetworkNamespaceWithPath(nsID, nsPath)
		if err != nil {
			// we'll get this namespace another time, ignore
			return nil, false
		}
		nr.networkNamespaces.Add(nsID, netns)
	} else {
		if netns.hasValidHandle() {
			// we already have a handle for this network namespace, ignore
			return netns, false
		}

		nsPath := nsPathFunc()
		if nsPath == nil {
			return nil, false
		}

		if err := netns.openHandle(nsPath); err != nil {
			// we'll get this namespace another time, ignore
			return nil, false
		}
	}

	return netns, true
}

// ResolveNetworkNamespace returns a file descriptor to the network namespace. WARNING: it is up to the caller to
// close this file descriptor when it is done using it. Do not forget to close this file descriptor, otherwise we might
// exhaust the host IPs by keeping all network namespaces alive.
func (nr *Resolver) ResolveNetworkNamespace(nsID uint32) *NetworkNamespace {
	if !nr.config.NetworkEnabled || nsID == 0 {
		return nil
	}

	nr.RLock()
	defer nr.RUnlock()

	if ns, found := nr.networkNamespaces.Peek(nsID); found {
		return ns
	}

	return nil
}

// snapshotNetworkDevices snapshots the network devices of the provided network namespace. This function returns the
// number of non-loopback network devices to which egress and ingress TC classifiers were successfully attached.
func (nr *Resolver) snapshotNetworkDevices(netns *NetworkNamespace) int {
	handle, err := netns.GetNamespaceHandleDup()
	if err != nil {
		return 0
	}
	defer func() {
		if cerr := handle.Close(); cerr != nil {
			seclog.Warnf("could not close file [%s]: %s", handle.Name(), cerr)
		}
	}()

	ntl, err := nr.manager.GetNetlinkSocket(uint64(handle.Fd()), netns.nsID)
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

		if err = nr.tcResolver.SetupNewTCClassifierWithNetNSHandle(device, handle, nr.manager); err == nil {
			// ignore interfaces that are lazily deleted
			if !nr.IsLazyDeletionInterface(device.Name) && attrs.HardwareAddr.String() != "" {
				attachedDeviceCountNoLazyDeletion++
			}
		} else {
			seclog.Errorf("error setting up new tc classifier on snapshot: %v", err)
		}
	}

	return attachedDeviceCountNoLazyDeletion
}

// IsLazyDeletionInterface returns true if an interface name is in the list of interfaces that aren't explicitly deleted by the
// container runtime when a container is deleted.
func (nr *Resolver) IsLazyDeletionInterface(name string) bool {
	for _, lazyPrefix := range nr.config.NetworkLazyInterfacePrefixes {
		if strings.HasPrefix(name, lazyPrefix) {
			return true
		}
	}
	return false
}

// SyncCache snapshots /proc to populate the namespace cache. This method returns true if it updated the namespace cache.
func (nr *Resolver) SyncCache() bool {
	if !nr.config.NetworkEnabled {
		return false
	}

	processes, err := utils.GetProcesses()
	if err != nil {
		return false
	}

	type nsEntry struct {
		netns *NetworkNamespace
		isNew bool
	}

	// Phase 1: collect (nsID, nsPath) pairs outside the lock to avoid holding
	// the write lock during per-process stat syscalls on /proc/<pid>/ns/net.
	type nsPair struct {
		nsID   uint32
		nsPath *utils.NSPath
	}
	nsSet := map[uint32]bool{}
	var nsPairs []nsPair
	for _, p := range processes {
		nsPath := utils.NewNSPathFromPid(uint32(p.Pid), utils.NetNsType)
		nsID, err := nsPath.GetNSID()
		if err != nil {
			continue
		}

		if nsSet[nsID] {
			continue
		}

		nsSet[nsID] = true
		nsPairs = append(nsPairs, nsPair{nsID: nsID, nsPath: nsPath})
	}

	// Phase 2: insert into LRU under the write lock
	var newEntries []nsEntry
	nr.Lock()
	for _, pair := range nsPairs {
		nsPath := pair.nsPath
		netns, isNew := nr.insertNetworkNamespaceHandleLazy(pair.nsID, func() *utils.NSPath {
			return nsPath
		})
		if isNew && netns != nil {
			newEntries = append(newEntries, nsEntry{netns: netns, isNew: isNew})
		}
	}
	nr.Unlock()

	for _, entry := range newEntries {
		entry.netns.dequeueNetworkDevices(nr.tcResolver, nr.manager)
		nr.snapshotNetworkDevices(entry.netns)
	}

	return len(newEntries) > 0
}

// QueueNetworkDevice adds the input device to the map of queued network devices. Once a handle for the network namespace
// of the device is resolved, a new TC classifier will automatically be added to the device. The queue is cleaned up
// periodically if a namespace does not own any process.
func (nr *Resolver) QueueNetworkDevice(device model.NetDevice) {
	if !nr.config.NetworkEnabled || device.NetNS == 0 {
		return
	}

	nr.Lock()
	defer nr.Unlock()

	netns, found := nr.networkNamespaces.Get(device.NetNS)
	if !found {
		netns = NewNetworkNamespace(device.NetNS)
		nr.networkNamespaces.Add(device.NetNS, netns)
	}

	netns.queueNetworkDevice(device)
}

// Start starts the namespace flush goroutine
func (nr *Resolver) Start(ctx context.Context) error {
	if !nr.config.NetworkEnabled {
		return nil
	}

	nr.ctx = ctx
	nr.startTcClassifierLoopGoroutine()

	nr.wg.Add(1)
	go func() {
		defer nr.wg.Done()
		nr.flushNamespaces(ctx)
	}()
	return nil
}

func (nr *Resolver) manualFlushNamespaces() {
	probesCount := nr.tcResolver.FlushInactiveProbes(nr.manager, nr.IsLazyDeletionInterface)

	// There is a possible race condition if we lose all network device creations but do notice the new network
	// namespace: we will create a handle that will never be flushed by `nr.probe.flushInactiveNamespaces()`.
	// To detect this race, compute the list of namespaces that are in cache, but for which we do not have any
	// device. Defer a snapshot process for each of those namespaces, and delete them if the snapshot yields
	// no new device.
	nr.preventNetworkNamespaceDrift(probesCount)
}

func (nr *Resolver) flushNamespaces(ctx context.Context) {
	ticker := time.NewTicker(flushNamespacesPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nr.manualFlushNamespaces()
		}
	}
}

// FlushNetworkNamespace flushes the cached entries for the provided network namespace.
// (WARNING: you probably want to use probe.FlushNetworkNamespace instead)
func (nr *Resolver) FlushNetworkNamespace(netns *NetworkNamespace) {
	nr.Lock()
	defer nr.Unlock()

	nr.flushNetworkNamespace(netns)
}

// flushNetworkNamespace flushes the cached entries for the provided network namespace.
func (nr *Resolver) flushNetworkNamespace(netns *NetworkNamespace) {
	if _, ok := nr.networkNamespaces.Peek(netns.nsID); ok {
		// remove the entry now, removing the entry will call this function again
		_ = nr.networkNamespaces.Remove(netns.nsID)
		return
	}

	// if we can, make sure the manager has a valid netlink socket to this handle before removing everything
	handle, err := netns.getNamespaceHandleDup()
	if err == nil {
		defer func() {
			if cerr := handle.Close(); cerr != nil {
				seclog.Warnf("could not close file [%s]: %s", handle.Name(), cerr)
			}
		}()
		_, _ = nr.manager.GetNetlinkSocket(uint64(handle.Fd()), netns.nsID)
	}

	// close network namespace handle to release the namespace
	if netns.hasValidHandle() {
		err = netns.close()
		if err != nil {
			seclog.Warnf("could not close file [%s]: %s", netns.handle.Name(), err)
		}
	}

	// remove all references to this network namespace from the manager
	_ = nr.manager.CleanupNetworkNamespace(netns.nsID)
}

// preventNetworkNamespaceDrift ensures that we do not keep network namespace handles indefinitely
func (nr *Resolver) preventNetworkNamespaceDrift(probesCount map[uint32]int) {
	nr.Lock()
	defer nr.Unlock()

	now := time.Now()
	timeout := now.Add(lonelyNamespaceTimeout)

	// compute the list of network namespaces without any probe
	for _, nsID := range nr.networkNamespaces.Keys() {
		netns, _ := nr.networkNamespaces.Peek(nsID)

		netns.Lock()
		netnsCount := probesCount[netns.nsID]

		shouldSnapshot := false
		// is this network namespace lonely ?
		if !netns.lonelyTimeout.IsZero() && netnsCount == 0 {
			// snapshot lonely namespace and delete it if it is all alone on earth
			if now.After(netns.lonelyTimeout) {
				netns.lonelyTimeout = time.Time{}
				shouldSnapshot = true
			}
		} else {
			if netnsCount == 0 {
				netns.lonelyTimeout = timeout
			} else {
				netns.lonelyTimeout = time.Time{}
			}
		}
		netns.Unlock()

		if shouldSnapshot {
			deviceCountNoLoopbackNoDummy := nr.snapshotNetworkDevices(netns)
			if deviceCountNoLoopbackNoDummy == 0 {
				nr.flushNetworkNamespace(netns)
				nr.tcResolver.FlushNetworkNamespaceID(netns.nsID, nr.manager)
			}
		}
	}
}

// SendStats sends metrics about the current state of the namespace resolver
func (nr *Resolver) SendStats() error {
	nr.RLock()

	networkNamespacesCount := float64(nr.networkNamespaces.Len())

	var queuedNetworkDevicesCount float64
	var lonelyNetworkNamespacesCount float64

	for _, nsID := range nr.networkNamespaces.Keys() {
		netns, _ := nr.networkNamespaces.Peek(nsID)

		netns.RLock()
		if count := len(netns.networkDevicesQueue); count > 0 {
			queuedNetworkDevicesCount += float64(count)
		}
		if !netns.lonelyTimeout.IsZero() {
			lonelyNetworkNamespacesCount++
		}
		netns.RUnlock()
	}

	nr.RUnlock()

	if networkNamespacesCount > 0 {
		_ = nr.client.Gauge(metrics.MetricNamespaceResolverNetNSHandle, networkNamespacesCount, []string{}, 1.0)
	}
	if queuedNetworkDevicesCount > 0 {
		_ = nr.client.Gauge(metrics.MetricNamespaceResolverQueuedNetworkDevice, queuedNetworkDevicesCount, []string{}, 1.0)
	}
	if lonelyNetworkNamespacesCount > 0 {
		_ = nr.client.Gauge(metrics.MetricNamespaceResolverLonelyNetworkNamespace, lonelyNetworkNamespacesCount, []string{}, 1.0)
	}
	return nil
}

// Close closes this resolver and frees all the resources
func (nr *Resolver) Close() {
	close(nr.tcRequests)

	nr.wg.Wait()

	if nr.networkNamespaces != nil {
		nr.Lock()
		nr.networkNamespaces.Purge()
		nr.Unlock()
	}
	nr.manualFlushNamespaces()
}

func newTmpFile(prefix string) (*os.File, error) {
	f, err := os.CreateTemp("/tmp", prefix)
	if err != nil {
		return nil, err
	}

	if err = os.Chmod(f.Name(), 0400); err != nil {
		f.Close()
		os.Remove(f.Name())
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

func (nr *Resolver) dump(params *api.DumpNetworkNamespaceParams) []NetworkNamespaceDump {
	nr.RLock()
	defer nr.RUnlock()

	var handle *os.File
	var ntl *manager.NetlinkSocket
	var links []netlink.Link
	var dump []NetworkNamespaceDump
	var err error

	// iterate over the list of network namespaces
	for _, nsID := range nr.networkNamespaces.Keys() {
		netns, _ := nr.networkNamespaces.Peek(nsID)
		netns.RLock()

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
				netns.RUnlock()
				continue
			}

			ntl, err = nr.manager.GetNetlinkSocket(uint64(handle.Fd()), netns.nsID)
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

		netns.RUnlock()
		dump = append(dump, netnsDump)
	}

	return dump
}

// DumpNetworkNamespaces dumps the network namespaces held by the namespace resolver
func (nr *Resolver) DumpNetworkNamespaces(params *api.DumpNetworkNamespaceParams) *api.DumpNetworkNamespaceMessage {
	resp := &api.DumpNetworkNamespaceMessage{}
	dump := nr.dump(params)

	// create the dump file
	dumpFile, err := newTmpFile("network-namespace-dump-*.json")
	if err != nil {
		resp.Error = fmt.Sprintf("couldn't create temporary file: %v", err)
		seclog.Warnf("%s", err.Error())
		return resp
	}
	defer dumpFile.Close()
	resp.DumpFilename = dumpFile.Name()

	// dump to JSON file
	encoder := json.NewEncoder(dumpFile)
	if err = encoder.Encode(dump); err != nil {
		resp.Error = fmt.Sprintf("couldn't encode list of network namespace: %v", err)
		seclog.Warnf("%s", err.Error())
		return resp
	}

	if err = dumpFile.Close(); err != nil {
		resp.Error = fmt.Sprintf("could not close file [%s]: %s", dumpFile.Name(), err)
		seclog.Warnf("%s", err.Error())
		return resp
	}

	// create graph file
	graphFile, err := newTmpFile("network-namespace-graph-*.dot")
	if err != nil {
		resp.Error = fmt.Sprintf("couldn't create temporary file: %v", err)
		seclog.Warnf("%s", err.Error())
		return resp
	}
	defer graphFile.Close()
	resp.GraphFilename = graphFile.Name()

	// generate dot graph
	if err = nr.generateGraph(dump, graphFile); err != nil {
		resp.Error = fmt.Sprintf("couldn't generate dot graph: %v", err)
		seclog.Warnf("%s", err.Error())
		return resp
	}

	if err = graphFile.Close(); err != nil {
		resp.Error = fmt.Sprintf("could not close file [%s]: %s", graphFile.Name(), err)
		seclog.Warnf("%s", err.Error())
		return resp
	}

	return resp
}
