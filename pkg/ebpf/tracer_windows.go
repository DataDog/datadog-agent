// +build windows

package ebpf

/*
#include "c/ddfilterapi.h"
*/
import "C"
import (
	"expvar"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultPollInterval = int(15)
)

var (
	expvarEndpoints map[string]*expvar.Map
	expvarTypes     = []string{"driver_total_flow_stats", "driver_flow_handle_stats", "total_flows"}
)

func init() {
	expvarEndpoints = make(map[string]*expvar.Map, len(expvarTypes))
	for _, name := range expvarTypes {
		expvarEndpoints[name] = expvar.NewMap(name)
	}
}

// Tracer struct for tracking network state and connections
type Tracer struct {
	config          *Config
	driverInterface *DriverInterface
	stopChan        chan struct{}

	timerInterval int
	// waitgroup for all of the running goroutines
	waitgroup sync.WaitGroup

	// ticker for the polling interval for writing
	inTicker            *time.Ticker
	stopInTickerRoutine chan bool
}

// NewTracer returns an initialized tracer struct
func NewTracer(config *Config) (*Tracer, error) {
	di, err := NewDriverInterface()
	if err != nil {
		return nil, fmt.Errorf("could not create windows driver controller: %v", err)
	}

	tr := &Tracer{
		driverInterface: di,
		stopChan:        make(chan struct{}),
		timerInterval:   defaultPollInterval,
	}

	err = tr.initFlowPolling(tr.stopChan)
	if err != nil {
		return nil, fmt.Errorf("issue polling packets from driver: %v", err)
	}
	go tr.expvarStats(tr.stopChan)
	return tr, nil
}

// Stop function stops running tracer
func (t *Tracer) Stop() {
	close(t.stopChan)
	t.driverInterface.close()
}

func (t *Tracer) expvarStats(exit <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	// starts running the body immediately instead waiting for the first tick
	for range ticker.C {
		select {
		case <-exit:
			return
		default:
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
}

func (t *Tracer) initFlowPolling(exit <-chan struct{}) (err error) {

	log.Debugf("Started flow polling")
	go func() {
		t.waitgroup.Add(1)
		defer t.waitgroup.Done()
		t.inTicker = time.NewTicker(time.Second * time.Duration(t.timerInterval))
		defer t.inTicker.Stop()
		for {
			select {
			case <-t.stopInTickerRoutine:
				return
			case <-t.inTicker.C:
				log.Infof("getFlowsruns")
				flows, err := t.driverInterface.getFlows(&t.waitgroup)
				if err != nil {
					return
				}
				printFlows(flows)
			}
		}
	}()
	return nil
}

func printFlows(pfds []C.struct__perFlowData) {
	for _, pfd := range pfds {
		var local net.IP
		var remot net.IP
		var len C.int

		if pfd.addressFamily == syscall.AF_INET {
			len = 4
		} else {
			len = 16
		}
		local = C.GoBytes(unsafe.Pointer(&pfd.localAddress), len)
		remot = C.GoBytes(unsafe.Pointer(&pfd.remoteAddress), len)

		state := "Open  "
		if (pfd.flags & 0x10) == 0x10 {
			state = "Closed"
		}
		log.Infof("    %v  FH:  %8v    PID:  %8v   AF: %2v    P: %3v        Flags:  0x%v\n",
			state, pfd.flowHandle, pfd.processId, pfd.addressFamily, pfd.protocol, pfd.flags)
		log.Infof("    L:  %16s:%5d     R: %16s:%5d\n", local.String(), pfd.localPort, remot.String(), pfd.remotePort)
		log.Infof("    PktIn:  %8v  BytesIn:  %8v    PktOut:  %8v:  BytesOut:  %8v\n", pfd.packetsIn, pfd.bytesIn, pfd.packetsOut, pfd.bytesOut)
		return
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
	driverStats, err := t.driverInterface.getStats()
	if err != nil {
		log.Errorf("not printing driver stats: %v", err)
	}

	return map[string]interface{}{
		"total_flows":              driverStats["total_flows"],
		"driver_total_flow_stats":  driverStats["driver_total_flow_stats"],
		"driver_flow_handle_stats": driverStats["driver_flow_handle_stats"],
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
