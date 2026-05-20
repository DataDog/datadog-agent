// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package driver

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	telemetryComp "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
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
	numFlowCollisions     telemetryComp.Gauge
	newFlowsSkippedMax    telemetryComp.Gauge
	closedFlowsSkippedMax telemetryComp.Gauge

	numFlowStructs           telemetryComp.Gauge
	peakNumFlowStructs       telemetryComp.Gauge
	numFlowClosedStructs     telemetryComp.Gauge
	peakNumFlowClosedStructs telemetryComp.Gauge

	openTableAdds      telemetryComp.Gauge
	openTableRemoves   telemetryComp.Gauge
	closedTableAdds    telemetryComp.Gauge
	closedTableRemoves telemetryComp.Gauge

	noHandleFlows             telemetryComp.Gauge
	noHandleFlowsPeak         telemetryComp.Gauge
	numFlowsMissedMaxNoHandle telemetryComp.Gauge
	numPacketsAfterClosed     telemetryComp.Gauge

	classifyNoDirection       telemetryComp.Gauge
	classifyMultipleRequest   telemetryComp.Gauge
	classifyMultipleResponse  telemetryComp.Gauge
	classifyResponseNoRequest telemetryComp.Gauge

	noStateAtAleAuthConnect     telemetryComp.Gauge
	noStateAtAleAuthRecv        telemetryComp.Gauge
	noStateAtAleflowEstablished telemetryComp.Gauge
	noStateAtAleEndpointClosure telemetryComp.Gauge
	noStateAtInboundTransport   telemetryComp.Gauge
	noStateAtOutboundTransport  telemetryComp.Gauge

	httpTxnsCaptured      telemetryComp.Gauge
	httpTxnsSkippedMax    telemetryComp.Gauge
	httpNdisNonContiguous telemetryComp.Gauge
	flowsIgnoredAsEtw     telemetryComp.Gauge
	httpTxnNoLatency      telemetryComp.Gauge
	httpTxnBatchedOnRead  telemetryComp.Gauge

	ReadPacketsSkipped *telemetryComp.StatGaugeWrapper
	readsRequested     telemetryComp.Gauge
	readsCompleted     telemetryComp.Gauge
	readsCancelled     telemetryComp.Gauge
}{
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "num_flow_collisions", []string{}, "Gauge measuring the number of flow collisions"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "new_flows_skipped_max", []string{}, "Gauge measuring the maximum number of new flows skipped"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "closed_flows_skipped_max", []string{}, "Gauge measuring the maximum number of closed flows skipped"),

	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "num_flow_structs", []string{}, "Gauge measuring the number of flow structs"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "peak_num_flow_structs", []string{}, "Gauge measuring the peak number of flow structs"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "num_flow_closed_structs", []string{}, "Gauge measuring the number of closed flow structs"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "peak_num_flow_closed_structs", []string{}, "Gauge measuring the peak number of closed flow structs"),

	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "open_table_adds", []string{}, "Gauge measuring the number of additions to the open table"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "open_table_removes", []string{}, "Gauge measuring the number of removals from the open table"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "closed_table_adds", []string{}, "Gauge measuring the number of additions to the closed table"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "closed_table_removes", []string{}, "Gauge measuring the number of removals from the closed table"),

	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "no_handle_flows", []string{}, "Gauge measuring the number of no handle flows"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "no_handle_flows_peak", []string{}, "Gauge measuring the peak number of no handle flows"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "num_flows_missed_max_no_handle", []string{}, "Gauge measuring the max number of no handle missed flows"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "num_packets_after_closed", []string{}, "Gauge measuring the number of packets after close"),

	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "classify_no_direction", []string{}, "Gauge measuring the number of no direction flows"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "classify_multiple_request", []string{}, "Gauge measuring the number of multiple request flows"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "classify_multiple_response", []string{}, "Gauge measuring the number of multiple response flows"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "classify_response_no_request", []string{}, "Gauge measuring the number of no request flows"),

	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "no_state_at_ale_auth_connect", []string{}, "Gauge measuring the number of no request flows"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "no_state_at_ale_auth_recv", []string{}, "Gauge measuring the number of no request flows"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "no_state_at_ale_flow_established", []string{}, "Gauge measuring the number of no request flows"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "no_state_at_ale_endpoint_closure", []string{}, "Gauge measuring the number of no request flows"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "no_state_at_inbound_transport", []string{}, "Gauge measuring the number of no request flows"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "no_state_at_outbound_transport", []string{}, "Gauge measuring the number of no request flows"),

	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "http_txns_captured", []string{}, "Gauge measuring the number of http transactions captured"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "http_txns_skipped_max", []string{}, "Gauge measuring the max number of http transactions skipped"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "http_ndis_non_contiguous", []string{}, "Gauge measuring the number of non contiguous http ndis"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "flows_ignored_as_etw", []string{}, "Gauge measuring the number of flows ignored as etw"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "txn_zero_latency", []string{}, "Gauge measuring number of http transactions computed zero latency"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "txn_batched_on_read", []string{}, "Gauge measuring number of http transactions computed zero latency"),

	telemetryComp.NewStatGaugeWrapper(telemetryimpl.GetCompatComponent(), handleModuleName, "read_packets_skipped", []string{}, "Gauge measuring the number of read packets skipped"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "reads_requested", []string{}, "Gauge measuring the number of reads requested"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "reads_completed", []string{}, "Gauge measuring the number of reads completed"),
	telemetryimpl.GetCompatComponent().NewGauge(handleModuleName, "reads_cancelled", []string{}, "Gauge measuring the number of reads_cancelled"),
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
	SynchronousDeviceIoControl(ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32) (bytesReturned uint32, err error)
	// Deprecated: use SynchronousDeviceIoControl. This shim is kept temporarily
	// for external consumers (github.com/DataDog/datadog-traceroute) that have
	// not yet migrated. Will be removed once those consumers update.
	DeviceIoControl(ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32, bytesReturned *uint32, overlapped *windows.Overlapped) error
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
	// overlapped is true when Handle was opened with FILE_FLAG_OVERLAPPED.
	// SynchronousDeviceIoControl branches on this to use the correct Win32
	// invocation: synchronous handles take the simple sync path; overlapped
	// handles need a synthesized OVERLAPPED + private event + GetOverlappedResult
	// to avoid the IOCP-routing hang documented on SynchronousDeviceIoControl.
	overlapped bool

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

