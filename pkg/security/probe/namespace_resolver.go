package probe

import (
	"context"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/DataDog/gopsutil/process"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"

	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// NamespaceResolver is used to store namespace handles
type NamespaceResolver struct {
	sync.RWMutex
	probe                              *Probe
	resolvers                          *Resolvers
	client                             *statsd.Client
	networkNamespaceHandles            map[uint32]*os.File
	queuedNetworkDevicesLock           sync.RWMutex
	queuedNetworkDevices               map[uint32][]model.NetDevice
	lonelyNetworkNamespacesTimeoutLock sync.RWMutex
	lonelyNetworkNamespacesTimeout     map[uint32]time.Time
}

// NewNamespaceResolver returns a new instance of NamespaceResolver
func NewNamespaceResolver(probe *Probe) *NamespaceResolver {
	return &NamespaceResolver{
		probe:                          probe,
		resolvers:                      probe.resolvers,
		client:                         probe.statsdClient,
		networkNamespaceHandles:        make(map[uint32]*os.File),
		queuedNetworkDevices:           make(map[uint32][]model.NetDevice),
		lonelyNetworkNamespacesTimeout: make(map[uint32]time.Time),
	}
}

func (nr *NamespaceResolver) openProcNetworkNamespace(pid uint32) (*os.File, error) {
	return os.Open(utils.NetNSPath(pid))
}

// SaveNetworkNamespaceHandle inserts the provided process network namespace in the list of tracked network. Returns
// true if a new handle was created.
func (nr *NamespaceResolver) SaveNetworkNamespaceHandle(netns uint32, tid uint32) bool {
	if !nr.probe.config.NetworkEnabled {
		return false
	}

	nr.Lock()
	defer nr.Unlock()

	if netns == 0 {
		return false
	}

	_, ok := nr.networkNamespaceHandles[netns]
	if ok {
		// we already have a handle for this network namespace, ignore
		return false
	}

	f, err := nr.openProcNetworkNamespace(tid)
	if err != nil {
		// we'll get this namespace another time, ignore
		return false
	}

	nr.networkNamespaceHandles[netns] = f

	// dequeue devices
	nr.queuedNetworkDevicesLock.Lock()
	defer nr.queuedNetworkDevicesLock.Unlock()

	for _, queuedDevice := range nr.queuedNetworkDevices[netns] {
		_ = nr.probe.setupNewTCClassifierWithNetNSHandle(queuedDevice, f)
	}
	delete(nr.queuedNetworkDevices, netns)
	return true
}

// ResolveNetworkNamespaceHandle returns a file descriptor to the network namespace. WARNING: it is up to the caller to
// close this file descriptor when it is done using it. Do not forget to close this file descriptor, otherwise we might
// exhaust the host IPs by keeping all network namespaces alive.
func (nr *NamespaceResolver) ResolveNetworkNamespaceHandle(netns uint32) *os.File {
	if !nr.probe.config.NetworkEnabled {
		return nil
	}

	nr.Lock()
	defer nr.Unlock()

	if netns == 0 {
		return nil
	}

	handle, ok := nr.networkNamespaceHandles[netns]
	if !ok {
		return nil
	}

	// duplicate the file descriptor to avoid race conditions with the resync
	dup, err := unix.Dup(int(handle.Fd()))
	if err != nil {
		return nil
	}
	return os.NewFile(uintptr(dup), handle.Name())
}

// snapshotNetworkDevices snapshot the network devices of the provided network namespace. This function returns the
// number of network devices to which egress and ingress TC classifiers were successfully attached.
func (nr *NamespaceResolver) snapshotNetworkDevices(netns uint32) int {
	if netns == 0 {
		return 0
	}

	// fetch namespace handle
	netnsHandle := nr.ResolveNetworkNamespaceHandle(netns)
	if netnsHandle == nil {
		seclog.Errorf("network namespace handle not found for %d", netns)
		return 0
	}
	defer netnsHandle.Close()

	conn, err := netlink.Dial(unix.NETLINK_ROUTE, &netlink.Config{NetNS: int(netnsHandle.Fd())})
	if err != nil {
		seclog.Errorf("couldn't open netlink socket: %s", err)
		return 0
	}

	m := netlink.Message{
		Header: netlink.Header{
			Type:     netlink.HeaderType(syscall.RTM_GETLINK),
			Flags:    netlink.Request | netlink.Dump,
			Sequence: 1,
		},
		Data: []byte{syscall.AF_UNSPEC},
	}
	_, err = conn.Send(m)
	if err != nil {
		seclog.Errorf("failed to send interfaces list request: %s", err)
		return 0
	}

	msgs, err := conn.Receive()
	if err != nil {
		seclog.Errorf("failed to receive interfaces list: %s", err)
		return 0
	}

	var attachedDeviceCount int
msgLoop:
	for _, msg := range msgs {
		switch msg.Header.Type {
		case syscall.NLMSG_DONE:
			break msgLoop
		case syscall.RTM_NEWLINK:
			var attrs []netlink.Attribute
			ifim := (*syscall.IfInfomsg)(unsafe.Pointer(&msg.Data[0]))
			attrs, err = netlink.UnmarshalAttributes(msg.Data[unsafe.Sizeof(syscall.IfInfomsg{}):])
			if err != nil {
				continue
			}

			device := model.NetDevice{
				IfIndex: uint32(ifim.Index),
				NetNS:   netns,
			}

			// Parse interface name
			for _, attr := range attrs {
				if attr.Type != syscall.IFLA_IFNAME {
					continue
				}
				device.Name = string(attr.Data[:len(attr.Data)-1])
			}

			if err = nr.probe.setupNewTCClassifier(device); err == nil {
				attachedDeviceCount++
			}
		}
	}
	return attachedDeviceCount
}

// SyncCache snapshots /proc for the provided pid. This method returns true if it updated the namespace cache.
func (nr *NamespaceResolver) SyncCache(proc *process.Process) bool {
	if !nr.probe.config.NetworkEnabled {
		return false
	}

	pid := uint32(proc.Pid)
	netns, err := utils.GetProcessNetworkNamespace(pid)
	if err != nil {
		return false
	}

	newEntry := nr.SaveNetworkNamespaceHandle(netns, pid)
	if !newEntry {
		return false
	}

	if newEntry {
		_ = nr.snapshotNetworkDevices(netns)
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

	nr.queuedNetworkDevicesLock.Lock()
	defer nr.queuedNetworkDevicesLock.Unlock()

	nr.queuedNetworkDevices[device.NetNS] = append(nr.queuedNetworkDevices[device.NetNS], device)
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
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nr.probe.flushInactiveTCPrograms()

			// There is a possible race condition if we lose all network device creations but do notice the new network
			// namespace: we will create a handle that will never be flushed by `nr.probe.flushInactiveNamespaces()`.
			// To detect this race, compute the list of namespaces that are in cache, but for which we do not have any
			// device. Defer a snapshot process for each of those namespaces, and delete them if the snapshot yields
			// no new device.
			nr.preventNetworkNamespaceDrift()
		}
	}
}

