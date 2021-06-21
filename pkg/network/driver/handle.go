//+build windows

package driver

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows"
)

const (
	// deviceName identifies the name and location of the windows driver
	deviceName = `\\.\ddnpm`
)

var (
	// Buffer holding datadog driver filterapi (ddnpmapi) signature to ensure consistency with driver.
	ddAPIVersionBuf = makeDDAPIVersionBuffer(Signature)
)

// Creates a buffer that Driver will use to verify proper versions are communicating
// We create a buffer because the system calls we make need a *byte which is not
// possible with const value
func makeDDAPIVersionBuffer(signature uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, signature)
	return buf
}

// HandleType represents what type of data the windows handle created on the driver is intended to return. It implicitly implies if there are filters set for a handle
type HandleType string

const (
	// FlowHandle is keyed to return 5-tuples from the driver that represents a flow. Used with: (#define FILTER_LAYER_TRANSPORT ((uint64_t) 1)
	FlowHandle HandleType = "Flow"

	// DataHandle is keyed to return full packets from the driver. Used with: #define FILTER_LAYER_IPPACKET ((uint64_t) 0)
	DataHandle HandleType = "Data"

	// StatsHandle has no filter set and is used to pull total stats from the driver
	StatsHandle HandleType = "Stats"
)

// Handle struct stores the windows handle for the driver as well as information about what type of filter is set
type Handle struct {
	windows.Handle
	handleType HandleType

	// record the last value of number of flows missed due to max exceeded
	lastNumFlowsMissed uint64
}

// NewHandle creates a new windows handle attached to the driver
func NewHandle(flags uint32, handleType HandleType) (*Handle, error) {
	p, err := windows.UTF16PtrFromString(deviceName)
	if err != nil {
		return nil, err
	}
	log.Debugf("Creating driver handle of type %s", handleType)
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		flags,
		windows.Handle(0))
	if err != nil {
		return nil, err
	}
	return &Handle{Handle: h, handleType: handleType}, nil
}

// Close closes the underlying windows handle
func (dh *Handle) Close() error {
	return windows.CloseHandle(dh.Handle)
}

