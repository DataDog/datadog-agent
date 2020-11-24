// +build windows

package network

/*
//! These includes are needed to use constants defined in the ddnpmapi
#include <WinDef.h>
#include <WinIoCtl.h>

//! Defines the objects used to communicate with the driver as well as its control codes
#include "ddnpmapi.h"
*/
import "C"
import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// HandleType represents what type of data the windows handle created on the driver is intended to return. It implicitly implies if there are filters set for a handle
type HandleType string

const (
	// deviceName identifies the name and location of the windows driver
	deviceName = `\\.\ddnpm`

	// FlowHandle is keyed to return 5-tuples from the driver that represents a flow. Used with: (#define FILTER_LAYER_TRANSPORT ((uint64_t) 1)
	FlowHandle HandleType = "Flow"

	// DataHandle is keyed to return full packets from the driver. Used with: #define FILTER_LAYER_IPPACKET ((uint64_t) 0)
	DataHandle HandleType = "Data"

	// StatsHandle has no filter set and is used to pull total stats from the driver
	StatsHandle HandleType = "Stats"
)

var (
	// Buffer holding datadog driver filterapi (ddnpmapi) signature to ensure consistency with driver.
	ddAPIVersionBuf = makeDDAPIVersionBuffer(C.DD_NPMDRIVER_SIGNATURE)
)

// DriverExpvar is the name of a top-level driver expvar returned from GetStats
type DriverExpvar string

const (
	totalFlowStats  DriverExpvar = "driver_total_flow_stats"
	flowHandleStats              = "driver_flow_handle_stats"
	flowStats                    = "flows"
	driverStats                  = "driver"
)

// DriverExpvarNames is a list of all the DriverExpvar names returned from GetStats
var DriverExpvarNames = []DriverExpvar{totalFlowStats, flowHandleStats, flowStats, driverStats}

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
	// declare totalFlows first so it remains on a 64 bit boundary since it is used by atomic functions
	totalFlows     int64
	closedFlows    int64
	openFlows      int64
	moreDataErrors int64
	bufferSize     int64

	driverFlowHandle  *DriverHandle
	driverStatsHandle *DriverHandle

	path                  string
	enableMonotonicCounts bool

	bufferLock sync.Mutex
	readBuffer []uint8
}

// NewDriverInterface returns a DriverInterface struct for interacting with the driver
func NewDriverInterface(enableMonotonicCounts bool, bufferSize int) (*DriverInterface, error) {
	dc := &DriverInterface{
		path:                  deviceName,
		enableMonotonicCounts: enableMonotonicCounts,
		readBuffer:            make([]byte, bufferSize),
		bufferSize:            int64(bufferSize),
	}

	err := dc.setupFlowHandle()
	if err != nil {
		return nil, errors.Wrap(err, "error creating driver flow handle")
	}

	err = dc.setupStatsHandle()
	if err != nil {
		return nil, errors.Wrap(err, "Error creating stats handle")
	}

	return dc, nil
}

// Close shuts down the driver interface
func (di *DriverInterface) Close() error {
	if err := windows.CloseHandle(di.driverFlowHandle.handle); err != nil {
		return errors.Wrap(err, "error closing flow file handle")
	}
	if err := windows.CloseHandle(di.driverStatsHandle.handle); err != nil {
		return errors.Wrap(err, "error closing stat file handle")
	}
	return nil
}

// setupFlowHandle generates a windows Driver Handle, and creates a DriverHandle struct to pull flows from the driver
// by setting the necessary filters
func (di *DriverInterface) setupFlowHandle() error {
	h, err := di.generateDriverHandle()
	if err != nil {
		return err
	}
	dh, err := NewDriverHandle(h, FlowHandle)
	if err != nil {
		return err
	}
	di.driverFlowHandle = dh

	filters, err := createFlowHandleFilters()
	if err != nil {
		return err
	}

	// Create and set flow filters for each interface
	err = di.driverFlowHandle.setFilters(filters)
	if err != nil {
		return err
	}

	return nil
}

