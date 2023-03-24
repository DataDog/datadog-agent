// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package network

import (
	"fmt"
	"math"
	"sync"
	"unsafe"

	"go.uber.org/atomic"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DriverExpvar is the name of a top-level driver expvar returned from GetStats
type DriverExpvar string

const (
	totalFlowStats  DriverExpvar = "driver_total_flow_stats"
	flowHandleStats DriverExpvar = "driver_flow_handle_stats"
	flowStats       DriverExpvar = "flows"
	driverStats     DriverExpvar = "driver"
)

const (
	// set default max open & closed flows for windows.  See note in setParams(),
	// these are only sort-of honored for now
	defaultMaxOpenFlows   = uint64(32767)
	defaultMaxClosedFlows = uint64(65535)

	// starting number of entries usermode flow buffer can contain
	defaultFlowEntries      = 50
	defaultDriverBufferSize = defaultFlowEntries * driver.PerFlowDataSize
)

// DriverExpvarNames is a list of all the DriverExpvar names returned from GetStats
var DriverExpvarNames = []DriverExpvar{totalFlowStats, flowHandleStats, flowStats, driverStats}

type driverReadBuffer []uint8

type driverResizeResult int

const (
	ResizedDecreased driverResizeResult = -1
	ResizedUnchanged driverResizeResult = 0
	ResizedIncreased driverResizeResult = 1
)

// DriverInterface holds all necessary information for interacting with the windows driver
type DriverInterface struct {
	totalFlows     *atomic.Int64
	closedFlows    *atomic.Int64
	openFlows      *atomic.Int64
	moreDataErrors *atomic.Int64

	nOpenBufferIncreases *atomic.Int64
	nOpenBufferDecreases *atomic.Int64

	nClosedBufferIncreases *atomic.Int64
	nClosedBufferDecreases *atomic.Int64

	maxOpenFlows           uint64
	maxClosedFlows         uint64
	closedFlowsSignalLimit uint64

	driverFlowHandle driver.Handle
	closeFlowEvent   windows.Handle

	enableMonotonicCounts bool

	openBufferLock sync.Mutex
	openBuffer     driverReadBuffer

	closedBufferLock sync.Mutex
	closedBuffer     driverReadBuffer

	cfg *config.Config
}

// Function pointer definition passed to NewDriverInterface that enables
// creating DriverInterfaces with varying handle types like ReadDriverHandle or
// TestDriverHandle*
type HandleCreateFn func(flags uint32, handleType driver.HandleType) (driver.Handle, error)

// NewDriverInterface returns a DriverInterface struct for interacting with the driver
func NewDriverInterface(cfg *config.Config, handleFunc HandleCreateFn) (*DriverInterface, error) {
	dc := &DriverInterface{
		totalFlows:             atomic.NewInt64(0),
		closedFlows:            atomic.NewInt64(0),
		openFlows:              atomic.NewInt64(0),
		moreDataErrors:         atomic.NewInt64(0),
		nOpenBufferIncreases:   atomic.NewInt64(0),
		nOpenBufferDecreases:   atomic.NewInt64(0),
		nClosedBufferIncreases: atomic.NewInt64(0),
		nClosedBufferDecreases: atomic.NewInt64(0),

		cfg:                    cfg,
		enableMonotonicCounts:  cfg.EnableMonotonicCount,
		openBuffer:             make([]byte, defaultDriverBufferSize),
		closedBuffer:           make([]byte, defaultDriverBufferSize),
		maxOpenFlows:           uint64(cfg.MaxTrackedConnections),
		maxClosedFlows:         uint64(cfg.MaxClosedConnectionsBuffered),
		closedFlowsSignalLimit: uint64(cfg.ClosedConnectionFlushThreshold),
	}

	h, err := handleFunc(0, driver.FlowHandle)
	if err != nil {
		return nil, err
	}
	dc.driverFlowHandle = h

	err = dc.setupFlowHandle()
	if err != nil {
		return nil, fmt.Errorf("error creating driver flow handle: %w", err)
	}
	err = dc.setupClassification()
	if err != nil {
		return nil, fmt.Errorf("error configuring classification settings: %w", err)
	}
	return dc, nil
}

// Close shuts down the driver interface
func (di *DriverInterface) Close() error {
	if err := di.driverFlowHandle.Close(); err != nil {
		log.Warnf("error closing flow file handle: %v", err)
	}
	windows.SetEvent(di.closeFlowEvent)
	if err := windows.CloseHandle(di.closeFlowEvent); err != nil {
		log.Warnf("Error closing closed flow wait handle")
	}
	di.closeFlowEvent = windows.Handle(0)
	return nil
}

func (di *DriverInterface) GetHandle() driver.Handle {
	return di.driverFlowHandle
}

// setupFlowHandle generates a windows Driver Handle, and creates a DriverHandle struct to pull flows from the driver
// by setting the necessary filters
func (di *DriverInterface) setupFlowHandle() error {

	filters, err := di.createFlowHandleFilters()
	if err != nil {
		return err
	}

	// Create and set flow filters for each interface
	err = di.SetFlowFilters(filters)
	if err != nil {
		return err
	}

	// open the event handle for getting notifications that it's time
	// to go get the closed flows
	u16eventname, err := windows.UTF16PtrFromString("Global\\DDNPMClosedFlowsReadyEvent")
	if err != nil {
		return err
	}
	h, err := windows.CreateEvent(nil, 1, 0, u16eventname)
	if err != nil {
		if err == windows.ERROR_ALREADY_EXISTS && h != windows.Handle(0) {
			// ERROR_ALREADY_EXISTS is expected, because the driver will open
			// the event on fh creation; we're just opening a different handle
			// to the same event
			di.closeFlowEvent = h
		} else {
			return fmt.Errorf("Failed to create closed flow event %v", err)
		}
	} else {
		di.closeFlowEvent = h
	}
	// Set up the maximum amount of connections
	err = di.setFlowParams()
	if err != nil {
		return err
	}
	return nil
}

func (di *DriverInterface) setupClassification() error {
	if di.cfg.ProtocolClassificationEnabled == false {
		log.Infof("Traffic classification not enabled")
		return nil
	}
	// else
	log.Infof("Enabling traffic classification")
	var settings driver.ClassificationSettings
	settings.Enabled = 1
	err := di.driverFlowHandle.DeviceIoControl(
		driver.EnableClassifyIOCTL,
		(*byte)(unsafe.Pointer(&settings)),
		uint32(driver.ClassificationSettingsTypeSize),
		nil,
		uint32(0), nil, nil)
	if err != nil {
		log.Warnf("Error enabling classification %v", err)
	}
	return err
}

// SetFlowFilters installs the provided filters for flows
func (di *DriverInterface) SetFlowFilters(filters []driver.FilterDefinition) error {
	var id int64
	for _, filter := range filters {
		err := di.driverFlowHandle.DeviceIoControl(
			driver.SetFlowFilterIOCTL,
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

// GetStats returns statistics for the driver interface used by the windows tracer
func (di *DriverInterface) GetStats() (map[DriverExpvar]interface{}, error) {
	stats, err := di.driverFlowHandle.GetStatsForHandle()
	if err != nil {
		return nil, err
	}

	totalFlows := di.totalFlows.Load()
	openFlows := di.openFlows.Swap(0)
	closedFlows := di.closedFlows.Swap(0)
	moreDataErrors := di.moreDataErrors.Swap(0)
	di.closedBufferLock.Lock()
	closedBufferSize := int64(cap(di.closedBuffer))
	di.closedBufferLock.Unlock()

	nClosedBufferIncreases := di.nClosedBufferIncreases.Load()
	nClosedBufferDecreases := di.nClosedBufferDecreases.Load()

	di.openBufferLock.Lock()
	openBufferSize := int64(cap(di.openBuffer))
	di.openBufferLock.Unlock()

	nOpenBufferIncreases := di.nOpenBufferIncreases.Load()
	nOpenBufferDecreases := di.nOpenBufferDecreases.Load()

	return map[DriverExpvar]interface{}{
		flowHandleStats: stats["handle"],
		flowStats: map[string]int64{
			"total":  totalFlows,
			"open":   openFlows,
			"closed": closedFlows,
		},
		driverStats: map[string]int64{
			"more_data_errors":        moreDataErrors,
			"closed_buffer_size":      closedBufferSize,
			"closed_buffer_increases": nClosedBufferIncreases,
			"closed_buffer_decreases": nClosedBufferDecreases,
			"open_buffer_size":        openBufferSize,
			"open_buffer_increases":   nOpenBufferIncreases,
			"open_buffer_decreases":   nOpenBufferDecreases,
		},
	}, nil
}

//nolint:deadcode,unused // debugging helper normally commented out
func printClassification(fd *driver.PerFlowData) {
	if fd.ClassificationStatus != driver.ClassificationUnclassified {
		if fd.ClassifyRequest == driver.ClassificationRequestTLS || fd.ClassifyResponse == driver.ClassificationResponseTLS {
			log.Infof("Flow classified %v", fd.ClassificationStatus)
			log.Infof("Flow classify request %v", fd.ClassifyRequest)
			log.Infof("Flow classify response %v", fd.ClassifyResponse)
			log.Infof("Flow classify ALPN Requested Protocols %x", fd.Tls_alpn_requested)
			log.Infof("Flow classify ALPN chosen    Protocols %x", fd.Tls_alpn_chosen)
			log.Infof("tls versions offered:  %x", fd.Tls_versions_offered)
			log.Infof("tls version  chosen:   %x", fd.Tls_version_chosen)
		}
	}
}

func (di *DriverInterface) getFlowConnectionStats(ioctl uint32, connbuffer *driverReadBuffer, outbuffer *ConnectionBuffer, filter func(*ConnectionStats) bool) (int, error, int, int) {

	start := outbuffer.Len()

	increases := int(0)
	decreases := int(0)
	var bytesRead uint32
	var totalBytesRead uint32

	// keep reading while driver says there is more data available
	for err := error(windows.ERROR_MORE_DATA); err == windows.ERROR_MORE_DATA; {
		err = di.driverFlowHandle.DeviceIoControl(ioctl, nil, 0,
			(*byte)(unsafe.Pointer(&((*connbuffer)[0]))),
			uint32(len(*connbuffer)),
			&bytesRead, nil)
		if err != nil {
			if err == windows.ERROR_NO_MORE_ITEMS {
				break
			}
			if err != windows.ERROR_MORE_DATA {
				return 0, fmt.Errorf("ReadFile: %w", err), 0, 0
			}
		}
		// Windows driver hashmap implementation could return this if the
		// provided buffer is too small to contain all entries in one of
		// the hashmap's linkedlists
		if bytesRead == 0 && err == windows.ERROR_MORE_DATA {
			//log.Warnf("Buffer not large enough for hash bucket")
			//return 0, fmt.Errorf("Buffer not large enough for hash bucket"), 0, 0
			connbuffer.resizeDriverBuffer(len(*connbuffer) * 2)
			increases++
			continue
		}
		totalBytesRead += bytesRead

		var buf []byte
		for bytesUsed := uint32(0); bytesUsed < bytesRead; bytesUsed += driver.PerFlowDataSize {
			buf = (*connbuffer)[bytesUsed:]
			pfd := (*driver.PerFlowData)(unsafe.Pointer(&(buf[0])))
			c := outbuffer.Next()
			FlowToConnStat(c, pfd, di.enableMonotonicCounts)
			if !filter(c) {
				outbuffer.Reclaim(1)
				continue
			}
		}
		resized := connbuffer.resizeDriverBuffer(int(totalBytesRead))
		switch resized {
		case ResizedIncreased:
			increases++
		case ResizedDecreased:
			decreases++
		case ResizedUnchanged:
			fallthrough
		default:
			// do nothing
			break
		}
	}
	count := outbuffer.Len() - start
	return count, nil, increases, decreases
}

// GetConnectionStats will read all open flows from the driver and convert them into ConnectionStats.
// It returns the count of connections added to the active and closed buffers, respectively.
func (di *DriverInterface) GetOpenConnectionStats(openBuf *ConnectionBuffer, filter func(*ConnectionStats) bool) (int, error) {
	di.openBufferLock.Lock()
	defer di.openBufferLock.Unlock()

	count, err, increases, decreases := di.getFlowConnectionStats(driver.GetOpenFlowsIOCTL, &(di.openBuffer), openBuf, filter)
	if err != nil {
		return 0, err
	}
	di.openFlows.Add(int64(count))
	di.totalFlows.Add(int64(count))

	di.nOpenBufferIncreases.Add(int64(increases))
	di.nOpenBufferDecreases.Add(int64(decreases))
	return count, err

}

// GetConnectionStats will read all closed from the driver and convert them into ConnectionStats.
// It returns the count of connections added to the active and closed buffers, respectively.
func (di *DriverInterface) GetClosedConnectionStats(closedBuf *ConnectionBuffer, filter func(*ConnectionStats) bool) (int, error) {
	di.closedBufferLock.Lock()
	defer di.closedBufferLock.Unlock()

	count, err, increases, decreases := di.getFlowConnectionStats(driver.GetClosedFlowsIOCTL, &(di.closedBuffer), closedBuf, filter)
	if err != nil {
		return 0, err
	}
	di.closedFlows.Add(int64(count))
	di.totalFlows.Add(int64(count))
	di.nClosedBufferIncreases.Add(int64(increases))
	di.nClosedBufferDecreases.Add(int64(decreases))

	return count, err
}

func (db *driverReadBuffer) resizeDriverBuffer(compareSize int) driverResizeResult {
	// Explicitly setting len to 0 causes the ReadFile syscall to break, so allocate buffer with cap = len
	origcap := cap(*db)
	if compareSize >= origcap*2 {
		*db = make([]uint8, origcap*2)
		return ResizedIncreased
	} else if compareSize <= origcap/2 {
		// Take the max of driverReadBuffer/2 and compareSize to limit future array resizes
		*db = make([]uint8, int(math.Max(float64(origcap/2), float64(compareSize))))
		return ResizedDecreased
	}
	// else
	return ResizedUnchanged
}

func minUint64(a, b uint64) uint64 {
	if a > b {
		return b
	}
	return a
}

func (di *DriverInterface) setFlowParams() error {
	// set up the maximum flows

	// temporary setup.  Will set the maximum flows to the sum of the configured
	// max_tracked_connections and max_closed_connections_buffered, setting a
	// (hard_coded) maximum.  This will be updated to actually honor the separate
	// config values when the driver is updated to track them separately.

	// this makes it so that the config can clamp down, but can never make it
	// larger than the coded defaults above.
	maxOpenFlows := minUint64(defaultMaxOpenFlows, di.maxOpenFlows)
	maxClosedFlows := minUint64(defaultMaxClosedFlows, di.maxClosedFlows)

	err := di.driverFlowHandle.DeviceIoControl(
		driver.SetMaxOpenFlowsIOCTL,
		(*byte)(unsafe.Pointer(&maxOpenFlows)),
		uint32(unsafe.Sizeof(maxOpenFlows)),
		nil,
		uint32(0), nil, nil)
	if err != nil {
		log.Warnf("Failed to set max number of open flows to %v %v", maxOpenFlows, err)
	}
	err = di.driverFlowHandle.DeviceIoControl(
		driver.SetMaxClosedFlowsIOCTL,
		(*byte)(unsafe.Pointer(&maxClosedFlows)),
		uint32(unsafe.Sizeof(maxClosedFlows)),
		nil,
		uint32(0), nil, nil)
	if err != nil {
		log.Warnf("Failed to set max number of closed flows to %v %v", maxClosedFlows, err)
	}

	threshold := di.closedFlowsSignalLimit
	if 0 == threshold {
		threshold = maxClosedFlows / 2
	}
	err = di.driverFlowHandle.DeviceIoControl(
		driver.SetClosedFlowsLimitIOCTL,
		(*byte)(unsafe.Pointer(&threshold)),
		uint32(unsafe.Sizeof(threshold)),
		nil,
		uint32(0), nil, nil)
	if err != nil {
		log.Warnf("Failed to set closed flows threshold to %v %v", maxClosedFlows, err)
	}
	return err
}

func (di *DriverInterface) createFlowHandleFilters() ([]driver.FilterDefinition, error) {
	var filters []driver.FilterDefinition
	log.Debugf("Creating filters for all interfaces")
	if di.cfg.CollectTCPv4Conns {
		filters = append(filters, driver.FilterDefinition{
			FilterVersion:  driver.Signature,
			Size:           driver.FilterDefinitionSize,
			Direction:      driver.DirectionOutbound,
			FilterLayer:    driver.LayerTransport,
			InterfaceIndex: uint64(0),
			Af:             windows.AF_INET,
			Protocol:       windows.IPPROTO_TCP,
		}, driver.FilterDefinition{
			FilterVersion:  driver.Signature,
			Size:           driver.FilterDefinitionSize,
			Direction:      driver.DirectionInbound,
			FilterLayer:    driver.LayerTransport,
			InterfaceIndex: uint64(0),
			Af:             windows.AF_INET,
			Protocol:       windows.IPPROTO_TCP,
		})
	}
	if di.cfg.CollectTCPv6Conns {
		filters = append(filters, driver.FilterDefinition{
			FilterVersion:  driver.Signature,
			Size:           driver.FilterDefinitionSize,
			Direction:      driver.DirectionOutbound,
			FilterLayer:    driver.LayerTransport,
			InterfaceIndex: uint64(0),
			Af:             windows.AF_INET6,
			Protocol:       windows.IPPROTO_TCP,
		}, driver.FilterDefinition{
			FilterVersion:  driver.Signature,
			Size:           driver.FilterDefinitionSize,
			Direction:      driver.DirectionInbound,
			FilterLayer:    driver.LayerTransport,
			InterfaceIndex: uint64(0),
			Af:             windows.AF_INET6,
			Protocol:       windows.IPPROTO_TCP,
		})
	}

	if di.cfg.CollectUDPv4Conns {
		filters = append(filters, driver.FilterDefinition{
			FilterVersion:  driver.Signature,
			Size:           driver.FilterDefinitionSize,
			Direction:      driver.DirectionOutbound,
			FilterLayer:    driver.LayerTransport,
			InterfaceIndex: uint64(0),
			Af:             windows.AF_INET,
			Protocol:       windows.IPPROTO_UDP,
		}, driver.FilterDefinition{
			FilterVersion:  driver.Signature,
			Size:           driver.FilterDefinitionSize,
			Direction:      driver.DirectionInbound,
			FilterLayer:    driver.LayerTransport,
			InterfaceIndex: uint64(0),
			Af:             windows.AF_INET,
			Protocol:       windows.IPPROTO_UDP,
		})
	}
	if di.cfg.CollectUDPv6Conns {
		filters = append(filters, driver.FilterDefinition{
			FilterVersion:  driver.Signature,
			Size:           driver.FilterDefinitionSize,
			Direction:      driver.DirectionOutbound,
			FilterLayer:    driver.LayerTransport,
			InterfaceIndex: uint64(0),
			Af:             windows.AF_INET6,
			Protocol:       windows.IPPROTO_UDP,
		}, driver.FilterDefinition{
			FilterVersion:  driver.Signature,
			Size:           driver.FilterDefinitionSize,
			Direction:      driver.DirectionInbound,
			FilterLayer:    driver.LayerTransport,
			InterfaceIndex: uint64(0),
			Af:             windows.AF_INET6,
			Protocol:       windows.IPPROTO_UDP,
		})
	}

	return filters, nil
}

// GetClosedFlowsEvent returns the base Windows handle for the event
// that gets signalled whenever the number of closed flows exceeds
// the configured amount.
func (di *DriverInterface) GetClosedFlowsEvent() windows.Handle {
	return di.closeFlowEvent
}
