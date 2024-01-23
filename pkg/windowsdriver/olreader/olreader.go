// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package olreader

// the olreader (OverlappedReader) provides a generic interface for
// doing overlapped reads from a particular handle.  The handle is assumed
// to be a DataDog driver handle.

/*
#include <stdlib.h>
#include <memory.h>
*/
import "C"
import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows"
)

const (
	// ERROR_INVALID_PARAMETER this error isn't in the windows package for some reason
	ERROR_INVALID_PARAMETER windows.Errno = 87 //nolint:revive // use windows error naming convention
)

// This is the type that an overlapped read returns -- the overlapped object, which must be passed back to the kernel after reading
// followed by a predictably sized chunk of bytes
type readbuffer struct {
	// ol _must_ be first, because it is the pointer returned from the overlapped
	// operation and it's used to cast to the entire structure.
	ol windows.Overlapped

	buffersize int
	data       []uint8
}

// OverlappedCallbackFunc is called every time a read completes.
// if err is not nil, it will be set to
//
//nolint:revive // TODO(WKIT) Fix revive linter
type OverlappedCallback interface {
	OnData([]uint8)
	OnError(err error)
}

// OverlappedReader is the manager object for doing overlapped reads
// for a particular handle
//
//nolint:revive // TODO(WKIT) Fix revive linter
type OverlappedReader struct {
	h       windows.Handle
	iocp    windows.Handle
	bufsz   int
	count   int
	cb      OverlappedCallback
	wg      sync.WaitGroup
	buffers []*readbuffer
}

//nolint:revive // TODO(WKIT) Fix revive linter
func NewOverlappedReader(cbfn OverlappedCallback, bufsz, count int) (*OverlappedReader, error) {
	olr := &OverlappedReader{
		cb:    cbfn,
		bufsz: bufsz,
		count: count,
	}

	return olr, nil
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (olr *OverlappedReader) Open(name string) error {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return fmt.Errorf("Failed to create device name %v", err)
	}
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OVERLAPPED,
		windows.Handle(0))
	if err != nil {
		return fmt.Errorf("Failed to open handle to %s %v", name, err)
	}
	iocp, err := windows.CreateIoCompletionPort(h, windows.Handle(0), 0, 0)
	if err != nil {
		windows.CloseHandle(h)
		return fmt.Errorf("error creating IO completion port %v", err)
	}
	olr.h = h
	olr.iocp = iocp
	return nil
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (olr *OverlappedReader) Read() error {
	if err := olr.createBuffers(); err != nil {
		return fmt.Errorf("Failed to create overlapped read buffers")
	}
	if err := olr.initiateReads(); err != nil {
		return err
	}
	olr.wg.Add(1)
	go func() {
		defer olr.wg.Done()

		for {
			var bytesRead uint32
			var key uintptr
			var ol *windows.Overlapped

			err := windows.GetQueuedCompletionStatus(olr.iocp, &bytesRead, &key, &ol, windows.INFINITE)
			if err != nil {
				if err == syscall.Errno(syscall.WAIT_TIMEOUT) {
					// this indicates that there was no queued completion status, this is fine
					continue
				}
				if err == syscall.Errno(ERROR_INVALID_PARAMETER) {
					// this should never occur. It means that we supplied a buffer
					// too short for even the structure header.
					olr.cb.OnError(err)
					log.Error("Got ERROR_INVALID_PARAMETER, exiting oldreader event loop")
					return
				}
				if err != syscall.Errno(syscall.ERROR_INSUFFICIENT_BUFFER) {
					// if we get this error, there will still be at least
					// the structure header.  In any other case, fail out
					olr.cb.OnError(err)
					log.Errorf("Got unexpected error %v, exiting oldreader event loop", err)
					return

				}
			}
			if ol == nil {
				// the completion port was closed.  time to go home
				log.Infof("event loop completion port closed; exiting read loop")
				return
			}

			buf := (*readbuffer)(unsafe.Pointer(ol))
			data := buf.data[:bytesRead]

			olr.cb.OnData(data)

			// re-initiate the read
			// kick off another read
			if err := windows.ReadFile(olr.h, buf.data[:], nil, &(buf.ol)); err != nil && err != windows.ERROR_IO_PENDING {
				olr.cb.OnError(err)
				log.Warnf("error reinitiating overlapped read %v", err)
			}
		}
	}()
	return nil
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (olr *OverlappedReader) Stop() {
	_ = windows.CloseHandle(olr.iocp)
	_ = windows.CloseHandle(olr.h)
	olr.wg.Wait()
	olr.cleanBuffers()
}

// Ioctl passes an ioctl() through to the underlying handle
func (olr *OverlappedReader) Ioctl(ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32, bytesReturned *uint32, overlapped *windows.Overlapped) (err error) {
	return windows.DeviceIoControl(olr.h, ioControlCode, inBuffer, inBufferSize, outBuffer, outBufferSize, bytesReturned, overlapped)
}
func (olr *OverlappedReader) initiateReads() error {
	for _, buf := range olr.buffers {
		if buf == nil {
			// would only happen if `createbuffers` not called, or
			// cleanbuffers was called.  But ensure pointer is valid
			return fmt.Errorf("Invalid buffer for read")
		}
		/*
		 * because this is an overlapped read, this will return ERROR_IO_PENDING
		 * even if we provide a buffer that's too small.  The initial
		 * GetQueuedCompletionStatus() will return with the error if it's
		 * too small
		 */
		err := windows.ReadFile(olr.h, buf.data[:], nil, &buf.ol)
		if err != nil && err != windows.ERROR_IO_PENDING {
			return fmt.Errorf("Failed to initiate read %v", err)
		}
	}
	return nil
}
func (olr *OverlappedReader) createBuffers() error {
	olr.buffers = make([]*readbuffer, olr.count)
	structsize := C.size_t(unsafe.Sizeof(readbuffer{}))
	totalsize := C.size_t(olr.bufsz) + structsize

	for i := 0; i < olr.count; i++ {
		buf := (*readbuffer)(C.malloc(totalsize))
		C.memset(unsafe.Pointer(buf), 0, C.size_t(unsafe.Sizeof(readbuffer{})))

		bufpointer := unsafe.Add(unsafe.Pointer(buf), structsize)
		buf.data = unsafe.Slice((*uint8)(bufpointer), olr.bufsz)
		buf.buffersize = olr.bufsz
		olr.buffers[i] = buf
	}
	return nil
}

func (olr *OverlappedReader) cleanBuffers() {
	for idx, buf := range olr.buffers {
		C.free(unsafe.Pointer(buf)) //nolint:govet
		olr.buffers[idx] = nil
	}
}
