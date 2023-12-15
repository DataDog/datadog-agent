// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package dns

/*
#include <stdlib.h>
#include <memory.h>
*/
import "C"
import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

const (
	dnsReadBufferCount = 100
)

// This is the type that an overlapped read returns -- the overlapped object, which must be passed back to the kernel after reading
// followed by a predictably sized chunk of bytes
type readbuffer struct {
	ol windows.Overlapped

	// This is the MTU of IPv6, which effectively governs the maximum DNS packet size over IPv6
	// see https://tools.ietf.org/id/draft-madi-dnsop-udp4dns-00.html
	data [1500]byte
}

type dnsDriver struct {
	h           driver.Handle
	readBuffers []*readbuffer
	iocp        windows.Handle
}

func newDriver() (*dnsDriver, error) {
	d := &dnsDriver{}
	err := d.setupDNSHandle()
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (d *dnsDriver) setupDNSHandle() error {
	var err error
	d.h, err = driver.NewHandle(windows.FILE_FLAG_OVERLAPPED, driver.DataHandle)
	if err != nil {
		return err
	}

	filters, err := createDNSFilters()
	if err != nil {
		return err
	}

	if err := d.SetDataFilters(filters); err != nil {
		return err
	}

	iocp, buffers, err := prepareCompletionBuffers(d.h.GetWindowsHandle(), dnsReadBufferCount)
	if err != nil {
		return err
	}

	d.iocp = iocp
	d.readBuffers = buffers

	return nil
}

// SetDataFilters installs the provided filters for data
func (d *dnsDriver) SetDataFilters(filters []driver.FilterDefinition) error {
	var id int64
	for _, filter := range filters {
		err := d.h.DeviceIoControl(
			driver.SetDataFilterIOCTL,
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

// ReadDNSPacket visits a raw DNS packet if one is available.
func (d *dnsDriver) ReadDNSPacket(visit func([]byte, time.Time) error) (didRead bool, err error) {
	var bytesRead uint32
	var key uintptr // returned by GetQueuedCompletionStatus, then ignored
	var ol *windows.Overlapped

	// NOTE: ideally we would pass a timeout of INFINITY to the GetQueuedCompletionStatus, but are using a
	// timeout of 0 and letting userspace do a busy loop to align better with the Linux code.
	err = windows.GetQueuedCompletionStatus(d.iocp, &bytesRead, &key, &ol, 0)
	if err != nil {
		if err == syscall.Errno(syscall.WAIT_TIMEOUT) {
			// this indicates that there was no queued completion status, this is fine
			return false, nil
		}

		return false, errors.Wrap(err, "could not get queued completion status")
	}

	//nolint:gosimple // TODO(WKIT) Fix gosimple linter
	var buf *readbuffer
	buf = (*readbuffer)(unsafe.Pointer(ol))

	fph := (*driver.FilterPacketHeader)(unsafe.Pointer(&buf.data[0]))
	captureTime := time.Unix(0, int64(fph.Timestamp))

	start := driver.FilterPacketHeaderSize

	if err := visit(buf.data[start:], captureTime); err != nil {
		return false, err
	}

	// kick off another read
	if err := d.h.ReadFile(buf.data[:], nil, &(buf.ol)); err != nil && err != windows.ERROR_IO_PENDING {
		return false, err
	}

	return true, nil
}

func (d *dnsDriver) Close() error {
	// destroy io completion port, and file
	if err := d.h.CancelIoEx(nil); err != nil {
		return fmt.Errorf("error cancelling DNS io completion: %w", err)
	}
	if err := windows.CloseHandle(d.iocp); err != nil {
		return fmt.Errorf("error closing DNS io completion handle: %w", err)
	}
	if err := d.h.Close(); err != nil {
		return fmt.Errorf("error closing driver DNS h: %w", err)
	}
	for _, buf := range d.readBuffers {
		C.free(unsafe.Pointer(buf))
	}
	d.readBuffers = nil
	return nil
}

func createDNSFilters() ([]driver.FilterDefinition, error) {
	var filters []driver.FilterDefinition

	filters = append(filters, driver.FilterDefinition{
		FilterVersion:  driver.Signature,
		Size:           driver.FilterDefinitionSize,
		FilterLayer:    driver.LayerTransport,
		Af:             windows.AF_INET,
		RemotePort:     53,
		InterfaceIndex: uint64(0),
		Direction:      driver.DirectionOutbound,
	})

	filters = append(filters, driver.FilterDefinition{
		FilterVersion:  driver.Signature,
		Size:           driver.FilterDefinitionSize,
		FilterLayer:    driver.LayerTransport,
		Af:             windows.AF_INET,
		RemotePort:     53,
		InterfaceIndex: uint64(0),
		Direction:      driver.DirectionInbound,
	})

	return filters, nil
}

// prepare N read buffers
// and return the IoCompletionPort that will be used to coordinate reads.
// danger: even though all reads will reference the returned iocp, buffers must be in-scope as long
// as reads are happening. Otherwise, the memory the kernel is writing to will be written to memory reclaimed
// by the GC
func prepareCompletionBuffers(h windows.Handle, count int) (iocp windows.Handle, buffers []*readbuffer, err error) {
	iocp, err = windows.CreateIoCompletionPort(h, windows.Handle(0), 0, 0)
	if err != nil {
		return windows.Handle(0), nil, errors.Wrap(err, "error creating IO completion port")
	}

	buffers = make([]*readbuffer, count)
	for i := 0; i < count; i++ {
		buf := (*readbuffer)(C.malloc(C.size_t(unsafe.Sizeof(readbuffer{}))))
		C.memset(unsafe.Pointer(buf), 0, C.size_t(unsafe.Sizeof(readbuffer{})))
		buffers[i] = buf

		err = windows.ReadFile(h, buf.data[:], nil, &(buf.ol))
		if err != nil && err != windows.ERROR_IO_PENDING {
			_ = windows.CloseHandle(iocp)
			return windows.Handle(0), nil, errors.Wrap(err, "failed to initiate readfile")
		}
	}

	return iocp, buffers, nil
}
