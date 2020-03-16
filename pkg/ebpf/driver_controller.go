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

	"golang.org/x/net/ipv4"
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

type DriverController struct {
	driverHandle windows.Handle
	iocp         windows.Handle
	path         string
}

func NewDriverController() (*DriverController, error) {
	dc := &DriverController{
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

	// Set the packet filters that will determine what we pull from the driver
	// TODO: Determine failure condition for not setting filter
	// TODO: I.e., one or more, all fail? I.E., at what point do we not create a tracer
	err = dc.prepareDriverFilters()
	if err != nil {
		return nil, fmt.Errorf("failed to setup packet filters on the driver: %v", err)
	}

	return dc, nil
}

func (dc *DriverController) NewDriverHandle() (windows.Handle, error) {
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

func (dc *DriverController) CloseDriverHandle(handle windows.Handle) error {
	return windows.CloseHandle(handle)
}

// Add buffers to Tracer object. Even though the windows API will actually
// keep the buffers, add it to the tracer so that golang doesn't garbage collect
// the buffers out from under us
func (dc *DriverController) prepareReadBuffers(bufs []readBuffer) ([]readBuffer, error) {
	for i := 0; i < totalReadBuffers; i++ {
		err := windows.ReadFile(dc.driverHandle, bufs[i].data[:], nil, &bufs[i].ol)
		if err != nil {
			if err != windows.ERROR_IO_PENDING {
				fmt.Printf("failed to initiate readfile %v\n", err)
				windows.CloseHandle(dc.iocp)
				dc.iocp = windows.Handle(0)
				bufs = nil
				return nil, err
			}
		}
	}
	return bufs, nil
}

func (dc *DriverController) prepareDriverFilters() (err error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Errorf("Error getting interfaces: %s\n", err.Error())
		return
	}

	for _, i := range ifaces {
		log.Debugf("Setting filters for interface: %s [%+v]", i.Name, i)

		for _, filter := range createFiltersForInterface(i) {
			err = dc.setFilter(filter)
			if err != nil {
				return log.Warnf("Failed to set filter [%+v] on interface [%+v]: %s\n", filter, i, err.Error())
			}
		}
	}
	return nil
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

func (dc *DriverController) setFilter(fd C.struct__filterDefinition) (err error) {
	var id int64
	err = windows.DeviceIoControl(dc.driverHandle, C.DDFILTER_IOCTL_SET_FILTER, (*byte)(unsafe.Pointer(&fd)), uint32(unsafe.Sizeof(fd)), (*byte)(unsafe.Pointer(&id)), uint32(unsafe.Sizeof(id)), nil, nil)
	if err != nil {
		return log.Errorf("Failed to set filter: %s\n", err.Error())
	}
	return
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
