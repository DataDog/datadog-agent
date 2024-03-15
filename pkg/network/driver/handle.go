// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package driver

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	nettelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/telemetry"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// deviceName identifies the name and location of the windows driver
	deviceName       = `\\.\ddnpm`
	handleModuleName = "network_tracer__handle"
)

var (
	//nolint:revive // TODO(WKIT) Fix revive linter
	// Buffer holding datadog driver filterapi (ddnpmapi) signature to ensure consistency with driver.
	DdAPIVersionBuf = makeDDAPIVersionBuffer(Signature)
)

// Telemetry
//
//nolint:revive // TODO(WKIT) Fix revive linter
var HandleTelemetry = struct {
	numFlowCollisions     telemetry.Gauge
	newFlowsSkippedMax    telemetry.Gauge
	closedFlowsSkippedMax telemetry.Gauge

	numFlowStructs           telemetry.Gauge
	peakNumFlowStructs       telemetry.Gauge
	numFlowClosedStructs     telemetry.Gauge
	peakNumFlowClosedStructs telemetry.Gauge

	openTableAdds      telemetry.Gauge
	openTableRemoves   telemetry.Gauge
	closedTableAdds    telemetry.Gauge
	closedTableRemoves telemetry.Gauge

	noHandleFlows             telemetry.Gauge
	noHandleFlowsPeak         telemetry.Gauge
	numFlowsMissedMaxNoHandle telemetry.Gauge
	numPacketsAfterClosed     telemetry.Gauge

	classifyNoDirection       telemetry.Gauge
	classifyMultipleRequest   telemetry.Gauge
	classifyMultipleResponse  telemetry.Gauge
	classifyResponseNoRequest telemetry.Gauge

	noStateAtAleAuthConnect     telemetry.Gauge
	noStateAtAleAuthRecv        telemetry.Gauge
	noStateAtAleflowEstablished telemetry.Gauge
	noStateAtAleEndpointClosure telemetry.Gauge
	noStateAtInboundTransport   telemetry.Gauge
	noStateAtOutboundTransport  telemetry.Gauge

	httpTxnsCaptured      telemetry.Gauge
	httpTxnsSkippedMax    telemetry.Gauge
	httpNdisNonContiguous telemetry.Gauge
	flowsIgnoredAsEtw     telemetry.Gauge
	httpTxnNoLatency      telemetry.Gauge
	httpTxnBatchedOnRead  telemetry.Gauge

	ReadPacketsSkipped *nettelemetry.StatGaugeWrapper
	readsRequested     telemetry.Gauge
	readsCompleted     telemetry.Gauge
	readsCancelled     telemetry.Gauge
}{
	telemetry.NewGauge(handleModuleName, "num_flow_collisions", []string{}, "Gauge measuring the number of flow collisions"),
	telemetry.NewGauge(handleModuleName, "new_flows_skipped_max", []string{}, "Gauge measuring the maximum number of new flows skipped"),
	telemetry.NewGauge(handleModuleName, "closed_flows_skipped_max", []string{}, "Gauge measuring the maximum number of closed flows skipped"),

	telemetry.NewGauge(handleModuleName, "num_flow_structs", []string{}, "Gauge measuring the number of flow structs"),
	telemetry.NewGauge(handleModuleName, "peak_num_flow_structs", []string{}, "Gauge measuring the peak number of flow structs"),
	telemetry.NewGauge(handleModuleName, "num_flow_closed_structs", []string{}, "Gauge measuring the number of closed flow structs"),
	telemetry.NewGauge(handleModuleName, "peak_num_flow_closed_structs", []string{}, "Gauge measuring the peak number of closed flow structs"),

	telemetry.NewGauge(handleModuleName, "open_table_adds", []string{}, "Gauge measuring the number of additions to the open table"),
	telemetry.NewGauge(handleModuleName, "open_table_removes", []string{}, "Gauge measuring the number of removals from the open table"),
	telemetry.NewGauge(handleModuleName, "closed_table_adds", []string{}, "Gauge measuring the number of additions to the closed table"),
	telemetry.NewGauge(handleModuleName, "closed_table_removes", []string{}, "Gauge measuring the number of removals from the closed table"),

	telemetry.NewGauge(handleModuleName, "no_handle_flows", []string{}, "Gauge measuring the number of no handle flows"),
	telemetry.NewGauge(handleModuleName, "no_handle_flows_peak", []string{}, "Gauge measuring the peak number of no handle flows"),
	telemetry.NewGauge(handleModuleName, "num_flows_missed_max_no_handle", []string{}, "Gauge measuring the max number of no handle missed flows"),
	telemetry.NewGauge(handleModuleName, "num_packets_after_closed", []string{}, "Gauge measuring the number of packets after close"),

	telemetry.NewGauge(handleModuleName, "classify_no_direction", []string{}, "Gauge measuring the number of no direction flows"),
	telemetry.NewGauge(handleModuleName, "classify_multiple_request", []string{}, "Gauge measuring the number of multiple request flows"),
	telemetry.NewGauge(handleModuleName, "classify_multiple_response", []string{}, "Gauge measuring the number of multiple response flows"),
	telemetry.NewGauge(handleModuleName, "classify_response_no_request", []string{}, "Gauge measuring the number of no request flows"),

	telemetry.NewGauge(handleModuleName, "no_state_at_ale_auth_connect", []string{}, "Gauge measuring the number of no request flows"),
	telemetry.NewGauge(handleModuleName, "no_state_at_ale_auth_recv", []string{}, "Gauge measuring the number of no request flows"),
	telemetry.NewGauge(handleModuleName, "no_state_at_ale_flow_established", []string{}, "Gauge measuring the number of no request flows"),
	telemetry.NewGauge(handleModuleName, "no_state_at_ale_endpoint_closure", []string{}, "Gauge measuring the number of no request flows"),
	telemetry.NewGauge(handleModuleName, "no_state_at_inbound_transport", []string{}, "Gauge measuring the number of no request flows"),
	telemetry.NewGauge(handleModuleName, "no_state_at_outbound_transport", []string{}, "Gauge measuring the number of no request flows"),

	telemetry.NewGauge(handleModuleName, "http_txns_captured", []string{}, "Gauge measuring the number of http transactions captured"),
	telemetry.NewGauge(handleModuleName, "http_txns_skipped_max", []string{}, "Gauge measuring the max number of http transactions skipped"),
	telemetry.NewGauge(handleModuleName, "http_ndis_non_contiguous", []string{}, "Gauge measuring the number of non contiguous http ndis"),
	telemetry.NewGauge(handleModuleName, "flows_ignored_as_etw", []string{}, "Gauge measuring the number of flows ignored as etw"),
	telemetry.NewGauge(handleModuleName, "txn_zero_latency", []string{}, "Gauge measuring number of http transactions computed zero latency"),
	telemetry.NewGauge(handleModuleName, "txn_batched_on_read", []string{}, "Gauge measuring number of http transactions computed zero latency"),

	nettelemetry.NewStatGaugeWrapper(handleModuleName, "read_packets_skipped", []string{}, "Gauge measuring the number of read packets skipped"),
	telemetry.NewGauge(handleModuleName, "reads_requested", []string{}, "Gauge measuring the number of reads requested"),
	telemetry.NewGauge(handleModuleName, "reads_completed", []string{}, "Gauge measuring the number of reads completed"),
	telemetry.NewGauge(handleModuleName, "reads_cancelled", []string{}, "Gauge measuring the number of reads_cancelled"),
}

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

