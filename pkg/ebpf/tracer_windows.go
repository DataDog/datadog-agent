// +build windows

package ebpf

import (
	"C"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"syscall"
	"unsafe"
)

var (
	kernel32    = syscall.MustLoadDLL("kernel32.dll")
	CreateFile  = kernel32.MustFindProc("CreateFileW")
	CloseHandle = kernel32.MustFindProc("CloseHandle")
)

// Tracer struct for tracking network state and connections
type Tracer struct {
	config *Config
}

// NewTracer returns an initialized tracer struct
func NewTracer(config *Config) (*Tracer, error) {
	return &Tracer{}, nil
}

// Stop function stops running tracer
func (t *Tracer) Stop() {}

func open(path string) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	log.Info("Creating file...")
	r, _, err := CreateFile.Call(uintptr(unsafe.Pointer(p)),
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		0,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_OVERLAPPED,
		0)
	log.Info("creating handle...")
	h := syscall.Handle(r)
	log.Info("Handle created")
	if h == syscall.InvalidHandle {
		return h, err
	}
	return h, nil
}

func GetIoCompletionPort(handleFile syscall.Handle) (syscall.Handle, error) {
	// logger.Print("creating io completion port")
	iocpHandle, err := syscall.CreateIoCompletionPort(handleFile,0,0, 0)
	if err != nil {
		return syscall.Handle(0), err
	}
	return iocpHandle, nil
}

func close(handle syscall.Handle) error {
	r, _, err := CloseHandle.Call(uintptr(handle))
	if r == 0 {
		return err
	}
	return nil
}

// Creates the IOCTLCode to be passed for DeviceIoControl syscall
func ctl_code(device_type, function, method, access uint32) uint32 {
	return (device_type << 16) | (access << 14) | (function << 2) | method
}

func createFilterDefinition(family uint16) FilterDefinition {

	return FilterDefinition{
		size: uint32(unsafe.Sizeof(FilterDefinition{})),
		dst: nil,
		sPort: 0,

	}
}

// GetActiveConnections returns all active connections
func (t *Tracer) GetActiveConnections(_ string) (*Connections, error) {

	log.Info("GetActiveConnections Called")
	h, err := open("\\\\.\\ddfilter")
	if err != nil {
		panic(err)
	}

	rdbbuf := make([]byte, syscall.MAXIMUM_REPARSE_DATA_BUFFER_SIZE)
	var bytesReturned uint32
	ioctlcd := ctl_code(0x00000012, 0x801, uint32(0), uint32(0))
	err = syscall.DeviceIoControl(h, ioctlcd, nil, 0, &rdbbuf[0], uint32(len(rdbbuf)), &bytesReturned, nil)
	if err != nil {
		log.Errorf("Close: %v", err)
	}

	log.Infof("Total bytes returned: %d\n", bytesReturned)

	err = close(h)
	if err != nil {
		log.Errorf("close: %v", err)
	}

	return &Connections{
		DNS: map[util.Address][]string{
			util.AddressFromString("127.0.0.1"): {"localhost"},
		},
		Conns: []ConnectionStats{
			{
				Source: util.AddressFromString("127.0.0.1"),
				Dest:   util.AddressFromString("128.0.0.1"),
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

/*
	if err != nil {
		log.Errorf("open: %v", err)
	}

	log.Info("Calling getiocompletionport")
	iocp, err := GetIoCompletionPort(h)
	overlapped := &syscall.Overlapped{
		Internal:     0,
		InternalHigh: 0,
		Offset:       0,
		OffsetHigh:   0,
		HEvent:       0,
	}
	bytes := uint32(0)
	key := uint32(0)
	log.Info("Calling getqueuedcompletionstatus")
	cs := syscall.GetQueuedCompletionStatus(iocp, &bytes, &key, &overlapped, 5)
	log.Info(cs)
	log.Infof("received %d bytes\n", bytes)
*/