// SynchronousDeviceIoControl issues a synchronous IOCTL on the underlying
// handle, branching internally based on whether the handle was opened with
// FILE_FLAG_OVERLAPPED.
//
// On the returned values: some IOCTLs return ERROR_MORE_DATA after filling
// part of the output buffer. When Windows reports a byte count, this wrapper
// returns it alongside the error so callers can inspect both.
//
// # Synchronous handles
//
// For handles opened without FILE_FLAG_OVERLAPPED, Win32's DeviceIoControl is
// naturally blocking. Per MSDN, lpBytesReturned must be non-NULL when
// lpOverlapped is NULL. The call is forwarded directly; bytesReturned is
// filled by Win32 even on partial-data errors.
//
// # Overlapped handles (and why this wrapper exists)
//
// For FILE_FLAG_OVERLAPPED handles, DeviceIoControl must be called with a
// valid OVERLAPPED containing an event. Passing lpOverlapped=NULL is outside
// the documented contract and has been observed to hang indefinitely when
// the handle is also associated with an IOCP -- asynchronous completions on
// IOCP-associated handles are intended to be consumed via the completion
// port, not via a direct wait on the handle. This is what produced the
// WINA-2669 datadog-system-probe shutdown hang: with Driver Verifier enabled
// on ddprocmon.sys, the STOP IOCTL on the overlapped+IOCP procmon handle was
// issued with lpOverlapped=NULL; the IRP's completion was routed through the
// IOCP and Win32's fallback wait on the file handle never returned. Only
// process termination released it.
//
// To provide synchronous caller semantics safely, this wrapper issues the
// IOCTL as an overlapped request using a private manual-reset event. The low
// bit of OVERLAPPED.HEvent is set so this IOCTL's completion is NOT queued
// to the handle's IOCP -- the IOCP read loops on these handles
// (pkg/network/dns ReadDNSPacket, pkg/windowsdriver/olreader
// OverlappedReader.Read) assume every completion is a *readbuffer and would
// corrupt or crash on an IOCTL completion. The wrapper waits on the raw
// (untagged) event handle and then calls GetOverlappedResult(bWait=false)
// for the final status and byte count. The bit-tagged HEvent isn't the raw
// wait handle; do the wait explicitly rather than rely on a Win32 wait API
// masking the IOCP-suppression bit.
//
// See: https://learn.microsoft.com/en-us/windows/win32/api/ioapiset/nf-ioapiset-getqueuedcompletionstatus
//
//	(lpOverlapped parameter: "A valid event handle whose low-order bit is set
//	prevents the completion of the overlapped I/O from enqueing a completion
//	packet to the completion port.")
//
//nolint:revive // TODO(WKIT) Fix revive linter
func (dh *RealDriverHandle) SynchronousDeviceIoControl(ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32) (bytesReturned uint32, err error) {
	if !dh.overlapped {
		// Native sync path. lpBytesReturned must be non-NULL when lpOverlapped
		// is NULL. bytesReturned is filled even on errors like ERROR_MORE_DATA.
		err = windows.DeviceIoControl(dh.Handle, ioControlCode, inBuffer, inBufferSize, outBuffer, outBufferSize, &bytesReturned, nil)
		return bytesReturned, err
	}

	ev, err := windows.CreateEvent(nil, 1, 0, nil) // manual-reset, non-signaled
	if err != nil {
		return 0, fmt.Errorf("CreateEvent for SynchronousDeviceIoControl: %w", err)
	}
	defer windows.CloseHandle(ev)

	var ol windows.Overlapped
	// Low bit on HEvent suppresses IOCP notification for this IRP (see doc above).
	ol.HEvent = windows.Handle(uintptr(ev) | 1)

	// Use a separate local for any byte count Windows writes on the initiation
	// path; the named return value `bytesReturned` is reserved for
	// GetOverlappedResult on the async-completion path. Keeping the two
	// variables separate avoids the classic overlapped-I/O race where the
	// initiation thread and the completion path both write the same DWORD
	// (Raymond Chen, "Why you should never use the same byte-count variable
	// for the initiation and completion of an overlapped I/O").
	var inlineBytes uint32
	err = windows.DeviceIoControl(dh.Handle, ioControlCode, inBuffer, inBufferSize, outBuffer, outBufferSize, &inlineBytes, &ol)

	// Per MSDN, when lpOverlapped is non-NULL the lpBytesReturned value is
	// "meaningless until the overlapped operation has completed". On any
	// inline return from DeviceIoControl (success or non-pending error) the
	// IRP IS complete, so inlineBytes is meaningful. Only ERROR_IO_PENDING
	// requires deferring to GetOverlappedResult.
	if !errors.Is(err, windows.ERROR_IO_PENDING) {
		return inlineBytes, err
	}

	// Async: IRP queued. Wait on the raw (untagged) event ourselves, then
	// collect the final status and byte count via GetOverlappedResult.
	status, werr := windows.WaitForSingleObject(ev, windows.INFINITE)
	if werr != nil {
		return 0, werr
	}
	if status != windows.WAIT_OBJECT_0 {
		return 0, fmt.Errorf("SynchronousDeviceIoControl: unexpected wait status %#x", status)
	}
	if gerr := windows.GetOverlappedResult(dh.Handle, &ol, &bytesReturned, false); gerr != nil {
		return bytesReturned, gerr
	}
	return bytesReturned, nil
}