//nolint:revive // TODO(WKIT) Fix revive linter
type Handle interface {
	ReadFile(p []byte, bytesRead *uint32, ol *windows.Overlapped) error
	DeviceIoControl(ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32, bytesReturned *uint32, overlapped *windows.Overlapped) (err error)
	CancelIoEx(ol *windows.Overlapped) error
	Close() error
	GetWindowsHandle() windows.Handle
	RefreshStats()
}

// Handle struct stores the windows handle for the driver as well as information about what type of filter is set
//
//nolint:revive // TODO(WKIT) Fix revive linter
type RealDriverHandle struct {
	Handle     windows.Handle
	handleType HandleType

	// record the last value of number of flows missed due to max exceeded
	lastNumFlowsMissed       uint64
	lastNumClosedFlowsMissed uint64
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (dh *RealDriverHandle) GetWindowsHandle() windows.Handle {
	return dh.Handle
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (dh *RealDriverHandle) ReadFile(p []byte, bytesRead *uint32, ol *windows.Overlapped) error {
	return windows.ReadFile(dh.Handle, p, bytesRead, ol)
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (dh *RealDriverHandle) DeviceIoControl(ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32, bytesReturned *uint32, overlapped *windows.Overlapped) (err error) {
	return windows.DeviceIoControl(dh.Handle, ioControlCode, inBuffer, inBufferSize, outBuffer, outBufferSize, bytesReturned, overlapped)
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (dh *RealDriverHandle) CancelIoEx(ol *windows.Overlapped) error {
	return windows.CancelIoEx(dh.Handle, ol)
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

// RefreshStats refreshes the relevant stats depending on the handle type
func (dh *RealDriverHandle) RefreshStats() {
	var (
		bytesReturned uint32
		statbuf       = make([]byte, StatsSize)
	)

	err := dh.DeviceIoControl(GetStatsIOCTL, &DdAPIVersionBuf[0], uint32(len(DdAPIVersionBuf)), &statbuf[0], uint32(len(statbuf)), &bytesReturned, nil)
	if err != nil {
		log.Errorf("failed to read driver stats for filter type %v - returned error %v", dh.handleType, err)
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

		HandleTelemetry.numFlowCollisions.Set(float64(stats.Flow_stats.Num_flow_collisions))
		HandleTelemetry.newFlowsSkippedMax.Set(float64(stats.Flow_stats.Num_flow_alloc_skipped_max_open_exceeded))
		HandleTelemetry.closedFlowsSkippedMax.Set(float64(stats.Flow_stats.Num_flow_closed_dropped_max_exceeded))

		HandleTelemetry.numFlowStructs.Set(float64(stats.Flow_stats.Num_flow_structures))
		HandleTelemetry.peakNumFlowStructs.Set(float64(stats.Flow_stats.Peak_num_flow_structures))
		HandleTelemetry.numFlowClosedStructs.Set(float64(stats.Flow_stats.Num_flow_closed_structures))
		HandleTelemetry.peakNumFlowStructs.Set(float64(stats.Flow_stats.Peak_num_flow_closed_structures))

		HandleTelemetry.openTableAdds.Set(float64(stats.Flow_stats.Open_table_adds))
		HandleTelemetry.openTableRemoves.Set(float64(stats.Flow_stats.Open_table_removes))
		HandleTelemetry.closedTableAdds.Set(float64(stats.Flow_stats.Closed_table_adds))
		HandleTelemetry.closedTableRemoves.Set(float64(stats.Flow_stats.Closed_table_removes))

		HandleTelemetry.noHandleFlows.Set(float64(stats.Flow_stats.Num_flows_no_handle))
		HandleTelemetry.noHandleFlowsPeak.Set(float64(stats.Flow_stats.Peak_num_flows_no_handle))
		HandleTelemetry.numFlowsMissedMaxNoHandle.Set(float64(stats.Flow_stats.Num_flows_missed_max_no_handle_exceeded))
		HandleTelemetry.numPacketsAfterClosed.Set(float64(stats.Flow_stats.Num_packets_after_flow_closed))

		HandleTelemetry.classifyNoDirection.Set(float64(stats.Flow_stats.Classify_with_no_direction))
		HandleTelemetry.classifyMultipleRequest.Set(float64(stats.Flow_stats.Classify_multiple_request))
		HandleTelemetry.classifyMultipleResponse.Set(float64(stats.Flow_stats.Classify_multiple_response))
		HandleTelemetry.classifyResponseNoRequest.Set(float64(stats.Flow_stats.Classify_response_no_request))

		HandleTelemetry.noStateAtAleAuthConnect.Set(float64(stats.Flow_stats.No_state_at_ale_auth_connect))
		HandleTelemetry.noStateAtAleAuthRecv.Set(float64(stats.Flow_stats.No_state_at_ale_auth_recv))
		HandleTelemetry.noStateAtAleflowEstablished.Set(float64(stats.Flow_stats.No_state_at_ale_flow_established))
		HandleTelemetry.noStateAtAleEndpointClosure.Set(float64(stats.Flow_stats.No_state_at_ale_endpoint_closure))
		HandleTelemetry.noStateAtInboundTransport.Set(float64(stats.Flow_stats.No_state_at_inbound_transport))
		HandleTelemetry.noStateAtOutboundTransport.Set(float64(stats.Flow_stats.No_state_at_outbound_transport))

		HandleTelemetry.httpTxnsCaptured.Set(float64(stats.Http_stats.Txns_captured))
		HandleTelemetry.httpTxnsSkippedMax.Set(float64(stats.Http_stats.Txns_skipped_max_exceeded))
		HandleTelemetry.httpNdisNonContiguous.Set(float64(stats.Http_stats.Ndis_buffer_non_contiguous))
		HandleTelemetry.flowsIgnoredAsEtw.Set(float64(stats.Http_stats.Flows_ignored_as_etw))
		HandleTelemetry.httpTxnNoLatency.Set(float64(stats.Http_stats.Txn_zero_latency))
		HandleTelemetry.httpTxnBatchedOnRead.Set(float64(stats.Http_stats.Txn_batched_on_read))

	// A DataHandle handle returns transfer stats specific to this handle
	case DataHandle:
		HandleTelemetry.ReadPacketsSkipped.Set(stats.Transport_stats.Packets_skipped)
		HandleTelemetry.readsRequested.Set(float64(stats.Transport_stats.Calls_requested))
		HandleTelemetry.readsCompleted.Set(float64(stats.Transport_stats.Calls_completed))
		HandleTelemetry.readsCancelled.Set(float64(stats.Transport_stats.Calls_cancelled))
	default:
		log.Errorf("no matching handle type for pulling handle stats")
	}
}