// setupStatsHandle generates a windows Driver Handle, and creates a DriverHandle struct
func (di *DriverInterface) setupStatsHandle() error {
	h, err := di.generateDriverHandle()
	if err != nil {
		return err
	}

	dh, err := NewDriverHandle(h, StatsHandle)
	if err != nil {
		return err
	}

	di.driverStatsHandle = dh
	return nil

}

// generateDriverHandle creates a new windows handle attached to the driver
func (di *DriverInterface) generateDriverHandle() (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(di.path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	log.Debug("Creating Driver handle...")
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		windows.Handle(0))
	if err != nil {
		return windows.InvalidHandle, err
	}
	return h, nil
}

// closeDriverHandle closes an open handle on the driver
func (di *DriverInterface) closeDriverHandle(handle windows.Handle) error {
	return windows.CloseHandle(handle)
}

// GetStats returns statistics for the driver interface used by the windows tracer
func (di *DriverInterface) GetStats() (map[DriverExpvar]interface{}, error) {

	handleStats, err := di.driverFlowHandle.getStatsForHandle()
	if err != nil {
		return nil, err
	}

	totalDriverStats, err := di.driverStatsHandle.getStatsForHandle()
	if err != nil {
		return nil, err
	}
	totalFlows := atomic.LoadInt64(&di.totalFlows)
	openFlows := atomic.SwapInt64(&di.openFlows, 0)
	closedFlows := atomic.SwapInt64(&di.closedFlows, 0)
	moreDataErrors := atomic.SwapInt64(&di.moreDataErrors, 0)
	bufferSize := atomic.LoadInt64(&di.bufferSize)

	return map[DriverExpvar]interface{}{
		totalFlowStats:  totalDriverStats,
		flowHandleStats: handleStats,
		flowStats: map[string]int64{
			"total":  totalFlows,
			"open":   openFlows,
			"closed": closedFlows,
		},
		driverStats: map[string]int64{
			"more_data_errors": moreDataErrors,
			"buffer_size":      bufferSize,
		},
	}, nil
}

// GetConnectionStats will read all flows from the driver and convert them into ConnectionStats.
// It returns the count of connections added to the active and closed buffers, respectively.
func (di *DriverInterface) GetConnectionStats(activeBuf *DriverBuffer, closedBuf *DriverBuffer) (int, int, error) {
	di.bufferLock.Lock()
	defer di.bufferLock.Unlock()

	var activeCount, closedCount int
	var bytesRead uint32
	var totalBytesRead uint32
	// keep reading while driver says there is more data available
	for err := error(windows.ERROR_MORE_DATA); err == windows.ERROR_MORE_DATA; {
		err = windows.ReadFile(di.driverFlowHandle.handle, di.readBuffer, &bytesRead, nil)
		if err != nil {
			if err != windows.ERROR_MORE_DATA {
				return 0, 0, err
			}
			atomic.AddInt64(&di.moreDataErrors, 1)
		}
		totalBytesRead += bytesRead

		var buf []byte
		for bytesUsed := uint32(0); bytesUsed < bytesRead; bytesUsed += C.sizeof_struct__perFlowData {
			buf = di.readBuffer[bytesUsed:]
			pfd := (*C.struct__perFlowData)(unsafe.Pointer(&(buf[0])))

			if isFlowClosed(pfd.flags) {
				FlowToConnStat(closedBuf.Next(), pfd, di.enableMonotonicCounts)
				closedCount++
			} else {
				FlowToConnStat(activeBuf.Next(), pfd, di.enableMonotonicCounts)
				activeCount++
			}
		}
	}

	di.readBuffer = resizeDriverBuffer(int(totalBytesRead), di.readBuffer)
	atomic.StoreInt64(&di.bufferSize, int64(len(di.readBuffer)))

	atomic.AddInt64(&di.openFlows, int64(activeCount))
	atomic.AddInt64(&di.closedFlows, int64(closedCount))
	atomic.AddInt64(&di.totalFlows, int64(activeCount+closedCount))

	return activeCount, closedCount, nil
}