// flushNetworkNamespace flushes the cached entries for the provided network namespace. It returns true if an entry was
// deleted and false if the provided entry wasn't in cache.
func (nr *NamespaceResolver) flushNetworkNamespace(netns uint32) bool {
	nr.Lock()
	defer nr.Unlock()
	nr.queuedNetworkDevicesLock.Lock()
	defer nr.queuedNetworkDevicesLock.Unlock()

	handle, ok := nr.networkNamespaceHandles[netns]
	if !ok {
		return false
	}

	// remove queued devices
	delete(nr.queuedNetworkDevices, netns)

	// close network namespace handle to release the namespace
	handle.Close()

	// delete map entry
	delete(nr.networkNamespaceHandles, netns)
	return true
}

// preventNetworkNamespaceDrift ensures that we do not keep network namespace handles indefinitely
func (nr *NamespaceResolver) preventNetworkNamespaceDrift() {
	nr.Lock()
	defer nr.Unlock()

	nr.lonelyNetworkNamespacesTimeoutLock.Lock()
	defer nr.lonelyNetworkNamespacesTimeoutLock.Unlock()

	nr.probe.tcProgramsLock.RLock()
	defer nr.probe.tcProgramsLock.RUnlock()

	now := time.Now()
	lonelyNamespaceTimeout := now.Add(5 * time.Minute)

	// compute the list of network namespaces without any TC program
	var isLonely bool
	for netns := range nr.networkNamespaceHandles {
		isLonely = true
		for tcKey := range nr.probe.tcPrograms {
			if netns == tcKey.NetNS {
				isLonely = false
			}
		}

		if !isLonely {
			continue
		}

		// insert lonely namespace
		_, ok := nr.lonelyNetworkNamespacesTimeout[netns]
		if !ok {
			nr.lonelyNetworkNamespacesTimeout[netns] = lonelyNamespaceTimeout
		}
	}

	// snapshot lonely namespaces and delete them if they are all alone on earth
	for lonelyNamespace, timeout := range nr.lonelyNetworkNamespacesTimeout {
		if now.Before(timeout) {
			// your doomsday hasn't come yet, lucky you
			continue
		}

		// snapshot the namespace
		deviceCount := nr.snapshotNetworkDevices(lonelyNamespace)
		if deviceCount == 0 {
			nr.flushNetworkNamespace(lonelyNamespace)
		}
		delete(nr.lonelyNetworkNamespacesTimeout, lonelyNamespace)
	}
}

// SendStats sends metrics about the current state of the namespace resolver
func (nr *NamespaceResolver) SendStats() error {
	nr.RLock()
	defer nr.RUnlock()
	val := float64(len(nr.networkNamespaceHandles))
	if val > 0 {
		_ = nr.client.Gauge(metrics.MetricNamespaceResolverNetNSHandle, val, []string{}, 1.0)
	}

	nr.queuedNetworkDevicesLock.RLock()
	defer nr.queuedNetworkDevicesLock.RUnlock()
	val = float64(len(nr.queuedNetworkDevices))
	if val > 0 {
		_ = nr.client.Gauge(metrics.MetricNamespaceResolverQueuedNetworkDevice, val, []string{}, 1.0)
	}

	nr.lonelyNetworkNamespacesTimeoutLock.RLock()
	defer nr.lonelyNetworkNamespacesTimeoutLock.RUnlock()
	val = float64(len(nr.lonelyNetworkNamespacesTimeout))
	if val > 0 {
		_ = nr.client.Gauge(metrics.MetricNamespaceResolverLonelyNetworkNamespace, val, []string{}, 1.0)
	}
	return nil
}
