// +build windows

package ebpf

/*
//! Defines the objects used to communicate with the driver as well as its control codes
#include "c/ddfilterapi.h"

//! These includes are needed to use constants defined in the ddfilterapi
#include <WinDef.h>
#include <WinIoCtl.h>
*/
import "C"
import (
	"encoding/binary"
	"expvar"
	"fmt"
	"net"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"golang.org/x/net/ipv4"
	"golang.org/x/sys/windows"
)

const (
	driverFile = `\\.\ddfilter`

	// Number of buffers to use with the IOCompletion port to communicate with the driver
	totalReadBuffers = 32
)

var (
	expvarEndpoints map[string]*expvar.Map
	expvarTypes     = []string{"driver_total_stats", "driver_handle_stats", "packet_count"}

	// Buffer holding datadog driver filterapi (ddfilterapi) signature to ensure consistency with driver.
	ddAPIVersionBuf = makeDDAPIVersionBuffer(C.DD_FILTER_SIGNATURE)
)

// Creates a buffer that Driver will use to verify proper versions are communicating
// We create a buffer because the system calls we make need a *byte which is not
// possible with const value
func makeDDAPIVersionBuffer(signature uint64) []byte {
	buf := make([]byte, C.sizeof_uint64_t)
	binary.LittleEndian.PutUint64(buf, signature)
	return buf
}

func init() {
	expvarEndpoints = make(map[string]*expvar.Map, len(expvarTypes))
	for _, name := range expvarTypes {
		expvarEndpoints[name] = expvar.NewMap(name)
	}
}

// Tracer struct for tracking network state and connections
type Tracer struct {
	config       *Config
	driverHandle windows.Handle
	iocp         windows.Handle
	bufs         []readBuffer
	packetCount  int64
}

// NewTracer returns an initialized tracer struct
func NewTracer(config *Config) (*Tracer, error) {
	handle, err := openDriverFile(driverFile)
	if err != nil {
		return nil, fmt.Errorf("%s : %s", "could not create driver handle", err)
	}

	// Create IO Completion port that we'll use to communicate with the driver
	iocp, err := windows.CreateIoCompletionPort(handle, windows.Handle(0), 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to create IO completion port %v", err)
	}

	tr := &Tracer{
		driverHandle: handle,
		iocp:         iocp,
		bufs:         make([]readBuffer, totalReadBuffers),
	}

	// Set the packet filters that will determine what we pull from the driver
	// TODO: Determine failure condition for not setting filter
	// TODO: I.e., one or more, all fail? I.E., at what point do we not create a tracer
	err = tr.prepareDriverFilters()
	if err != nil {
		return nil, fmt.Errorf("failed to setup packet filters on the driver: %v", err)
	}

	// Prepare the read buffers that will be used to send packets from kernel to user space
	err = tr.prepareReadBuffers()
	if err != nil {
		return nil, fmt.Errorf("failed to prepare ReadBuffers: %v", err)
	}

	err = tr.initPacketPolling()
	if err != nil {
		log.Warnf("issue polling packets from driver")
	}
	go tr.expvarStats()
	return tr, nil
}

// Stop function stops running tracer
func (t *Tracer) Stop() {}

func (t *Tracer) expvarStats() {
	ticker := time.NewTicker(5 * time.Second)
	// starts running the body immediately instead waiting for the first tick
	for range ticker.C {
		stats, err := t.GetStats()
		if err != nil {
			continue
		}

		for name, stat := range stats {
			for metric, val := range stat.(map[string]int64) {
				currVal := &expvar.Int{}
				currVal.Set(val)
				expvarEndpoints[name].Set(snakeToCapInitialCamel(metric), currVal)
			}
		}
	}
}

