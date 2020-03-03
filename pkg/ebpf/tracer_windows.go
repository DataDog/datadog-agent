// +build windows

package ebpf

/*
typedef struct _stats
{
    long Read_calls;		//! number of read calls to the driver
    long Read_bytes;
    long Read_calls_outstanding;
    long Read_calls_completed;
    long Read_calls_cancelled;
    long Read_packets_skipped;
    long Write_calls;	//! number of write calls to the driver
    long Write_bytes;
    long Ioctl_calls;	//! number of ioctl calls to the driver
} STATS;

typedef struct driver_stats
{
    STATS		Total;		//! stats since the driver was started
    STATS		Handle;		//! stats for the file handle in question
} DRIVER_STATS;
*/
import "C"
import (
	"expvar"
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	DRIVERFILE = `\\.\ddfilter`

	// https://github.com/DataDog/datadog-windows-filter/blob/master/include/ddfilterapi.h
	DDFILTER_IOCTL_GETSTATS               = 0x801
	DDFILTER_IOCTL_SIMULATE_COMPLETE_READ = 0x802
	DDFILTER_IOCTL_SET_FILTER             = 0x803

	// https://docs.microsoft.com/en-us/windows-hardware/drivers/kernel/specifying-device-types
	NETWORK_DEVICE_TYPE_CTL_CODE = 0x00000012
)

var (
	kernel32        = syscall.MustLoadDLL("kernel32.dll")
	CreateFile      = kernel32.MustFindProc("CreateFileW")
	CloseHandle     = kernel32.MustFindProc("CloseHandle")
	expvarEndpoints map[string]*expvar.Map
	expvarTypes     = []string{"driver_total_stats", "driver_handle_stats"}
)

func init() {
	expvarEndpoints = make(map[string]*expvar.Map, len(expvarTypes))
	for _, name := range expvarTypes {
		expvarEndpoints[name] = expvar.NewMap(name)
	}
}

// Tracer struct for tracking network state and connections
type Tracer struct {
	config       *Config
	DriverHandle syscall.Handle
}

// NewTracer returns an initialized tracer struct
func NewTracer(config *Config) (*Tracer, error) {
	handle, err := openDriverFile(DRIVERFILE)
	if err != nil {
		return nil, fmt.Errorf("%s : %s", "Could not create driver handle", err)
	}
	return &Tracer{
		DriverHandle: handle,
	}, nil
}

func (t *Tracer) expvarStats() {
	ticker := time.NewTicker(5 * time.Second)
	// starts running the body immediately instead waiting for the first tick
	for ; true; <-ticker.C {
		stats, err := t.GetStats()
		if err != nil {
			continue
		}

		for name, stat := range stats {
			for metric, val := range stat.(map[string]int64) {
				currVal := &expvar.Int{}
				currVal.Set(val)
				expvarEndpoints[name].Set(snakeToCapInitialCamel(metric), currVal)
			}
		}
	}
}

// Stop function stops running tracer
func (t *Tracer) Stop() {}

func openDriverFile(path string) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	log.Debug("Creating Driver File...")
	r, _, err := CreateFile.Call(uintptr(unsafe.Pointer(p)),
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		0,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_OVERLAPPED,
		0)
	log.Debug("Creating Driver Handle...")
	h := syscall.Handle(r)
	if h == syscall.InvalidHandle {
		return h, err
	}
	log.Info("Connected to driver and handle created")
	return h, nil
}

func GetIoCompletionPort(handleFile syscall.Handle) (syscall.Handle, error) {
	iocpHandle, err := syscall.CreateIoCompletionPort(handleFile, 0, 0, 0)
	if err != nil {
		return syscall.Handle(0), err
	}
	return iocpHandle, nil
}

func closeDriverFile(handle syscall.Handle) error {
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

// GetActiveConnections returns all active connections
func (t *Tracer) GetActiveConnections(_ string) (*Connections, error) {
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
	if t.DriverHandle == syscall.InvalidHandle {
		return nil, fmt.Errorf("Problem with handle cannot get stats.")
	}

	var (
		bytesReturned uint32
		stats         C.struct_driver_stats
		statbuf       = make([]byte, C.sizeof_struct_driver_stats)
		ioctlcd       = ctl_code(NETWORK_DEVICE_TYPE_CTL_CODE, DDFILTER_IOCTL_GETSTATS, uint32(0), uint32(0))
	)

	err := syscall.DeviceIoControl(t.DriverHandle, ioctlcd, nil, 0, &statbuf[0], uint32(len(statbuf)), &bytesReturned, nil)
	if err != nil {
		log.Errorf("Error reading Stats with DeviceIoControl: %v", err)
	}

	stats = *(*C.struct_driver_stats)(unsafe.Pointer(&statbuf[0]))

	return map[string]interface{}{
		"driver_total_stats": map[string]C.long{
			"read_calls":             stats.Total.Read_calls,
			"read_bytes":             stats.Total.Read_bytes,
			"read_calls_outstanding": stats.Total.Read_calls_outstanding,
			"read_calls_cancelled":   stats.Total.Read_calls_cancelled,
			"read_packets_skipped":   stats.Total.Read_packets_skipped,
			"write_calls":            stats.Total.Write_calls,
			"write_bytes":            stats.Total.Write_bytes,
			"ioctl_calls":            stats.Total.Ioctl_calls,
		},
		"driver_handle_stats": map[string]C.long{
			"read_calls":             stats.Handle.Read_calls,
			"read_bytes":             stats.Handle.Read_bytes,
			"read_calls_outstanding": stats.Handle.Read_calls_outstanding,
			"read_calls_cancelled":   stats.Handle.Read_calls_cancelled,
			"read_packets_skipped":   stats.Handle.Read_packets_skipped,
			"write_calls":            stats.Handle.Write_calls,
			"write_bytes":            stats.Handle.Write_bytes,
			"ioctl_calls":            stats.Handle.Ioctl_calls,
		},
	}, nil
}

// DebugNetworkState returns a map with the current tracer's internal state, for debugging
func (t *Tracer) DebugNetworkState(clientID string) (map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

// DebugNetworkMaps returns all connections stored in the maps without modifications from network state
func (t *Tracer) DebugNetworkMaps() (*Connections, error) {
	return nil, ErrNotImplemented
}

// CurrentKernelVersion is not implemented on this OS for Tracer
func CurrentKernelVersion() (uint32, error) {
	return 0, ErrNotImplemented
}
