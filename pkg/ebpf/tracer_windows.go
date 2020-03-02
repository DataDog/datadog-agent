// +build windows

package ebpf

/*
typedef struct in6_addr {
  union {
    unsigned char Byte[16];
    unsigned short Word[8];
  } U;
} IN6_ADDR;

struct in_addr {
  union {
    struct {
      unsigned char S_b1;
      unsigned char S_b2;
      unsigned char S_b3;
      unsigned char S_b4;
    } S_un_b;
    struct {
      unsigned short S_w1;
      unsigned short S_w2;
    } S_un_w;
    unsigned long S_addr;
  } S_un;
} IN_ADDR;

typedef struct _filterAddress
{
// _filterAddress defines an address to be matched, if supplied.
// it can be ipv4 or ipv6 but not both.
// supplying 0 for the address family means _any_ address (v4 or v6)
	unsigned short af; //! AF_INET, AF_INET6 or 0
	union
	{
		struct in6_addr         V6_address;
		struct in_addr          V4_address;
	}u;
	unsigned short mask; // number of mask bits.
} FILTER_ADDRESS;

typedef enum
{
	DIRECTION_INBOUND = 0,
	DIRECTION_OUTBOUND = 1
} FILTER_DIRECTION;

typedef struct _filterDefinition
{
	unsigned long Size;         //! size of this structure

//  if supplied, the source and destination address must have the same address family.
//  if both source and destination are applied, then the match for this filter
//  is a logical AND, i.e. the source and destination both match.
	unsigned short Af;     //! address family to filter

	FILTER_ADDRESS  SourceAddress;
	FILTER_ADDRESS  DestAddress;
	unsigned short SourcePort;
	unsigned short DestinationPort;
	unsigned short Protocol;
	FILTER_DIRECTION    Direction;
} FILTER_DEFINITION;

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
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"syscall"
	"unsafe"
)

const (
	DRIVERFILE                            = "\\\\.\\ddfilter"
	DDFILTER_IOCTL_GETSTATS               = 0x801
	DDFILTER_IOCTL_SIMULATE_COMPLETE_READ = 0x802
	DDFILTER_IOCTL_SET_FILTER             = 0x803
	NETWORK_DEVICE_TYPE_CTL_CODE          = 0x00000012
)

var (
	kernel32    = syscall.MustLoadDLL("kernel32.dll")
	CreateFile  = kernel32.MustFindProc("CreateFileW")
	CloseHandle = kernel32.MustFindProc("CloseHandle")
)

// Tracer struct for tracking network state and connections
type Tracer struct {
	config       *Config
	DriverHandle syscall.Handle
}

// NewTracer returns an initialized tracer struct
func NewTracer(config *Config) (*Tracer, error) {
	handle, err := openDriverFile(DRIVERFILE)
	if err != nil {
		return nil, fmt.Errorf("%s : %s", "Could not open driver file", err)
	}
	return &Tracer{
		DriverHandle: handle,
	}, nil
}

// Stop function stops running tracer
func (t *Tracer) Stop() {}

func openDriverFile(path string) (syscall.Handle, error) {
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

func createFilterDefinition(family uint16, direction FilterDirection, dPort uint16) FilterDefinition {
	return FilterDefinition{
		size:          uint32(unsafe.Sizeof(FilterDefinition{})),
		addressFamily: family,
		dPort:         dPort,
		direction:     direction,
	}
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
	var (
		bytesReturned uint32
		stats         C.struct_driver_stats
		statbuf       = make([]byte, C.sizeof_struct_driver_stats)
		ioctlcd       = ctl_code(NETWORK_DEVICE_TYPE_CTL_CODE, DDFILTER_IOCTL_GETSTATS, uint32(0), uint32(0))
	)

	err := syscall.DeviceIoControl(t.DriverHandle, ioctlcd, nil, 0, &statbuf[0], uint32(len(statbuf)), &bytesReturned, nil)
	if err != nil {
		log.Errorf("Error reading stats with DeviceIoControl: %v", err)
	}

	stats = *(*C.struct_driver_stats)(unsafe.Pointer(&statbuf[0]))
	return map[string]interface{}{
		"BytesReturned" : bytesReturned,
		"TotalStats": map[string]C.long{
			"Read_calls":             stats.Total.Read_calls,
			"Read_bytes":             stats.Total.Read_bytes,
			"Read_calls_outstanding": stats.Total.Read_calls_outstanding, // Skipped connections (e.g. Local DNS requests)
			"Read_calls_cancelled":   stats.Total.Read_calls_cancelled,
			"Read_packets_skipped":   stats.Total.Read_packets_skipped,
			"Write_calls":            stats.Total.Write_calls,
			"Write_bytes":            stats.Total.Write_bytes,
			"Ioctl_calls":            stats.Total.Ioctl_calls,
		},
		"HandleStats": map[string]C.long{
			"Read_calls":             stats.Handle.Read_calls,
			"Read_bytes":             stats.Handle.Read_bytes,
			"Read_calls_outstanding": stats.Handle.Read_calls_outstanding, // Skipped connections (e.g. Local DNS requests)
			"Read_calls_cancelled":   stats.Handle.Read_calls_cancelled,
			"Read_packets_skipped":   stats.Handle.Read_packets_skipped,
			"Write_calls":            stats.Handle.Write_calls,
			"Write_bytes":            stats.Handle.Write_bytes,
			"Ioctl_calls":            stats.Handle.Ioctl_calls,
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
