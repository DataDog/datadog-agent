// +build windows,npm

package network

import (
	"fmt"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows"
)

// DriverExpvar is the name of a top-level driver expvar returned from GetStats
type DriverExpvar string

const (
	totalFlowStats  DriverExpvar = "driver_total_flow_stats"
	flowHandleStats              = "driver_flow_handle_stats"
	flowStats                    = "flows"
	driverStats                  = "driver"
)

const (
	// set default max open & closed flows for windows.  See note in setParams(),
	// these are only sort-of honored for now
	defaultMaxOpenFlows   = uint64(32767)
	defaultMaxClosedFlows = uint64(32767)
)

// DriverExpvarNames is a list of all the DriverExpvar names returned from GetStats
var DriverExpvarNames = []DriverExpvar{totalFlowStats, flowHandleStats, flowStats, driverStats}

// DriverInterface holds all necessary information for interacting with the windows driver
type DriverInterface struct {
	// declare totalFlows first so it remains on a 64 bit boundary since it is used by atomic functions
	totalFlows     int64
	closedFlows    int64
	openFlows      int64
	moreDataErrors int64
	bufferSize     int64

	maxOpenFlows   uint64
	maxClosedFlows uint64

	driverFlowHandle  *driver.Handle
	driverStatsHandle *driver.Handle

	enableMonotonicCounts bool

	bufferLock sync.Mutex
	readBuffer []uint8
}

// NewDriverInterface returns a DriverInterface struct for interacting with the driver
func NewDriverInterface(config *config.Config) (*DriverInterface, error) {
	dc := &DriverInterface{
		enableMonotonicCounts: config.EnableMonotonicCount,
		readBuffer:            make([]byte, config.DriverBufferSize),
		bufferSize:            int64(config.DriverBufferSize),
		maxOpenFlows:          uint64(config.MaxTrackedConnections),
		maxClosedFlows:        uint64(config.MaxClosedConnectionsBuffered),
	}

	err := dc.setupFlowHandle()
	if err != nil {
		return nil, fmt.Errorf("error creating driver flow handle: %w", err)
	}

	err = dc.setupStatsHandle()
	if err != nil {
		return nil, fmt.Errorf("Error creating stats handle: %w", err)
	}

	return dc, nil
}

// Close shuts down the driver interface
func (di *DriverInterface) Close() error {
	if err := di.driverFlowHandle.Close(); err != nil {
		return fmt.Errorf("error closing flow file handle: %w", err)
	}
	if err := di.driverStatsHandle.Close(); err != nil {
		return fmt.Errorf("error closing stat file handle: %w", err)
	}
	return nil
}

// setupFlowHandle generates a windows Driver Handle, and creates a DriverHandle struct to pull flows from the driver
// by setting the necessary filters
func (di *DriverInterface) setupFlowHandle() error {
	dh, err := driver.NewHandle(0, driver.FlowHandle)
	if err != nil {
		return err
	}
	di.driverFlowHandle = dh

	filters, err := createFlowHandleFilters()
	if err != nil {
		return err
	}

	// Create and set flow filters for each interface
	err = di.driverFlowHandle.SetFlowFilters(filters)
	if err != nil {
		return err
	}

	// Set up the maximum amount of connections
	err = di.setFlowParams()
	if err != nil {
		return err
	}
	return nil
}

// setupStatsHandle generates a windows Driver Handle, and creates a DriverHandle struct
func (di *DriverInterface) setupStatsHandle() error {
	dh, err := driver.NewHandle(0, driver.StatsHandle)
	if err != nil {
		return err
	}

	di.driverStatsHandle = dh
	return nil
}

// GetStats returns statistics for the driver interface used by the windows tracer
func (di *DriverInterface) GetStats() (map[DriverExpvar]interface{}, error) {
	handleStats, err := di.driverFlowHandle.GetStatsForHandle()
	if err != nil {
		return nil, err
	}

	totalDriverStats, err := di.driverStatsHandle.GetStatsForHandle()
	if err != nil {
		return nil, err
	}
	totalFlows := atomic.LoadInt64(&di.totalFlows)
	openFlows := atomic.SwapInt64(&di.openFlows, 0)
	closedFlows := atomic.SwapInt64(&di.closedFlows, 0)
	moreDataErrors := atomic.SwapInt64(&di.moreDataErrors, 0)
	bufferSize := atomic.LoadInt64(&di.bufferSize)

	return map[DriverExpvar]interface{}{
		totalFlowStats:  totalDriverStats,
		flowHandleStats: handleStats,
		flowStats: map[string]int64{
			"total":  totalFlows,
			"open":   openFlows,
			"closed": closedFlows,
		},
		driverStats: map[string]int64{
			"more_data_errors": moreDataErrors,
			"buffer_size":      bufferSize,
		},
	}, nil
}