// DeviceIoControl is a deprecated shim around SynchronousDeviceIoControl, kept
// for external consumers (github.com/DataDog/datadog-traceroute) that have not
// yet migrated to the new name. All current agent code should call
// SynchronousDeviceIoControl directly.
//
// The shim only supports the safe usage that callers actually used: a nil
// overlapped (synchronous semantics). A non-nil overlapped is rejected because
// no existing caller threaded their own OVERLAPPED through this method, and
// the new wrapper synthesizes one internally.
//
// Deprecated: use SynchronousDeviceIoControl.
//
//nolint:revive // TODO(WKIT) Fix revive linter
func (dh *RealDriverHandle) DeviceIoControl(ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32, bytesReturned *uint32, overlapped *windows.Overlapped) error {
	if overlapped != nil {
		return errors.New("driver: deprecated DeviceIoControl does not support a caller-supplied OVERLAPPED; use SynchronousDeviceIoControl")
	}
	n, err := dh.SynchronousDeviceIoControl(ioControlCode, inBuffer, inBufferSize, outBuffer, outBufferSize)
	if bytesReturned != nil {
		*bytesReturned = n
	}
	return err
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (dh *RealDriverHandle) CancelIoEx(ol *windows.Overlapped) error {
	return windows.CancelIoEx(dh.Handle, ol)
}

// NewHandle creates a new windows handle attached to the driver
func NewHandle(flags uint32, handleType HandleType, _ telemetryComp.Component) (Handle, error) {
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
	return &RealDriverHandle{
		Handle:     h,
		handleType: handleType,
		overlapped: flags&windows.FILE_FLAG_OVERLAPPED != 0,
	}, nil
}

// Close closes the underlying windows handle
func (dh *RealDriverHandle) Close() error {
	return windows.CloseHandle(dh.Handle)
}

// RefreshStats refreshes the relevant stats depending on the handle type
func (dh *RealDriverHandle) RefreshStats() {
	statbuf := make([]byte, StatsSize)

	_, err := dh.SynchronousDeviceIoControl(GetStatsIOCTL, &DdAPIVersionBuf[0], uint32(len(DdAPIVersionBuf)), &statbuf[0], uint32(len(statbuf)))
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
