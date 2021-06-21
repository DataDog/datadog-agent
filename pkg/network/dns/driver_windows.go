package dns

/*
#include <stdlib.h>
#include <memory.h>
*/
import "C"
import (
	"fmt"
	"net"
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
	h           *driver.Handle
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

	iocp, buffers, err := prepareCompletionBuffers(dh.Handle, dnsReadBufferCount)
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

	var buf *readbuffer
	buf = (*readbuffer)(unsafe.Pointer(ol))

	fph := (*driver.FilterPacketHeader)(unsafe.Pointer(&buf.data[0]))
	captureTime := time.Unix(0, int64(fph.Timestamp))

	start := driver.FilterPacketHeaderSize

	if err := visit(buf.data[start:], captureTime); err != nil {
		return false, err
	}

	// kick off another read
	if err := windows.ReadFile(d.h.Handle, buf.data[:], nil, &(buf.ol)); err != nil && err != windows.ERROR_IO_PENDING {
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
			FilterVersion:    driver.Signature,
			Size:             driver.FilterDefinitionSize,
			FilterLayer:      driver.LayerTransport,
			Af:               windows.AF_INET,
			RemotePort:       53,
			V4InterfaceIndex: uint64(iface.Index),
			Direction:        driver.DirectionOutbound,
		})

		filters = append(filters, driver.FilterDefinition{
			FilterVersion:    driver.Signature,
			Size:             driver.FilterDefinitionSize,
			FilterLayer:      driver.LayerTransport,
			Af:               windows.AF_INET,
			RemotePort:       53,
			V4InterfaceIndex: uint64(iface.Index),
			Direction:        driver.DirectionInbound,
		})
	}

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
