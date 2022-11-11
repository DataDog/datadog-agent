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

// DriverInterface holds all necessary information for interacting with the windows driver
type DriverInterface struct {
	totalFlows       *atomic.Int64
	closedFlows      *atomic.Int64
	openFlows        *atomic.Int64
	moreDataErrors   *atomic.Int64
	bufferSize       *atomic.Int64
	nBufferIncreases *atomic.Int64
	nBufferDecreases *atomic.Int64

	maxOpenFlows   uint64
	maxClosedFlows uint64

	driverFlowHandle driver.Handle

	enableMonotonicCounts bool

	bufferLock sync.Mutex
	readBuffer []uint8

	cfg *config.Config
}

// Function pointer definition passed to NewDriverInterface that enables
// creating DriverInterfaces with varying handle types like ReadDriverHandle or
// TestDriverHandle*
type HandleCreateFn func(flags uint32, handleType driver.HandleType) (driver.Handle, error)

// NewDriverInterface returns a DriverInterface struct for interacting with the driver
func NewDriverInterface(cfg *config.Config, handleFunc HandleCreateFn) (*DriverInterface, error) {
	dc := &DriverInterface{
		totalFlows:       atomic.NewInt64(0),
		closedFlows:      atomic.NewInt64(0),
		openFlows:        atomic.NewInt64(0),
		moreDataErrors:   atomic.NewInt64(0),
		bufferSize:       atomic.NewInt64(defaultDriverBufferSize),
		nBufferIncreases: atomic.NewInt64(0),
		nBufferDecreases: atomic.NewInt64(0),

		cfg:                   cfg,
		enableMonotonicCounts: cfg.EnableMonotonicCount,
		readBuffer:            make([]byte, defaultDriverBufferSize),
		maxOpenFlows:          uint64(cfg.MaxTrackedConnections),
		maxClosedFlows:        uint64(cfg.MaxClosedConnectionsBuffered),
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
		return fmt.Errorf("error closing flow file handle: %w", err)
	}
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
	bufferSize := di.bufferSize.Load()
	nBufferIncreases := di.nBufferIncreases.Load()
	nBufferDecreases := di.nBufferDecreases.Load()

	return map[DriverExpvar]interface{}{
		flowHandleStats: stats["handle"],
		flowStats: map[string]int64{
			"total":  totalFlows,
			"open":   openFlows,
			"closed": closedFlows,
		},
		driverStats: map[string]int64{
			"more_data_errors": moreDataErrors,
			"buffer_size":      bufferSize,
			"buffer_increases": nBufferIncreases,
			"buffer_decreases": nBufferDecreases,
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

// GetConnectionStats will read all flows from the driver and convert them into ConnectionStats.
// It returns the count of connections added to the active and closed buffers, respectively.
func (di *DriverInterface) GetConnectionStats(activeBuf *ConnectionBuffer, closedBuf *ConnectionBuffer, filter func(*ConnectionStats) bool) (int, int, error) {
	di.bufferLock.Lock()
	defer di.bufferLock.Unlock()

	startActive, startClosed := activeBuf.Len(), closedBuf.Len()

	var bytesRead uint32
	var totalBytesRead uint32
	// keep reading while driver says there is more data available
	for err := error(windows.ERROR_MORE_DATA); err == windows.ERROR_MORE_DATA; {
		err = di.driverFlowHandle.ReadFile(di.readBuffer, &bytesRead, nil)
		if err != nil {
			if err == windows.ERROR_NO_MORE_ITEMS {
				break
			}
			if err != windows.ERROR_MORE_DATA {
				return 0, 0, fmt.Errorf("ReadFile: %w", err)
			}
			di.moreDataErrors.Inc()
		}

		// Windows driver hashmap implementation could return this if the
		// provided buffer is too small to contain all entries in one of
		// the hashmap's linkedlists
		if bytesRead == 0 && err == windows.ERROR_MORE_DATA {
			di.resizeDriverBuffer(cap(di.readBuffer) * 2)
			continue
		}
		totalBytesRead += bytesRead

		var buf []byte
		for bytesUsed := uint32(0); bytesUsed < bytesRead; bytesUsed += driver.PerFlowDataSize {
			buf = di.readBuffer[bytesUsed:]
			pfd := (*driver.PerFlowData)(unsafe.Pointer(&(buf[0])))

			//printClassification(pfd)
			if isFlowClosed(pfd.Flags) {
				c := closedBuf.Next()
				FlowToConnStat(c, pfd, di.enableMonotonicCounts)
				if !filter(c) {
					closedBuf.Reclaim(1)
					continue
				}
			} else {
				c := activeBuf.Next()
				FlowToConnStat(c, pfd, di.enableMonotonicCounts)
				if !filter(c) {
					activeBuf.Reclaim(1)
					continue
				}
			}
		}
		di.resizeDriverBuffer(int(totalBytesRead))
	}

	activeCount := activeBuf.Len() - startActive
	closedCount := closedBuf.Len() - startClosed
	di.openFlows.Add(int64(activeCount))
	di.closedFlows.Add(int64(closedCount))
	di.totalFlows.Add(int64(activeCount + closedCount))

	return activeCount, closedCount, nil
}

func (di *DriverInterface) resizeDriverBuffer(compareSize int) {
	// Explicitly setting len to 0 causes the ReadFile syscall to break, so allocate buffer with cap = len
	if compareSize >= cap(di.readBuffer)*2 {
		di.readBuffer = make([]uint8, cap(di.readBuffer)*2)
		di.nBufferIncreases.Inc()
		di.bufferSize.Store(int64(len(di.readBuffer)))
	} else if compareSize <= cap(di.readBuffer)/2 {
		// Take the max of di.readBuffer/2 and compareSize to limit future array resizes
		di.readBuffer = make([]uint8, int(math.Max(float64(cap(di.readBuffer)/2), float64(compareSize))))
		di.nBufferDecreases.Inc()
		di.bufferSize.Store(int64(len(di.readBuffer)))
	}
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
	return err
}

func (di *DriverInterface) createFlowHandleFilters() ([]driver.FilterDefinition, error) {
	var filters []driver.FilterDefinition
	log.Debugf("Creating filters for all interfaces")
	if di.cfg.CollectTCPConns {
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
		if di.cfg.CollectIPv6Conns {
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
	}

	if di.cfg.CollectUDPConns {
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
		if di.cfg.CollectIPv6Conns {
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
	}

	return filters, nil
}