// GetConnectionStats will read all flows from the driver and convert them into ConnectionStats.
// It returns the count of connections added to the active and closed buffers, respectively.
func (di *DriverInterface) GetConnectionStats(activeBuf *DriverBuffer, closedBuf *DriverBuffer) (int, int, error) {
	di.bufferLock.Lock()
	defer di.bufferLock.Unlock()

	var activeCount, closedCount int
	var bytesRead uint32
	var totalBytesRead uint32
	// keep reading while driver says there is more data available
	for err := error(windows.ERROR_MORE_DATA); err == windows.ERROR_MORE_DATA; {
		err = windows.ReadFile(di.driverFlowHandle.Handle, di.readBuffer, &bytesRead, nil)
		if err != nil {
			if err == windows.ERROR_NO_MORE_ITEMS {
				break
			}
			if err != windows.ERROR_MORE_DATA {
				return 0, 0, fmt.Errorf("ReadFile: %w", err)
			}
			atomic.AddInt64(&di.moreDataErrors, 1)
		}
		totalBytesRead += bytesRead

		var buf []byte
		for bytesUsed := uint32(0); bytesUsed < bytesRead; bytesUsed += driver.PerFlowDataSize {
			buf = di.readBuffer[bytesUsed:]
			pfd := (*driver.PerFlowData)(unsafe.Pointer(&(buf[0])))

			if isFlowClosed(pfd.Flags) {
				FlowToConnStat(closedBuf.Next(), pfd, di.enableMonotonicCounts)
				closedCount++
			} else {
				FlowToConnStat(activeBuf.Next(), pfd, di.enableMonotonicCounts)
				activeCount++
			}
		}
	}

	di.readBuffer = resizeDriverBuffer(int(totalBytesRead), di.readBuffer)
	atomic.StoreInt64(&di.bufferSize, int64(len(di.readBuffer)))

	atomic.AddInt64(&di.openFlows, int64(activeCount))
	atomic.AddInt64(&di.closedFlows, int64(closedCount))
	atomic.AddInt64(&di.totalFlows, int64(activeCount+closedCount))

	return activeCount, closedCount, nil
}

func resizeDriverBuffer(compareSize int, buffer []uint8) []uint8 {
	// Explicitly setting len to 0 causes the ReadFile syscall to break, so allocate buffer with cap = len
	if compareSize >= cap(buffer)*2 {
		return make([]uint8, cap(buffer)*2)
	} else if compareSize <= cap(buffer)/2 {
		// Take the max of buffer/2 and compareSize to limit future array resizes
		return make([]uint8, int(math.Max(float64(cap(buffer)/2), float64(compareSize))))
	}
	return buffer
}

func minUint64(a, b uint64) uint64 {
	if a > b {
		return b
	}
	return a
}

// setParams passes any configuration values from the config file down
// to the driver.
func (di *DriverInterface) setFlowParams() error {
	// set up the maximum flows

	// temporary setup.  Will set the maximum flows to the sum of the configured
	// max_tracked_connections and max_closed_connections_buffered, setting a
	// (hard_coded) maximum.  This will be updated to actually honor the separate
	// config values when the driver is updated to track them separately.

	// this makes it so that the config can clamp down, but can never make it
	// larger than the coded defaults above.
	maxFlows := minUint64(defaultMaxOpenFlows+defaultMaxClosedFlows, di.maxOpenFlows+di.maxClosedFlows)
	log.Debugf("Setting max flows in driver to %v", maxFlows)
	err := windows.DeviceIoControl(di.driverFlowHandle.Handle,
		driver.SetMaxFlowsIOCTL,
		(*byte)(unsafe.Pointer(&maxFlows)),
		uint32(unsafe.Sizeof(maxFlows)),
		nil,
		uint32(0), nil, nil)
	if err != nil {
		log.Warnf("Failed to set max number of flows to %v %v", maxFlows, err)
	}
	return err
}

func createFlowHandleFilters() (filters []driver.FilterDefinition, err error) {
	ifaces, err := net.Interfaces()

	// Two filters per iface
	if err != nil {
		return nil, fmt.Errorf("error getting interfaces: %s", err.Error())
	}

	for _, iface := range ifaces {
		log.Debugf("Creating filters for interface: %s [%+v]", iface.Name, iface)
		// Set ipv4 Traffic
		filters = append(filters, newDDAPIFilter(driver.DirectionOutbound, driver.LayerTransport, iface.Index, true))
		filters = append(filters, newDDAPIFilter(driver.DirectionInbound, driver.LayerTransport, iface.Index, true))
		// Set ipv6
		filters = append(filters, newDDAPIFilter(driver.DirectionOutbound, driver.LayerTransport, iface.Index, false))
		filters = append(filters, newDDAPIFilter(driver.DirectionInbound, driver.LayerTransport, iface.Index, false))
	}

	return filters, nil
}

// NewDDAPIFilter returns a filter we can apply to the driver
func newDDAPIFilter(direction, layer uint64, ifaceIndex int, isIPV4 bool) driver.FilterDefinition {
	var fd driver.FilterDefinition
	fd.FilterVersion = driver.Signature
	fd.Size = driver.FilterDefinitionSize
	// TODO Remove direction setting for flow filters once all verification code has been removed from driver
	fd.Direction = direction
	fd.FilterLayer = layer

	if isIPV4 {
		fd.Af = windows.AF_INET
		fd.V4InterfaceIndex = uint64(ifaceIndex)
	} else {
		fd.Af = windows.AF_INET6
		fd.V6InterfaceIndex = uint64(ifaceIndex)
	}

	return fd
}
