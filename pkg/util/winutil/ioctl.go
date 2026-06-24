// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package winutil

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// SynchronousOverlappedDeviceIoControl issues an IOCTL on a handle opened
// with FILE_FLAG_OVERLAPPED and blocks until completion, returning the byte
// count and final status as a synchronous call would.
//
// The handle must be an overlapped handle. Callers with non-overlapped
// handles should use windows.DeviceIoControl directly.
//
// # Why this wrapper exists
//
// For FILE_FLAG_OVERLAPPED handles, Win32's DeviceIoControl must be called
// with a valid OVERLAPPED containing an event. Passing lpOverlapped=NULL is
// outside the documented contract and has been observed to hang indefinitely
// when the handle is also associated with an I/O completion port (IOCP).
// In WINA-2669, the stuck thread was waiting inside DeviceIoControl after
// issuing an IOCTL this way on an overlapped+IOCP handle; using a valid
// private OVERLAPPED avoids that unsafe path.
//
// To provide synchronous caller semantics safely, this wrapper issues the
// IOCTL as an overlapped request using a private manual-reset event. The low
// bit of OVERLAPPED.HEvent is set so this IOCTL's completion is NOT queued to
// the handle's IOCP -- callers that read from such handles via
// GetQueuedCompletionStatus assume every completion is one of their own
// queued operations and could corrupt or crash on an IOCTL completion. The
// wrapper waits on the raw (untagged) event handle and then calls
// GetOverlappedResult(bWait=false) for the final status and byte count. The
// bit-tagged HEvent isn't the raw wait handle; do the wait explicitly rather
// than rely on a Win32 wait API masking the IOCP-suppression bit.
//
// # Return values
//
// Some IOCTLs return ERROR_MORE_DATA after filling part of the output buffer.
// When Windows reports a byte count, this wrapper returns it alongside the
// error so callers can inspect both.
//
// See: https://learn.microsoft.com/en-us/windows/win32/api/ioapiset/nf-ioapiset-getqueuedcompletionstatus
//
//	(lpOverlapped parameter: "A valid event handle whose low-order bit is set
//	prevents the completion of the overlapped I/O from enqueing a completion
//	packet to the completion port.")
func SynchronousOverlappedDeviceIoControl(h windows.Handle, ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32) (uint32, error) {
	ev, err := windows.CreateEvent(nil, 1, 0, nil) // manual-reset, non-signaled
	if err != nil {
		return 0, fmt.Errorf("CreateEvent for SynchronousOverlappedDeviceIoControl: %w", err)
	}
	defer windows.CloseHandle(ev)

	var ol windows.Overlapped
	// Low bit on HEvent suppresses IOCP notification for this IRP (see doc above).
	ol.HEvent = windows.Handle(uintptr(ev) | 1)

	// Use a separate local for any byte count Windows writes on the initiation
	// path; the value returned to the caller is taken from GetOverlappedResult
	// on the async-completion path. Keeping the two variables separate avoids
	// the classic overlapped-I/O race where the initiation thread and the
	// completion path both write the same DWORD (Raymond Chen, "Why you
	// should never use the same byte-count variable for the initiation and
	// completion of an overlapped I/O").
	var inlineBytes uint32
	err = windows.DeviceIoControl(h, ioControlCode, inBuffer, inBufferSize, outBuffer, outBufferSize, &inlineBytes, &ol)

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
		return 0, fmt.Errorf("SynchronousOverlappedDeviceIoControl wait: %w", werr)
	}
	if status != windows.WAIT_OBJECT_0 {
		return 0, fmt.Errorf("SynchronousOverlappedDeviceIoControl: unexpected wait status %#x", status)
	}
	var bytesReturned uint32
	if gerr := windows.GetOverlappedResult(h, &ol, &bytesReturned, false); gerr != nil {
		return bytesReturned, gerr
	}
	return bytesReturned, nil
}
