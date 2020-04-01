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
	"fmt"
	"net"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"golang.org/x/sys/windows"
)

const (
	driverFile = `\\.\ddfilter`
)

var (
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

// DriverInterface holds all necessary information for interacting with the windows driver
type DriverInterface struct {
	driverHandle windows.Handle
	iocp         windows.Handle
	path         string
}

// NewDriverInterface returns a DriverInterface struct for interacting with the driver
func NewDriverInterface() (*DriverInterface, error) {
	dc := &DriverInterface{
		path: driverFile,
	}

	handle, err := dc.NewDriverHandle()
	if err != nil {
		return nil, fmt.Errorf("%s : %s", "error creating driver handle", err)
	}

	// Create IO Completion port that we'll use to communicate with the driver
	iocp, err := windows.CreateIoCompletionPort(handle, windows.Handle(0), 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to create IO completion port %v", err)
	}

	dc.driverHandle = handle
	dc.iocp = iocp

	// Set the driver buffer parameters
	var params C.struct__handle_buffer_cfg
	params.filterVersion = C.DD_FILTER_SIGNATURE
	params.numPacketBuffers = 2048
	params.sizeOfBuffer = 128
	params.packetMode = C.PACKET_REPORT_MODE_HEADER_ONLY;
	err = dc.setDriverBufferConfig(params)
	if err != nil {
		return nil, err
	}

	// Set the packet filters that will determine what we pull from the driver
	err = dc.prepareDriverFilters()
	if err != nil {
		return nil, fmt.Errorf("failed to setup packet filters on the driver: %v", err)
	}

	return dc, nil
}

// NewDriverHandle creates a new driver handle attached to the windows driver
func (dc *DriverInterface) NewDriverHandle() (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(dc.path)
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

// CloseDriverHandle closes an open handle on the driver
func (dc *DriverInterface) CloseDriverHandle(handle windows.Handle) error {
	return windows.CloseHandle(handle)
}

// Add buffers to Tracer object. Even though the windows API will actually
// keep the buffers, add it to the tracer so that golang doesn't garbage collect
// the buffers out from under us
func (dc *DriverInterface) prepareReadBuffers(bufs []readBuffer) ([]readBuffer, error) {
	for i := 0; i < totalReadBuffers; i++ {
		err := windows.ReadFile(dc.driverHandle, bufs[i].data[:], nil, &bufs[i].ol)
		if err != nil {
			if err != windows.ERROR_IO_PENDING {
				fmt.Printf("failed to initiate readfile %v\n", err)
				windows.CloseHandle(dc.iocp)
				dc.iocp = windows.Handle(0)
				return nil, err
			}
		}
	}
	return bufs, nil
}

func (dc *DriverInterface) prepareDriverFilters() error {
	ifaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("error getting interfaces: %s", err.Error())
	}

	for _, i := range ifaces {
		log.Debugf("Setting filters for interface: %s [%+v]", i.Name, i)

		for _, filter := range createFiltersForInterface(i) {
			err = dc.setFilter(filter)
			if err != nil {
				return log.Warnf("failed to set filter [%+v] on interface [%+v]: %v", filter, i, err)
			}
		}
	}
	return nil
}

// To capture all traffic for an interface, we create an inbound/outbound traffic filter
// for both IPV4 and IPV6 traffic going to that interface
func createFiltersForInterface(iface net.Interface) (filters []C.struct__filterDefinition) {
	filters = append(filters, newDDAPIFilter(C.DIRECTION_OUTBOUND, C.FILTER_LAYER_ALE_CONNECT, iface.Index, true))
	filters = append(filters, newDDAPIFilter(C.DIRECTION_OUTBOUND, C.FILTER_LAYER_TRANSPORT, iface.Index, true))

	filters = append(filters, newDDAPIFilter(C.DIRECTION_INBOUND, C.FILTER_LAYER_ALE_RECVCONN, iface.Index, true))
	filters = append(filters, newDDAPIFilter(C.DIRECTION_INBOUND, C.FILTER_LAYER_TRANSPORT, iface.Index, true))

	// this one doesn't really have a "direction".  TODO  Should probably clarify the API
	filters = append(filters, newDDAPIFilter(C.DIRECTION_INBOUND, C.FILTER_LAYER_ALE_CLOSURE, iface.Index, true))

	// for now skip ipv6
	//filters = append(filters, newDDAPIFilter(C.DIRECTION_INBOUND, iface.Index, false))
	//filters = append(filters, newDDAPIFilter(C.DIRECTION_OUTBOUND, iface.Index, false))
	return
}

// NewDDAPIFilter returns a filter we can apply to the driver
func newDDAPIFilter(direction, layer C.uint64_t, ifaceIndex int, isIPV4 bool) (fd C.struct__filterDefinition) {
	fd.filterVersion = C.DD_FILTER_SIGNATURE
	fd.size = C.sizeof_struct__filterDefinition
	fd.direction = direction
	fd.filterLayer = layer

	if isIPV4 {
		fd.af = windows.AF_INET
		fd.v4InterfaceIndex = (C.ulonglong)(ifaceIndex)
	} else {
		fd.af = windows.AF_INET6
		fd.v6InterfaceIndex = (C.ulonglong)(ifaceIndex)
	}

	return fd
}

func (dc *DriverInterface) setFilter(fd C.struct__filterDefinition) error {
	var id int64
	err := windows.DeviceIoControl(dc.driverHandle, C.DDFILTER_IOCTL_SET_FILTER, (*byte)(unsafe.Pointer(&fd)), uint32(unsafe.Sizeof(fd)), (*byte)(unsafe.Pointer(&id)), uint32(unsafe.Sizeof(id)), nil, nil)
	if err != nil {
		return fmt.Errorf("failed to set filter: %v", err)
	}
	return nil
}

func (dc *DriverInterface)  setDriverBufferConfig(cfg C.struct__handle_buffer_cfg) error {
	var outlen uint32
	err := windows.DeviceIoControl(dc.driverHandle, C.DDFILTER_IOCTL_SET_HANDLE_BUFFER_CONFIG, (*byte)(unsafe.Pointer(&cfg)), uint32(unsafe.Sizeof(cfg)), 
	nil, 0, &outlen, nil)
	if err != nil {
		return fmt.Errorf("Failed to set driver buffer params")
	}
	return nil
}
func (dc *DriverInterface) getStats() (map[string]interface{}, error) {
	var (
		bytesReturned uint32
		statbuf       = make([]byte, C.sizeof_struct_driver_stats)
	)

	err := windows.DeviceIoControl(dc.driverHandle, C.DDFILTER_IOCTL_GETSTATS, &ddAPIVersionBuf[0], uint32(len(ddAPIVersionBuf)), &statbuf[0], uint32(len(statbuf)), &bytesReturned, nil)
	if err != nil {
		return nil, fmt.Errorf("error reading driver stats with DeviceIoControl: %v", err)
	}

	stats := *(*C.struct_driver_stats)(unsafe.Pointer(&statbuf[0]))
	return map[string]interface{}{
		"driver_total_stats": map[string]int64{
			"read_calls":             int64(stats.total.read_calls),
			"read_bytes":             int64(stats.total.read_bytes),
			"read_calls_outstanding": int64(stats.total.read_calls_outstanding),
			"read_calls_cancelled":   int64(stats.total.read_calls_cancelled),
			"read_packets_skipped":   int64(stats.total.read_packets_skipped),
			"write_calls":            int64(stats.total.write_calls),
			"write_bytes":            int64(stats.total.write_bytes),
			"packets_processed":      int64(stats.total.packets_processed),
			"packets_queued":         int64(stats.total.packets_queued),
			"packets_reported":       int64(stats.total.packets_reported),
			"packets_pended":         int64(stats.total.packets_pended),
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
			"packets_processed":      int64(stats.handle.packets_processed),
			"packets_queued":         int64(stats.handle.packets_queued),
			"packets_reported":       int64(stats.handle.packets_reported),
			"packets_pended":         int64(stats.handle.packets_pended),
		},
	}, nil
}