// SetFlowFilters installs the provided filters for flows
func (dh *Handle) SetFlowFilters(filters []FilterDefinition) error {
	var id int64
	for _, filter := range filters {
		err := windows.DeviceIoControl(dh.Handle,
			SetFlowFilterIOCTL,
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

// SetDataFilters installs the provided filters for data
func (dh *Handle) SetDataFilters(filters []FilterDefinition) error {
	var id int64
	for _, filter := range filters {
		err := windows.DeviceIoControl(dh.Handle,
			SetDataFilterIOCTL,
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

// GetStatsForHandle gets the relevant stats depending on the handle type
func (dh *Handle) GetStatsForHandle() (map[string]int64, error) {
	var (
		bytesReturned uint32
		statbuf       = make([]byte, DriverStatsSize)
	)

	err := windows.DeviceIoControl(dh.Handle, GetStatsIOCTL, &ddAPIVersionBuf[0], uint32(len(ddAPIVersionBuf)), &statbuf[0], uint32(len(statbuf)), &bytesReturned, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read driver stats for filter type %v - returned error %v", dh.handleType, err)
	}
	stats := *(*DriverStats)(unsafe.Pointer(&statbuf[0]))

	switch dh.handleType {

	// A stats handle returns the total values of the driver
	case StatsHandle:
		return map[string]int64{
			"read_calls":                  stats.Total.Handle_stats.Read_calls,
			"read_calls_outstanding":      stats.Total.Handle_stats.Read_calls_outstanding,
			"read_calls_completed":        stats.Total.Handle_stats.Read_calls_completed,
			"read_calls_cancelled":        stats.Total.Handle_stats.Read_calls_cancelled,
			"write_calls":                 stats.Total.Handle_stats.Write_calls,
			"write_bytes":                 stats.Total.Handle_stats.Write_bytes,
			"ioctl_calls":                 stats.Total.Handle_stats.Ioctl_calls,
			"packets_observed":            stats.Total.Flow_stats.Packets_observed,
			"packets_processed_flow":      stats.Total.Flow_stats.Packets_processed,
			"open_flows":                  stats.Total.Flow_stats.Open_flows,
			"total_flows":                 stats.Total.Flow_stats.Total_flows,
			"num_flow_searches":           stats.Total.Flow_stats.Num_flow_searches,
			"num_flow_search_misses":      stats.Total.Flow_stats.Num_flow_search_misses,
			"num_flow_collisions":         stats.Total.Flow_stats.Num_flow_collisions,
			"packets_processed_transport": stats.Total.Transport_stats.Packets_processed,
			"read_packets_skipped":        stats.Total.Transport_stats.Read_packets_skipped,
			"packets_reported":            stats.Total.Transport_stats.Packets_reported,
		}, nil
	// A FlowHandle handle returns the flow stats specific to this handle
	case FlowHandle:
		if dh.lastNumFlowsMissed < uint64(stats.Handle.Flow_stats.Num_flows_missed_max_exceeded) {
			log.Warnf("Flows missed due to maximum flow limit. %v", stats.Handle.Flow_stats.Num_flows_missed_max_exceeded)
		}
		dh.lastNumFlowsMissed = uint64(stats.Handle.Flow_stats.Num_flows_missed_max_exceeded)
		return map[string]int64{
			"read_calls":                    stats.Handle.Handle_stats.Read_calls,
			"read_calls_outstanding":        stats.Handle.Handle_stats.Read_calls_outstanding,
			"read_calls_completed":          stats.Handle.Handle_stats.Read_calls_completed,
			"read_calls_cancelled":          stats.Handle.Handle_stats.Read_calls_cancelled,
			"write_calls":                   stats.Handle.Handle_stats.Write_calls,
			"write_bytes":                   stats.Handle.Handle_stats.Write_bytes,
			"ioctl_calls":                   stats.Handle.Handle_stats.Ioctl_calls,
			"packets_observed":              stats.Handle.Flow_stats.Packets_observed,
			"packets_processed_flow":        stats.Handle.Flow_stats.Packets_processed,
			"open_flows":                    stats.Handle.Flow_stats.Open_flows,
			"total_flows":                   stats.Handle.Flow_stats.Total_flows,
			"num_flow_searches":             stats.Handle.Flow_stats.Num_flow_searches,
			"num_flow_search_misses":        stats.Handle.Flow_stats.Num_flow_search_misses,
			"num_flow_collisions":           stats.Handle.Flow_stats.Num_flow_collisions,
			"num_flow_structures":           stats.Handle.Flow_stats.Num_flow_structures,
			"peak_num_flow_structures":      stats.Handle.Flow_stats.Peak_num_flow_structures,
			"num_flows_missed_max_exceeded": stats.Handle.Flow_stats.Num_flows_missed_max_exceeded,
		}, nil
	// A DataHandle handle returns transfer stats specific to this handle
	case DataHandle:
		return map[string]int64{
			"read_calls":                  stats.Handle.Handle_stats.Read_calls,
			"read_calls_outstanding":      stats.Handle.Handle_stats.Read_calls_outstanding,
			"read_calls_completed":        stats.Handle.Handle_stats.Read_calls_completed,
			"read_calls_cancelled":        stats.Handle.Handle_stats.Read_calls_cancelled,
			"write_calls":                 stats.Handle.Handle_stats.Write_calls,
			"write_bytes":                 stats.Handle.Handle_stats.Write_bytes,
			"ioctl_calls":                 stats.Handle.Handle_stats.Ioctl_calls,
			"packets_processed_transport": stats.Handle.Transport_stats.Packets_processed,
			"read_packets_skipped":        stats.Handle.Transport_stats.Read_packets_skipped,
			"packets_reported":            stats.Handle.Transport_stats.Packets_reported,
		}, nil
	default:
		return nil, fmt.Errorf("no matching handle type for pulling handle stats")
	}
}