func (t *Tracer) initPacketPolling() (err error) {
	log.Debugf("Started packet polling")
	go func() {
		var (
			bytes uint32
			key   uint32
			ol    *windows.Overlapped
		)

		for {
			err := windows.GetQueuedCompletionStatus(t.iocp, &bytes, &key, &ol, windows.INFINITE)
			if err == nil {
				var buf *readBuffer
				buf = (*readBuffer)(unsafe.Pointer(ol))
				buf.printPacket()
				atomic.AddInt64(&t.packetCount, 1)
				windows.ReadFile(t.driverHandle, buf.data[:], nil, &(buf.ol))
			}
		}
	}()
	return
}

// GetActiveConnections returns all active connections
func (t *Tracer) GetActiveConnections(_ string) (*Connections, error) {
	return &Connections{
		DNS: map[util.Address][]string{
			util.AddressFromString("127.0.0.1"): {"localhost"},
		},
		Conns: []ConnectionStats{
			{
				Source: util.AddressFromString("127.0.0.1"),
				Dest:   util.AddressFromString("127.0.0.1"),
				SPort:  35673,
				DPort:  8000,
				Type:   TCP,
			},
		},
	}, nil
}

// getConnections returns all of the active connections in the ebpf maps along with the latest timestamp.  It takes
// a reusable buffer for appending the active connections so that this doesn't continuously allocate
func (t *Tracer) getConnections(active []ConnectionStats) ([]ConnectionStats, uint64, error) {
	return nil, 0, ErrNotImplemented
}

// GetStats returns a map of statistics about the current tracer's internal state
func (t *Tracer) GetStats() (map[string]interface{}, error) {
	var (
		bytesReturned uint32
		statbuf       = make([]byte, C.sizeof_struct_driver_stats)
	)

	err := windows.DeviceIoControl(t.driverHandle, C.DDFILTER_IOCTL_GETSTATS, &ddAPIVersionBuf[0], uint32(len(ddAPIVersionBuf)), &statbuf[0], uint32(len(statbuf)), &bytesReturned, nil)
	if err != nil {
		return nil, fmt.Errorf("error reading Stats with DeviceIoControl: %v", err)
	}

	stats := *(*C.struct_driver_stats)(unsafe.Pointer(&statbuf[0]))
	packetCount := atomic.LoadInt64(&t.packetCount)
	return map[string]interface{}{
		"packet_count": map[string]int64{
			"count": packetCount,
		},
		"driver_total_stats": map[string]int64{
			"read_calls":             int64(stats.total.read_calls),
			"read_bytes":             int64(stats.total.read_bytes),
			"read_calls_outstanding": int64(stats.total.read_calls_outstanding),
			"read_calls_cancelled":   int64(stats.total.read_calls_cancelled),
			"read_packets_skipped":   int64(stats.total.read_packets_skipped),
			"write_calls":            int64(stats.total.write_calls),
			"write_bytes":            int64(stats.total.write_bytes),
			"ioctl_calls":            int64(stats.total.ioctl_calls),
		},
		"driver_handle_stats": map[string]int64{
			"read_calls":             int64(stats.handle.read_calls),
			"read_bytes":             int64(stats.handle.read_bytes),
			"read_calls_outstanding": int64(stats.handle.read_calls_outstanding),
			"read_calls_cancelled":   int64(stats.handle.read_calls_cancelled),
			"read_packets_skipped":   int64(stats.handle.read_packets_skipped),
			"write_calls":            int64(stats.handle.write_calls),
			"write_bytes":            int64(stats.handle.write_bytes),
			"ioctl_calls":            int64(stats.handle.ioctl_calls),
		},
	}, nil
}

// NewDDAPIFilter returns a filter we can apply to the driver
func newDDAPIFilter(direction C.uint64_t, ifaceIndex int, isIPV4 bool) (fd C.struct__filterDefinition) {
	fd.filterVersion = C.DD_FILTER_SIGNATURE
	fd.size = C.sizeof_struct__filterDefinition
	fd.direction = direction

	if isIPV4 {
		fd.af = windows.AF_INET
		fd.v4InterfaceIndex = (C.ulonglong)(ifaceIndex)
	} else {
		fd.af = windows.AF_INET6
		fd.v6InterfaceIndex = (C.ulonglong)(ifaceIndex)
	}

	return fd
}

