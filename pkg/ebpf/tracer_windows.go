// +build windows

package ebpf

import (
	"C"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"sync"
	"syscall"
	"os"
	"unsafe"
)

/*
#cgo LDFLAGS: -luser32
#include <windows.h>
#include <stdint.h>
*/
var (
	kernel32    = syscall.MustLoadDLL("kernel32.dll")
	CreateFile  = kernel32.MustFindProc("CreateFileW")
	CloseHandle = kernel32.MustFindProc("CloseHandle")
)

type Tracer struct {
	config *Config
}

type I2c_client struct {
	f *os.File
}

func Open(fname string) (c *I2c_client, err error) {
	f, err := os.OpenFile(fname , syscall.O_RDWR, 0666)
	if err != nil {
		return nil, err
	}
	return &I2c_client{f: f}, nil
}

func (c *I2c_client) Close() (err error) {
	return c.f.Close()
}

func NewTracer(config *Config) (*Tracer, error) {
	return &Tracer{}, nil
}

func (t *Tracer) Stop() {}


/*
WINDOWS:
CreateFileW(
  LPCWSTR               lpFileName,
  DWORD                 dwDesiredAccess,
  DWORD                 dwShareMode,
  LPSECURITY_ATTRIBUTES lpSecurityAttributes,
  DWORD                 dwCreationDisposition,
  DWORD                 dwFlagsAndAttributes,
  HANDLE                hTemplateFile
);
*/

func open(path string) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	r, _, err := CreateFile.Call(uintptr(unsafe.Pointer(p)),
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		0,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0)
	h := syscall.Handle(r)
	if h == syscall.InvalidHandle {
		return h, err
	}
	return h, nil
}

func close(handle syscall.Handle) error {
	r, _, err := CloseHandle.Call(uintptr(handle))
	if r == 0 {
		return err
	}
	return nil
}

/*
func Encode(s string) C.LPCWSTR {
	wstr := utf16.Encode([]rune(s))

	p := C.calloc(C.size_t(len(wstr)+1), C.sizeof_uint16_t)
	pp := (*[1 << 30]uint16)(p)
	copy(pp[:], wstr)

	return (C.LPCWSTR)(p)
}*/

func (t *Tracer) GetActiveConnections(_ string) (*Connections, error) {

	// p, err := Encode("\\\\.\\ddfilter") // Note the subtle change here
	blah := C.ulong(3)
	log.Infof("%d", blah)

	h, err := open("\\\\.\\ddfilter")
	if err != nil {
		panic(err)
	}

	if err != nil {
		log.Errorf("open: %v", err)
	}
	err = close(h)
	if err != nil {
		log.Errorf("close: %v", err)
	}
	err = close(h)
	if err != nil {
		log.Errorf("second close: %v", err)
	}

	return &Connections{
		DNS: map[util.Address][]string{
			util.AddressFromString("127.0.0.1"): {"localhost"},
		},
		Conns: []ConnectionStats{
			{
				Source: util.AddressFromString("127.0.0.1"),
				Dest:   util.AddressFromString("127.0.0.1"),
				SPort:  35673,
				DPort:  8000,
				Type:   TCP,
			},
		},
	}, nil
}

// getConnections returns all of the active connections in the ebpf maps along with the latest timestamp.  It takes
// a reusable buffer for appending the active connections so that this doesn't continuously allocate
func (t *Tracer) getConnections(active []ConnectionStats) ([]ConnectionStats, uint64, error) {
	return nil, 0, ErrNotImplemented
}

// GetStats returns a map of statistics about the current tracer's internal state
func (t *Tracer) GetStats() (map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

// DebugNetworkState returns a map with the current tracer's internal state, for debugging
func (t *Tracer) DebugNetworkState(clientID string) (map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

// DebugNetworkMaps returns all connections stored in the maps without modifications from network state
func (t *Tracer) DebugNetworkMaps() (*Connections, error) {
	return nil, ErrNotImplemented
}
