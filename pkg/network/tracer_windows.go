// +build windows

package network

/*
//! Defines the objects used to communicate with the driver as well as its control codes
#include "c/ddfilterapi.h"

//! These includes are needed to use constants defined in the ddfilterapi
#include <WinDef.h>
#include <WinIoCtl.h>
*/
import "C"
import (
	"encoding/binary"
	"expvar"
	"fmt"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"golang.org/x/sys/windows"
)

const (
	driverFile = `\\.\ddfilter`
)

var (
	expvarEndpoints map[string]*expvar.Map
	expvarTypes     = []string{"driver_total_stats", "driver_handle_stats"}

	// Buffer holding datadog driver filterapi (ddfilterapi) signature to ensure consistency with driver.
	ddAPIVersionBuf = makeDDAPIVersionBuffer(C.DD_FILTER_SIGNATURE)
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
	driverHandle windows.Handle
}

// NewTracer returns an initialized tracer struct
func NewTracer(config *Config) (*Tracer, error) {
	handle, err := openDriverFile(driverFile)
	if err != nil {
		return nil, fmt.Errorf("%s : %s", "Could not create driver handle", err)
	}

	tr := &Tracer{
		driverHandle: handle,
	}

	go tr.expvarStats()
	return tr, nil
}

func (t *Tracer) expvarStats() {
	ticker := time.NewTicker(5 * time.Second)
	// starts running the body immediately instead waiting for the first tick
	for range ticker.C {
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

func openDriverFile(path string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	log.Debug("Creating Driver handle...")
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OVERLAPPED,
		windows.Handle(0))
	if err != nil {
		return windows.InvalidHandle, err
	}
	log.Info("Connected to driver and handle created")
	return h, nil
}

func closeDriverFile(handle windows.Handle) error {
	return windows.CloseHandle(handle)
}

// We create a buffer because the system calls we make need a *byte which is not
// possible with const value
func makeDDAPIVersionBuffer(signature uint64) []byte {
	buf := make([]byte, C.sizeof_uint64_t)
	binary.LittleEndian.PutUint64(buf, signature)
	return buf
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
	var (
		bytesReturned uint32
		statbuf       = make([]byte, C.sizeof_struct_driver_stats)
	)

	err := windows.DeviceIoControl(t.driverHandle, C.DDFILTER_IOCTL_GETSTATS, &ddAPIVersionBuf[0], uint32(len(ddAPIVersionBuf)), &statbuf[0], uint32(len(statbuf)), &bytesReturned, nil)
	if err != nil {
		return nil, fmt.Errorf("error reading Stats with DeviceIoControl: %v", err)
	}

	stats := *(*C.struct_driver_stats)(unsafe.Pointer(&statbuf[0]))
	return map[string]interface{}{
		"driver_total_stats": map[string]int64{
			"read_calls":             int64(stats.total.read_calls),
			"read_bytes":             int64(stats.total.read_bytes),
			"read_calls_outstanding": int64(stats.total.read_calls_outstanding),
			"read_calls_cancelled":   int64(stats.total.read_calls_cancelled),
			"read_packets_skipped":   int64(stats.total.read_packets_skipped),
			"write_calls":            int64(stats.total.write_calls),
			"write_bytes":            int64(stats.total.write_bytes),
			"ioctl_calls":            int64(stats.total.ioctl_calls),
		},
		"driver_handle_stats": map[string]int64{
			"read_calls":             int64(stats.handle.read_calls),
			"read_bytes":             int64(stats.handle.read_bytes),
			"read_calls_outstanding": int64(stats.handle.read_calls_outstanding),
			"read_calls_cancelled":   int64(stats.handle.read_calls_cancelled),
			"read_packets_skipped":   int64(stats.handle.read_packets_skipped),
			"write_calls":            int64(stats.handle.write_calls),
			"write_bytes":            int64(stats.handle.write_bytes),
			"ioctl_calls":            int64(stats.handle.ioctl_calls),
		},
	}, nil
}

// DebugState returns a map with the current tracer's internal state, for debugging
func (t *Tracer) DebugState(clientID string) (map[string]interface{}, error) {
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
