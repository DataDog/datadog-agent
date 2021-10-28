// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dns

/*
#include <stdlib.h>
#include <memory.h>
*/
import "C"
import (
	"fmt"
	"net"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"golang.org/x/sys/windows"
)

const (
	dnsReadBufferCount = 100
)

type dnsDriver struct {
	h           *driver.Handle
	readBuffers []*driver.ReadBuffer
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
	dh, err := driver.NewHandle(windows.FILE_FLAG_OVERLAPPED, driver.DataHandle)
	if err != nil {
		return err
	}

	filters, err := createDNSFilters()
	if err != nil {
		return err
	}

	if err := dh.SetDataFilters(filters); err != nil {
		return err
	}

	iocp, buffers, err := driver.PrepareCompletionBuffers(dh.Handle, dnsReadBufferCount)
	if err != nil {
		return err
	}

	d.iocp = iocp
	d.readBuffers = buffers
	d.h = dh
	return nil
}

// ReadDNSPacket visits a raw DNS packet if one is available.
func (d *dnsDriver) ReadDNSPacket(visit func([]byte, time.Time) error) (didRead bool, err error) {
	buf, _, err := driver.GetReadBufferIfReady(d.iocp)
	if err != nil {
		return false, fmt.Errorf("could not get read buffer: %w", err)
	}
	if buf == nil {
		return false, nil
	}

	fph := (*driver.FilterPacketHeader)(unsafe.Pointer(&buf.Data[0]))
	captureTime := time.Unix(0, int64(fph.Timestamp))

	start := driver.FilterPacketHeaderSize

	if err := visit(buf.Data[start:], captureTime); err != nil {
		return false, err
	}

	if err := driver.StartNextRead(d.h.Handle, buf); err != nil && err != windows.ERROR_IO_PENDING {
		return false, err
	}

	return true, nil
}

func (d *dnsDriver) Close() error {
	// destroy io completion port, and file
	if err := windows.CancelIoEx(d.h.Handle, nil); err != nil {
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
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		filters = append(filters, driver.FilterDefinition{
			FilterVersion:  driver.Signature,
			Size:           driver.FilterDefinitionSize,
			FilterLayer:    driver.LayerTransport,
			Af:             windows.AF_INET,
			RemotePort:     53,
			InterfaceIndex: uint64(iface.Index),
			Direction:      driver.DirectionOutbound,
		})

		filters = append(filters, driver.FilterDefinition{
			FilterVersion:  driver.Signature,
			Size:           driver.FilterDefinitionSize,
			FilterLayer:    driver.LayerTransport,
			Af:             windows.AF_INET,
			RemotePort:     53,
			InterfaceIndex: uint64(iface.Index),
			Direction:      driver.DirectionInbound,
		})
	}

	return filters, nil
}