func resizeDriverBuffer(compareSize int, buffer []uint8) []uint8 {
	// Explicitly setting len to 0 causes the ReadFile syscall to break, so allocate buffer with cap = len
	if compareSize >= cap(buffer)*2 {
		return make([]uint8, cap(buffer)*2)
	} else if compareSize <= cap(buffer)/2 {
		// Take the max of buffer/2 and compareSize to limit future array resizes
		return make([]uint8, int(math.Max(float64(cap(buffer)/2), float64(compareSize))))
	}
	return buffer
}

// DriverHandle struct stores the windows handle for the driver as well as information about what type of filter is set
type DriverHandle struct {
	handle     windows.Handle
	handleType HandleType
}

// NewDriverHandle returns a DriverHandle struct
func NewDriverHandle(h windows.Handle, handleType HandleType) (*DriverHandle, error) {
	return &DriverHandle{handle: h, handleType: handleType}, nil
}

func (dh *DriverHandle) setFilters(filters []C.struct__filterDefinition) error {
	var id int64
	for _, filter := range filters {
		err := windows.DeviceIoControl(dh.handle,
			C.DDNPMDRIVER_IOCTL_SET_FLOW_FILTER,
			(*byte)(unsafe.Pointer(&filter)),
			uint32(unsafe.Sizeof(filter)),
			(*byte)(unsafe.Pointer(&id)),
			uint32(unsafe.Sizeof(id)), nil, nil)
		if err != nil {
			return fmt.Errorf("failed to set filter: %v", err)
		}
	}
	return nil
}

