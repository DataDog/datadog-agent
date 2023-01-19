// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package driver

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// deviceName identifies the name and location of the windows driver
	deviceName = `\\.\ddnpm`
)

var (
	// Buffer holding datadog driver filterapi (ddnpmapi) signature to ensure consistency with driver.
	DdAPIVersionBuf = makeDDAPIVersionBuffer(Signature)
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

// handleTypeToPathName maps the handle type to the path name that the driver is expecting.
var handleTypeToPathName = map[HandleType]string{
	FlowHandle:  "flowstatshandle",
	DataHandle:  "transporthandle",
	StatsHandle: "driverstatshandle", // for now just use that; any path will do
}

type Handle interface {
	ReadFile(p []byte, bytesRead *uint32, ol *windows.Overlapped) error
	DeviceIoControl(ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32, bytesReturned *uint32, overlapped *windows.Overlapped) (err error)
	CancelIoEx(ol *windows.Overlapped) error
	Close() error
	GetWindowsHandle() windows.Handle
	GetStatsForHandle() (map[string]map[string]int64, error)
}

// Handle struct stores the windows handle for the driver as well as information about what type of filter is set
type RealDriverHandle struct {
	Handle     windows.Handle
	handleType HandleType

	// record the last value of number of flows missed due to max exceeded
	lastNumFlowsMissed       uint64
	lastNumClosedFlowsMissed uint64
}

func (rdh *RealDriverHandle) GetWindowsHandle() windows.Handle {
	return rdh.Handle
}
func (rdh *RealDriverHandle) ReadFile(p []byte, bytesRead *uint32, ol *windows.Overlapped) error {
	return windows.ReadFile(rdh.Handle, p, bytesRead, ol)
}

func (rdh *RealDriverHandle) DeviceIoControl(ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32, bytesReturned *uint32, overlapped *windows.Overlapped) (err error) {
	return windows.DeviceIoControl(rdh.Handle, ioControlCode, inBuffer, inBufferSize, outBuffer, outBufferSize, bytesReturned, overlapped)
}

func (rdh *RealDriverHandle) CancelIoEx(ol *windows.Overlapped) error {
	return windows.CancelIoEx(rdh.Handle, ol)
}

// NewHandle creates a new windows handle attached to the driver
func NewHandle(flags uint32, handleType HandleType) (Handle, error) {
	pathext, ok := handleTypeToPathName[handleType]
	if !ok {
		return nil, fmt.Errorf("unknown Handle type %v", handleType)
	}
	fullpath := deviceName + `\` + pathext
	pFullPath, err := windows.UTF16PtrFromString(fullpath)
	if err != nil {
		return nil, err
	}
	log.Debugf("Creating driver handle of type %s", handleType)
	h, err := windows.CreateFile(pFullPath,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		flags,
		windows.Handle(0))
	if err != nil {
		log.Errorf("Error creating file handle %v", err)
		return nil, err
	}
	return &RealDriverHandle{Handle: h, handleType: handleType}, nil
}

// Close closes the underlying windows handle
func (dh *RealDriverHandle) Close() error {
	return windows.CloseHandle(dh.Handle)
}

// GetStatsForHandle gets the relevant stats depending on the handle type
func (dh *RealDriverHandle) GetStatsForHandle() (map[string]map[string]int64, error) {
	var (
		bytesReturned uint32
		statbuf       = make([]byte, StatsSize)
		returnmap     = make(map[string]map[string]int64)
	)

	err := dh.DeviceIoControl(GetStatsIOCTL, &DdAPIVersionBuf[0], uint32(len(DdAPIVersionBuf)), &statbuf[0], uint32(len(statbuf)), &bytesReturned, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read driver stats for filter type %v - returned error %v", dh.handleType, err)
	}
	stats := *(*Stats)(unsafe.Pointer(&statbuf[0]))

	switch dh.handleType {

	// A FlowHandle handle returns the flow stats specific to this handle
	case FlowHandle:
		if dh.lastNumFlowsMissed < uint64(stats.Flow_stats.Num_flow_alloc_skipped_max_open_exceeded) {
			log.Warnf("Open Flows missed due to maximum flow limit. %v", stats.Flow_stats.Num_flow_alloc_skipped_max_open_exceeded)
		}
		if dh.lastNumClosedFlowsMissed < uint64(stats.Flow_stats.Num_flow_closed_dropped_max_exceeded) {
			log.Warnf("Closed Flows dropped due to maximum flow limit. %v", stats.Flow_stats.Num_flow_closed_dropped_max_exceeded)
		}
		dh.lastNumFlowsMissed = uint64(stats.Flow_stats.Num_flow_alloc_skipped_max_open_exceeded)
		dh.lastNumClosedFlowsMissed = uint64(stats.Flow_stats.Num_flow_closed_dropped_max_exceeded)
		returnmap["handle"] = map[string]int64{
			"num_flow_collisions":      stats.Flow_stats.Num_flow_collisions,
			"new_flows_skipped_max":    stats.Flow_stats.Num_flow_alloc_skipped_max_open_exceeded,
			"closed_flows_skipped_max": stats.Flow_stats.Num_flow_closed_dropped_max_exceeded,

			"num_flow_structs":             stats.Flow_stats.Num_flow_structures,
			"peak_num_flow_structs":        stats.Flow_stats.Peak_num_flow_structures,
			"num_flow_closed_structs":      stats.Flow_stats.Num_flow_closed_structures,
			"peak_num_flow_closed_structs": stats.Flow_stats.Peak_num_flow_closed_structures,

			"open_table_adds":      stats.Flow_stats.Open_table_adds,
			"open_table_removes":   stats.Flow_stats.Open_table_removes,
			"closed_table_adds":    stats.Flow_stats.Closed_table_adds,
			"closed_table_removes": stats.Flow_stats.Closed_table_removes,

			"no_handle_flows":                stats.Flow_stats.Num_flows_no_handle,
			"no_handle_flows_peak":           stats.Flow_stats.Peak_num_flows_no_handle,
			"num_flows_missed_max_no_handle": stats.Flow_stats.Num_flows_missed_max_no_handle_exceeded,
			"num_packets_after_closed":       stats.Flow_stats.Num_packets_after_flow_closed,

			"classify_no_direction":        stats.Flow_stats.Classify_with_no_direction,
			"classify_multiple_request":    stats.Flow_stats.Classify_multiple_request,
			"classify_multiple_response":   stats.Flow_stats.Classify_multiple_response,
			"classify_response_no_request": stats.Flow_stats.Classify_response_no_request,

			"http_txns_captured":       stats.Http_stats.Txns_captured,
			"http_txns_skipped_max":    stats.Http_stats.Txns_skipped_max_exceeded,
			"http_ndis_non_contiguous": stats.Http_stats.Ndis_buffer_non_contiguous,
			"Flows_ignored_as_etw":     stats.Http_stats.Flows_ignored_as_etw,
		}
	// A DataHandle handle returns transfer stats specific to this handle
	case DataHandle:
		returnmap["handle"] = map[string]int64{
			"read_packets_skipped": stats.Transport_stats.Packets_skipped,
			"reads_requested":      stats.Transport_stats.Calls_requested,
			"reads_completed":      stats.Transport_stats.Calls_completed,
			"reads_cancelled":      stats.Transport_stats.Calls_cancelled,
		}
	default:
		return nil, fmt.Errorf("no matching handle type for pulling handle stats")
	}
	return returnmap, nil
}