func (t *Tracer) setFilter(fd C.struct__filterDefinition) (err error) {
	var id int64
	err = windows.DeviceIoControl(t.driverHandle, C.DDFILTER_IOCTL_SET_FILTER, (*byte)(unsafe.Pointer(&fd)), uint32(unsafe.Sizeof(fd)), (*byte)(unsafe.Pointer(&id)), uint32(unsafe.Sizeof(id)), nil, nil)
	if err != nil {
		return log.Error("Failed to set filter: %s\n", err.Error())
	}
	return
}

// To capture all traffic for an interface, we create an inbound/outbound traffic filter
// for both IPV4 and IPV6 traffic going to that interface
func createFiltersForInterface(iface net.Interface) (filters []C.struct__filterDefinition) {
	filters = append(filters, newDDAPIFilter(C.DIRECTION_INBOUND, iface.Index, true))
	filters = append(filters, newDDAPIFilter(C.DIRECTION_OUTBOUND, iface.Index, true))
	filters = append(filters, newDDAPIFilter(C.DIRECTION_INBOUND, iface.Index, false))
	filters = append(filters, newDDAPIFilter(C.DIRECTION_OUTBOUND, iface.Index, false))
	return
}

func (t *Tracer) prepareDriverFilters() (err error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Error("Error getting interfaces: %s\n", err.Error())
		return
	}

	for _, i := range ifaces {
		log.Debugf("Setting filters for interface: %s [%+v]", i.Name, i)

		for _, filter := range createFiltersForInterface(i) {
			err = t.setFilter(filter)
			if err != nil {
				log.Warnf("Failed to set filter [%+v] on interface [%+v]\n", filter, i, err.Error())
			}
		}
	}
	return nil
}

// Add buffers to Tracer object. Even though the windows API will actually
// keep the buffers, add it to the tracer so that golang doesn't garbage collect
// the buffers out from under us
func (t *Tracer) prepareReadBuffers() (err error) {
	for i := 0; i < totalReadBuffers; i++ {
		err = windows.ReadFile(t.driverHandle, t.bufs[i].data[:], nil, &t.bufs[i].ol)
		if err != nil {
			if err != windows.ERROR_IO_PENDING {
				fmt.Printf("failed to initiate readfile %v\n", err)
				windows.CloseHandle(t.iocp)
				t.iocp = windows.Handle(0)
				t.bufs = nil
				return
			}
		}
	}
	err = nil
	return
}

// DebugNetworkState returns a map with the current tracer's internal state, for debugging
func (t *Tracer) DebugNetworkState(clientID string) (map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

// DebugNetworkMaps returns all connections stored in the maps without modifications from network state
func (t *Tracer) DebugNetworkMaps() (*Connections, error) {
	return nil, ErrNotImplemented
}

// CurrentKernelVersion is not implemented on this OS for Tracer
func CurrentKernelVersion() (uint32, error) {
	return 0, ErrNotImplemented
}

// readBuffer is the buffer to pass into ReadFile system call to pull out packets
type readBuffer struct {
	ol   windows.Overlapped
	data [128]byte
}

func (rb *readBuffer) printPacket() {
	var header ipv4.Header
	var pheader C.struct_filterPacketHeader
	dataStart := unsafe.Sizeof(pheader)
	log.Infof("data start is %d\n", dataStart)
	header.Parse(rb.data[dataStart:])
	log.Infof(" %v    ==>>    %v", header.Src.String(), header.Dst.String())
}

func openDriverFile(path string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	log.Debug("Creating Driver handle...")
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OVERLAPPED,
		windows.Handle(0))
	if err != nil {
		return windows.InvalidHandle, err
	}
	log.Info("Connected to driver and handle created")
	return h, nil
}

func closeDriverFile(handle windows.Handle) error {
	return windows.CloseHandle(handle)
}