func (dh *DriverHandle) getStatsForHandle() (map[string]int64, error) {
	var (
		bytesReturned uint32
		statbuf       = make([]byte, C.sizeof_struct_driver_stats)
	)

	err := windows.DeviceIoControl(dh.handle, C.DDNPMDRIVER_IOCTL_GETSTATS, &ddAPIVersionBuf[0], uint32(len(ddAPIVersionBuf)), &statbuf[0], uint32(len(statbuf)), &bytesReturned, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read driver stats for filter type %v - returned error %v", dh.handleType, err)
	}
	stats := *(*C.struct_driver_stats)(unsafe.Pointer(&statbuf[0]))

	switch dh.handleType {

	// A stats handle returns the total values of the driver
	case StatsHandle:
		return map[string]int64{
			"read_calls":                  int64(stats.total.handle_stats.read_calls),
			"read_calls_outstanding":      int64(stats.total.handle_stats.read_calls_outstanding),
			"read_calls_completed":        int64(stats.total.handle_stats.read_calls_completed),
			"read_calls_cancelled":        int64(stats.total.handle_stats.read_calls_cancelled),
			"write_calls":                 int64(stats.total.handle_stats.write_calls),
			"write_bytes":                 int64(stats.total.handle_stats.write_bytes),
			"ioctl_calls":                 int64(stats.total.handle_stats.ioctl_calls),
			"packets_observed":            int64(stats.total.flow_stats.packets_observed),
			"packets_processed_flow":      int64(stats.total.flow_stats.packets_processed),
			"open_flows":                  int64(stats.total.flow_stats.open_flows),
			"total_flows":                 int64(stats.total.flow_stats.total_flows),
			"num_flow_searches":           int64(stats.total.flow_stats.num_flow_searches),
			"num_flow_search_misses":      int64(stats.total.flow_stats.num_flow_search_misses),
			"num_flow_collisions":         int64(stats.total.flow_stats.num_flow_collisions),
			"packets_processed_transport": int64(stats.total.transport_stats.packets_processed),
			"read_packets_skipped":        int64(stats.total.transport_stats.read_packets_skipped),
			"packets_reported":            int64(stats.total.transport_stats.packets_reported),
		}, nil
	// A FlowHandle handle returns the flow stats specific to this handle
	case FlowHandle:
		return map[string]int64{
			"read_calls":             int64(stats.handle.handle_stats.read_calls),
			"read_calls_outstanding": int64(stats.handle.handle_stats.read_calls_outstanding),
			"read_calls_completed":   int64(stats.handle.handle_stats.read_calls_completed),
			"read_calls_cancelled":   int64(stats.handle.handle_stats.read_calls_cancelled),
			"write_calls":            int64(stats.handle.handle_stats.write_calls),
			"write_bytes":            int64(stats.handle.handle_stats.write_bytes),
			"ioctl_calls":            int64(stats.handle.handle_stats.ioctl_calls),
			"packets_observed":       int64(stats.handle.flow_stats.packets_observed),
			"packets_processed_flow": int64(stats.handle.flow_stats.packets_processed),
			"open_flows":             int64(stats.handle.flow_stats.open_flows),
			"total_flows":            int64(stats.handle.flow_stats.total_flows),
			"num_flow_searches":      int64(stats.handle.flow_stats.num_flow_searches),
			"num_flow_search_misses": int64(stats.handle.flow_stats.num_flow_search_misses),
			"num_flow_collisions":    int64(stats.handle.flow_stats.num_flow_collisions),
		}, nil
	// A DataHandle handle returns transfer stats specific to this handle
	case DataHandle:
		return map[string]int64{
			"read_calls":                  int64(stats.handle.handle_stats.read_calls),
			"read_calls_outstanding":      int64(stats.handle.handle_stats.read_calls_outstanding),
			"read_calls_completed":        int64(stats.handle.handle_stats.read_calls_completed),
			"read_calls_cancelled":        int64(stats.handle.handle_stats.read_calls_cancelled),
			"write_calls":                 int64(stats.handle.handle_stats.write_calls),
			"write_bytes":                 int64(stats.handle.handle_stats.write_bytes),
			"ioctl_calls":                 int64(stats.handle.handle_stats.ioctl_calls),
			"packets_processed_transport": int64(stats.handle.transport_stats.packets_processed),
			"read_packets_skipped":        int64(stats.handle.transport_stats.read_packets_skipped),
			"packets_reported":            int64(stats.handle.transport_stats.packets_reported),
		}, nil
	default:
		return nil, fmt.Errorf("no matching handle type for pulling handle stats")
	}
}

func createFlowHandleFilters() (filters []C.struct__filterDefinition, err error) {
	ifaces, err := net.Interfaces()

	// Two filters per iface
	if err != nil {
		return nil, fmt.Errorf("error getting interfaces: %s", err.Error())
	}

	for _, iface := range ifaces {
		log.Debugf("Creating filters for interface: %s [%+v]", iface.Name, iface)
		// Set ipv4 Traffic
		filters = append(filters, newDDAPIFilter(C.DIRECTION_OUTBOUND, C.FILTER_LAYER_TRANSPORT, iface.Index, true))
		filters = append(filters, newDDAPIFilter(C.DIRECTION_INBOUND, C.FILTER_LAYER_TRANSPORT, iface.Index, true))
		// Set ipv6
		filters = append(filters, newDDAPIFilter(C.DIRECTION_OUTBOUND, C.FILTER_LAYER_TRANSPORT, iface.Index, false))
		filters = append(filters, newDDAPIFilter(C.DIRECTION_INBOUND, C.FILTER_LAYER_TRANSPORT, iface.Index, false))
	}

	return filters, nil
}

// NewDDAPIFilter returns a filter we can apply to the driver
func newDDAPIFilter(direction, layer C.uint64_t, ifaceIndex int, isIPV4 bool) C.struct__filterDefinition {
	var fd C.struct__filterDefinition
	fd.filterVersion = C.DD_NPMDRIVER_SIGNATURE
	fd.size = C.sizeof_struct__filterDefinition
	// TODO Remove direction setting for flow filters once all verification code has been removed from driver
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
