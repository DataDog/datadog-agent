package probe

import (
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/DataDog/gopsutil/process"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"

	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// NamespaceResolver is used to store namespace handles
type NamespaceResolver struct {
	sync.RWMutex
	state     int64
	probe     *Probe
	resolvers *Resolvers

	networkNamespaceHandles map[uint32]*os.File

	queuedNetworkDevicesLock sync.RWMutex
	queuedNetworkDevices     map[uint32][]model.NetDevice
}

// NewNamespaceResolver returns a new instance of NamespaceResolver
func NewNamespaceResolver(probe *Probe) *NamespaceResolver {
	return &NamespaceResolver{
		probe:                   probe,
		resolvers:               probe.resolvers,
		networkNamespaceHandles: make(map[uint32]*os.File),
		queuedNetworkDevices:    make(map[uint32][]model.NetDevice),
	}
}

// SetState sets the state of the namespace resolver
func (nr *NamespaceResolver) SetState(state int64) {
	atomic.StoreInt64(&nr.state, state)
}

func (nr *NamespaceResolver) openProcNetworkNamespace(pid uint32) (*os.File, error) {
	return os.Open(utils.NetNSPath(pid))
}

// SaveNetworkNamespaceHandle inserts the provided process network namespace in the list of tracked network. Returns
// true if a new handle was created.
func (nr *NamespaceResolver) SaveNetworkNamespaceHandle(netns uint32, tid uint32) bool {
	if atomic.LoadInt64(&nr.state) == snapshotting {
		nr.Lock()
		defer nr.Unlock()
	}

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
		nr.probe.setupNewTCClassifier(queuedDevice)
	}
	delete(nr.queuedNetworkDevices, netns)
	return true
}

// ResolveNetworkNamespaceHandle returns a file descriptor to the network namespace. WARNING: it is up to the caller to
// close this file descriptor when it is done using it. Do not forget to close this file descriptor, otherwise we might
// exhaust the host IPs by keeping all network namespaces alive.
func (nr *NamespaceResolver) ResolveNetworkNamespaceHandle(netns uint32) *os.File {
	if atomic.LoadInt64(&nr.state) == snapshotting {
		nr.Lock()
		defer nr.Unlock()
	}

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

// snapshotNetworkDevices snapshot the network devices of the provided network namespace
func (nr *NamespaceResolver) snapshotNetworkDevices(netns uint32) {
	if netns == 0 {
		return
	}

	// fetch namespace handle
	netnsHandle, ok := nr.networkNamespaceHandles[netns]
	if !ok {
		seclog.Errorf("no network namespace handle for %d", netns)
		return
	}

	conn, err := netlink.Dial(unix.NETLINK_ROUTE, &netlink.Config{NetNS: int(netnsHandle.Fd())})
	if err != nil {
		seclog.Errorf("couldn't open netlink socket: %s", err)
		return
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
		return
	}

	msgs, err := conn.Receive()
	if err != nil {
		seclog.Errorf("failed to receive interfaces list: %s", err)
		return
	}

	for _, msg := range msgs {
		switch msg.Header.Type {
		case syscall.NLMSG_DONE:
			return
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

			nr.probe.setupNewTCClassifier(device)
		}
	}
}

// SyncCache snapshots /proc for the provided pid. This method returns true if it updated the namespace cache.
func (nr *NamespaceResolver) SyncCache(proc *process.Process) bool {
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
		nr.snapshotNetworkDevices(netns)
	}
	return true
}

// QueueNetworkDevice adds the input device to the map of queued network devices. Once a handle for the network namespace
// of the device is resolved, a new TC classifier will automatically be added to the device. The queue is cleaned up
// periodically if a namespace do not own any process.
func (nr *NamespaceResolver) QueueNetworkDevice(device model.NetDevice) {
	if device.NetNS == 0 {
		return
	}

	nr.queuedNetworkDevicesLock.Lock()
	defer nr.queuedNetworkDevicesLock.Unlock()

	nr.queuedNetworkDevices[device.NetNS] = append(nr.queuedNetworkDevices[device.NetNS], device)
}
