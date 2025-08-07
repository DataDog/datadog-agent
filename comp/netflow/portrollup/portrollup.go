// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package portrollup provides a type for tracking observed connections
// between ports on different devices and identifying when a port connects
// to many different ports and so should have all traffic rolled up into a
// single flow for reporting purposes.
package portrollup

import (
	"slices"
	"sync"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

// EphemeralPort port number is represented by `-1` internally
const EphemeralPort int32 = -1

// IsEphemeralStatus enum type
type IsEphemeralStatus int32

const (
	// NoEphemeralPort both source port and destination are not ephemeral
	NoEphemeralPort = IsEphemeralStatus(0)
	// IsEphemeralSourcePort represent whether source port is ephemeral
	IsEphemeralSourcePort = IsEphemeralStatus(1)
	// IsEphemeralDestPort represent whether destination port is ephemeral
	IsEphemeralDestPort = IsEphemeralStatus(2)
)

// endpointType is source or destination
type endpointType int8

const (
	isSourceEndpoint      = endpointType(0)
	isDestinationEndpoint = endpointType(1)
)

type portsAndActiveFlag struct {
	ports  []uint16
	active bool // indicates whether the ports are active (seen in the last portRollupThreshold seconds)
}

// EndpointPairPortRollupStore contains port rollup states.
// It tracks ports that have been seen so far and help decide whether a port should be rolled up or not.
// We use two stores (curStore, newStore) to be able to clean old tracked ports when they are not seen anymore.
// Adding a port will double write to curStore and newStore. This means a port is tracked for `2 * portRollupThreshold` seconds.
// When IsEphemeral is called, only curStore is used.
// UseNewStoreAsCurrentStore is meant to be called externally to use new store as current store and empty the new store.
type EndpointPairPortRollupStore struct {
	portRollupThreshold int
	useFixedSizeKey     bool
	useSingleStore      bool // Added field for single store mode

	// Original string-based stores
	// We might also use map[uint16]struct to store ports, but using []uint16 takes less mem.
	// - Empty map is about 128 bytes
	// - Empty list is about 24 bytes
	// - It's more costly to search in a list, but the number of expected entry is at most equal to `portRollupThreshold`.
	curStore    map[string][]uint16
	newStore    map[string][]uint16
	singleStore map[string]*portsAndActiveFlag // Used when useSingleStore is true

	// Fixed-size key stores for IPv4 (11 bytes: 4+4+1+2)
	curStoreIPv4    map[[11]byte][]uint16
	newStoreIPv4    map[[11]byte][]uint16
	singleStoreIPv4 map[[11]byte]*portsAndActiveFlag // Used when useSingleStore is true

	// mutex used to protect access to stores
	storeMu sync.RWMutex

	// Track IPv6 warnings to avoid spam
	ipv6WarningLogged bool

	// logger component for logging
	logger log.Component

	// Call counter for logging store sizes
	callCounter uint64

	// Logging interval for store sizes (0 disables logging)
	logMapSizesEveryN int
}

// NewEndpointPairPortRollupStore create a new *EndpointPairPortRollupStore
func NewEndpointPairPortRollupStore(portRollupThreshold int, useFixedSizeKey bool, useSingleStore bool, logMapSizesEveryN int, logger log.Component) *EndpointPairPortRollupStore {
	store := &EndpointPairPortRollupStore{
		portRollupThreshold: portRollupThreshold,
		useFixedSizeKey:     useFixedSizeKey,
		useSingleStore:      useSingleStore,
		logMapSizesEveryN:   logMapSizesEveryN,
		ipv6WarningLogged:   false,
		logger:              logger,
	}

	if useSingleStore {
		if useFixedSizeKey {
			store.singleStoreIPv4 = make(map[[11]byte]*portsAndActiveFlag)
		} else {
			store.singleStore = make(map[string]*portsAndActiveFlag)
		}
	} else {
		if useFixedSizeKey {
			store.curStoreIPv4 = make(map[[11]byte][]uint16)
			store.newStoreIPv4 = make(map[[11]byte][]uint16)
		} else {
			// curStore and newStore map key is composed of `<SOURCE_IP>|<DESTINATION_IP>`
			// SOURCE_IP and SOURCE_IP are converted from []byte to string.
			// string is used as map key since we can't use []byte as map key.
			store.curStore = make(map[string][]uint16)
			store.newStore = make(map[string][]uint16)
		}
	}

	return store
}

// Add will record new sourcePort and destPort for a specific sourceAddr and destAddr
func (prs *EndpointPairPortRollupStore) Add(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) {
	if prs.useFixedSizeKey {
		prs.addWithFixedSizeKey(sourceAddr, destAddr, sourcePort, destPort)
	} else {
		prs.addWithStringKey(sourceAddr, destAddr, sourcePort, destPort)
	}
}

// addWithStringKey implements the original string-based approach
func (prs *EndpointPairPortRollupStore) addWithStringKey(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) {
	srcToDestKey := buildStoreKey(sourceAddr, destAddr, isSourceEndpoint, sourcePort)
	destToSrcKey := buildStoreKey(sourceAddr, destAddr, isDestinationEndpoint, destPort)

	if prs.useSingleStore {
		prs.AddToSingleStore(srcToDestKey, destToSrcKey, sourceAddr, destAddr, sourcePort, destPort)
	} else {
		prs.AddToStore(prs.curStore, srcToDestKey, destToSrcKey, sourceAddr, destAddr, sourcePort, destPort, NoEphemeralPort)

		// We pass isEphemeralStatus here to avoid writing to newStore if a port is known to be ephemeral already in curStore
		prs.AddToStore(prs.newStore, srcToDestKey, destToSrcKey, sourceAddr, destAddr, sourcePort, destPort, prs.IsEphemeralFromKeys(srcToDestKey, destToSrcKey))
	}
}

// addWithFixedSizeKey implements the new fixed-size key approach for IPv4 only
func (prs *EndpointPairPortRollupStore) addWithFixedSizeKey(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) {
	// Check for IPv6 and skip processing
	if len(sourceAddr) != 4 || len(destAddr) != 4 {
		if !prs.ipv6WarningLogged {
			prs.logger.Infof("WARNING: IPv6 flows detected but fixed-size key mode only supports IPv4. Skipping flow processing.")
			prs.ipv6WarningLogged = true
		}
		return
	}

	srcToDestKey := buildIPv4StoreKey(sourceAddr, destAddr, isSourceEndpoint, sourcePort)
	destToSrcKey := buildIPv4StoreKey(sourceAddr, destAddr, isDestinationEndpoint, destPort)

	if prs.useSingleStore {
		prs.AddToSingleStoreIPv4(srcToDestKey, destToSrcKey, sourceAddr, destAddr, sourcePort, destPort)
	} else {
		prs.AddToStoreIPv4(prs.curStoreIPv4, srcToDestKey, destToSrcKey, sourceAddr, destAddr, sourcePort, destPort, NoEphemeralPort)

		// We pass isEphemeralStatus here to avoid writing to newStore if a port is known to be ephemeral already in curStore
		prs.AddToStoreIPv4(prs.newStoreIPv4, srcToDestKey, destToSrcKey, sourceAddr, destAddr, sourcePort, destPort, prs.IsEphemeralFromKeysIPv4(srcToDestKey, destToSrcKey))
	}
}

// AddToStore will add ports to store
func (prs *EndpointPairPortRollupStore) AddToStore(store map[string][]uint16, srcToDestKey string, destToSrcKey string, sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16, curStoreIsEphemeralStatus IsEphemeralStatus) {
	prs.storeMu.Lock()

	// Increment call counter and log store sizes
	prs.callCounter++
	if prs.logMapSizesEveryN > 0 && prs.callCounter%uint64(prs.logMapSizesEveryN) == 0 {
		curStoreSize := common.Sizeof(prs.curStore)
		newStoreSize := common.Sizeof(prs.newStore)
		curStoreLen := len(prs.curStore)
		newStoreLen := len(prs.newStore)

		var curAvgSize, newAvgSize float64
		if curStoreLen > 0 {
			curAvgSize = float64(curStoreSize) / float64(curStoreLen)
		}
		if newStoreLen > 0 {
			newAvgSize = float64(newStoreSize) / float64(newStoreLen)
		}

		prs.logger.Infof("After %d calls - curStore: %d elements (%d bytes, %.1f bytes/entry)",
			prs.callCounter, curStoreLen, curStoreSize, curAvgSize)
		prs.logger.Infof("After %d calls - newStore: %d elements (%d bytes, %.1f bytes/entry)",
			prs.callCounter, newStoreLen, newStoreSize, newAvgSize)
	}

	sourceToDestPorts := len(store[srcToDestKey])
	destToSourcePorts := len(store[destToSrcKey])

	// either source or dest port is already ephemeral
	if sourceToDestPorts >= prs.portRollupThreshold || destToSourcePorts >= prs.portRollupThreshold {
		prs.storeMu.Unlock()
		return
	}
	if destToSourcePorts+1 < prs.portRollupThreshold && curStoreIsEphemeralStatus != IsEphemeralSourcePort {
		store[srcToDestKey] = appendPort(store[srcToDestKey], destPort)
	}
	// if the destination port is ephemeral, we can delete the corresponding destToSrc entries
	if len(store[srcToDestKey]) >= prs.portRollupThreshold {
		for _, port := range store[srcToDestKey] {
			delete(store, buildStoreKey(sourceAddr, destAddr, isDestinationEndpoint, port))
		}
	}

	if sourceToDestPorts+1 < prs.portRollupThreshold && curStoreIsEphemeralStatus != IsEphemeralDestPort {
		store[destToSrcKey] = appendPort(store[destToSrcKey], sourcePort)
	}
	// if the source port is ephemeral, we can delete the corresponding srcToDest entries
	if len(store[destToSrcKey]) >= prs.portRollupThreshold {
		for _, port := range store[destToSrcKey] {
			delete(store, buildStoreKey(sourceAddr, destAddr, isSourceEndpoint, port))
		}
	}

	prs.storeMu.Unlock()
}

// AddToStoreIPv4 will add ports to store using fixed-size key for IPv4
func (prs *EndpointPairPortRollupStore) AddToStoreIPv4(store map[[11]byte][]uint16, srcToDestKey [11]byte, destToSrcKey [11]byte, sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16, curStoreIsEphemeralStatus IsEphemeralStatus) {
	prs.storeMu.Lock()

	// Increment call counter and log store sizes
	prs.callCounter++
	if prs.logMapSizesEveryN > 0 && prs.callCounter%uint64(prs.logMapSizesEveryN) == 0 {
		curStoreSize := common.Sizeof(prs.curStoreIPv4)
		newStoreSize := common.Sizeof(prs.newStoreIPv4)
		curStoreLen := len(prs.curStoreIPv4)
		newStoreLen := len(prs.newStoreIPv4)

		var curAvgSize, newAvgSize float64
		if curStoreLen > 0 {
			curAvgSize = float64(curStoreSize) / float64(curStoreLen)
		}
		if newStoreLen > 0 {
			newAvgSize = float64(newStoreSize) / float64(newStoreLen)
		}

		prs.logger.Infof("After %d calls - curStoreIPv4: %d elements (%d bytes, %.1f bytes/entry)",
			prs.callCounter, curStoreLen, curStoreSize, curAvgSize)
		prs.logger.Infof("After %d calls - newStoreIPv4: %d elements (%d bytes, %.1f bytes/entry)",
			prs.callCounter, newStoreLen, newStoreSize, newAvgSize)
	}

	sourceToDestPorts := len(store[srcToDestKey])
	destToSourcePorts := len(store[destToSrcKey])

	// either source or dest port is already ephemeral
	if sourceToDestPorts >= prs.portRollupThreshold || destToSourcePorts >= prs.portRollupThreshold {
		prs.storeMu.Unlock()
		return
	}

	if destToSourcePorts+1 < prs.portRollupThreshold && curStoreIsEphemeralStatus != IsEphemeralSourcePort {
		store[srcToDestKey] = appendPort(store[srcToDestKey], destPort)
	}
	// if the destination port is ephemeral, we can delete the corresponding destToSrc entries
	if len(store[srcToDestKey]) >= prs.portRollupThreshold {
		for _, port := range store[srcToDestKey] {
			delete(store, buildIPv4StoreKey(sourceAddr, destAddr, isDestinationEndpoint, port))
		}
	}

	if sourceToDestPorts+1 < prs.portRollupThreshold && curStoreIsEphemeralStatus != IsEphemeralDestPort {
		store[destToSrcKey] = appendPort(store[destToSrcKey], sourcePort)
	}
	// if the source port is ephemeral, we can delete the corresponding srcToDest entries
	if len(store[destToSrcKey]) >= prs.portRollupThreshold {
		for _, port := range store[destToSrcKey] {
			delete(store, buildIPv4StoreKey(sourceAddr, destAddr, isSourceEndpoint, port))
		}
	}

	prs.storeMu.Unlock()
}

// AddToSingleStore will add ports to single store for string keys
func (prs *EndpointPairPortRollupStore) AddToSingleStore(srcToDestKey string, destToSrcKey string, sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) {
	prs.storeMu.Lock()
	defer prs.storeMu.Unlock()

	// Increment call counter and log store sizes
	prs.callCounter++
	if prs.logMapSizesEveryN > 0 && prs.callCounter%uint64(prs.logMapSizesEveryN) == 0 {
		singleStoreSize := common.Sizeof(prs.singleStore)
		singleStoreLen := len(prs.singleStore)

		var singleAvgSize float64
		if singleStoreLen > 0 {
			singleAvgSize = float64(singleStoreSize) / float64(singleStoreLen)
		}

		prs.logger.Infof("After %d calls - singleStore: %d elements (%d bytes, %.1f bytes/entry)",
			prs.callCounter, singleStoreLen, singleStoreSize, singleAvgSize)
	}

	// Check existing entries first to see if either source or dest port is already ephemeral
	var sourceToDestPorts, destToSourcePorts int
	if srcToDest, exists := prs.singleStore[srcToDestKey]; exists {
		sourceToDestPorts = len(srcToDest.ports)
	}
	if destToSrc, exists := prs.singleStore[destToSrcKey]; exists {
		destToSourcePorts = len(destToSrc.ports)
	}

	// either source or dest port is already ephemeral
	if sourceToDestPorts >= prs.portRollupThreshold || destToSourcePorts >= prs.portRollupThreshold {
		return
	}

	// Get or create source-to-dest entry (only if not ephemeral)
	srcToDest, srcToDestExists := prs.singleStore[srcToDestKey]
	if !srcToDestExists {
		srcToDest = &portsAndActiveFlag{ports: []uint16{}, active: true}
		prs.singleStore[srcToDestKey] = srcToDest
	}
	srcToDest.active = true

	// Get or create dest-to-source entry (only if not ephemeral)
	destToSrc, destToSrcExists := prs.singleStore[destToSrcKey]
	if !destToSrcExists {
		destToSrc = &portsAndActiveFlag{ports: []uint16{}, active: true}
		prs.singleStore[destToSrcKey] = destToSrc
	}
	destToSrc.active = true

	if destToSourcePorts+1 < prs.portRollupThreshold {
		srcToDest.ports = appendPort(srcToDest.ports, destPort)
	}
	// if the destination port is ephemeral, we can delete the corresponding destToSrc entries
	if len(srcToDest.ports) >= prs.portRollupThreshold {
		for _, port := range srcToDest.ports {
			delete(prs.singleStore, buildStoreKey(sourceAddr, destAddr, isDestinationEndpoint, port))
		}
	}

	if sourceToDestPorts+1 < prs.portRollupThreshold {
		destToSrc.ports = appendPort(destToSrc.ports, sourcePort)
	}
	// if the source port is ephemeral, we can delete the corresponding srcToDest entries
	if len(destToSrc.ports) >= prs.portRollupThreshold {
		for _, port := range destToSrc.ports {
			delete(prs.singleStore, buildStoreKey(sourceAddr, destAddr, isSourceEndpoint, port))
		}
	}
}

// AddToSingleStoreIPv4 will add ports to single store for IPv4 fixed-size keys
func (prs *EndpointPairPortRollupStore) AddToSingleStoreIPv4(srcToDestKey [11]byte, destToSrcKey [11]byte, sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) {
	prs.storeMu.Lock()
	defer prs.storeMu.Unlock()

	// Increment call counter and log store sizes
	prs.callCounter++
	if prs.logMapSizesEveryN > 0 && prs.callCounter%uint64(prs.logMapSizesEveryN) == 0 {
		singleStoreSize := common.Sizeof(prs.singleStoreIPv4)
		singleStoreLen := len(prs.singleStoreIPv4)

		var singleAvgSize float64
		if singleStoreLen > 0 {
			singleAvgSize = float64(singleStoreSize) / float64(singleStoreLen)
		}

		prs.logger.Infof("After %d calls - singleStoreIPv4: %d elements (%d bytes, %.1f bytes/entry)",
			prs.callCounter, singleStoreLen, singleStoreSize, singleAvgSize)
	}

	// Check existing entries first to see if either source or dest port is already ephemeral
	var sourceToDestPorts, destToSourcePorts int
	if srcToDest, exists := prs.singleStoreIPv4[srcToDestKey]; exists {
		sourceToDestPorts = len(srcToDest.ports)
	}
	if destToSrc, exists := prs.singleStoreIPv4[destToSrcKey]; exists {
		destToSourcePorts = len(destToSrc.ports)
	}

	// either source or dest port is already ephemeral
	if sourceToDestPorts >= prs.portRollupThreshold || destToSourcePorts >= prs.portRollupThreshold {
		return
	}

	// Get or create source-to-dest entry (only if not ephemeral)
	srcToDest, srcToDestExists := prs.singleStoreIPv4[srcToDestKey]
	if !srcToDestExists {
		srcToDest = &portsAndActiveFlag{ports: []uint16{}, active: true}
		prs.singleStoreIPv4[srcToDestKey] = srcToDest
	}
	srcToDest.active = true

	// Get or create dest-to-source entry (only if not ephemeral)
	destToSrc, destToSrcExists := prs.singleStoreIPv4[destToSrcKey]
	if !destToSrcExists {
		destToSrc = &portsAndActiveFlag{ports: []uint16{}, active: true}
		prs.singleStoreIPv4[destToSrcKey] = destToSrc
	}
	destToSrc.active = true

	if destToSourcePorts+1 < prs.portRollupThreshold {
		srcToDest.ports = appendPort(srcToDest.ports, destPort)
	}
	// if the destination port is ephemeral, we can delete the corresponding destToSrc entries
	if len(srcToDest.ports) >= prs.portRollupThreshold {
		for _, port := range srcToDest.ports {
			delete(prs.singleStoreIPv4, buildIPv4StoreKey(sourceAddr, destAddr, isDestinationEndpoint, port))
		}
	}

	if sourceToDestPorts+1 < prs.portRollupThreshold {
		destToSrc.ports = appendPort(destToSrc.ports, sourcePort)
	}
	// if the source port is ephemeral, we can delete the corresponding srcToDest entries
	if len(destToSrc.ports) >= prs.portRollupThreshold {
		for _, port := range destToSrc.ports {
			delete(prs.singleStoreIPv4, buildIPv4StoreKey(sourceAddr, destAddr, isSourceEndpoint, port))
		}
	}
}

// GetPortCount returns max port count and indicate whether the source or destination is ephemeral (isEphemeralSource)
func (prs *EndpointPairPortRollupStore) GetPortCount(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) (uint16, bool) {
	if prs.useFixedSizeKey {
		return prs.GetPortCountIPv4(sourceAddr, destAddr, sourcePort, destPort)
	}
	return prs.GetPortCountString(sourceAddr, destAddr, sourcePort, destPort)
}

// IsEphemeral checks if source port and destination port are ephemeral
func (prs *EndpointPairPortRollupStore) IsEphemeral(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) IsEphemeralStatus {
	if prs.useFixedSizeKey {
		return prs.IsEphemeralIPv4(sourceAddr, destAddr, sourcePort, destPort)
	}
	return prs.IsEphemeralString(sourceAddr, destAddr, sourcePort, destPort)
}

// IsEphemeralFromKeys gets the ephemeral status of a link based on its keys.
func (prs *EndpointPairPortRollupStore) IsEphemeralFromKeys(srcToDestKey string, destToSrcKey string) IsEphemeralStatus {
	prs.storeMu.RLock()
	var sourceToDestPortCount, destToSourcePortCount int

	if prs.useSingleStore {
		if srcToDest, exists := prs.singleStore[srcToDestKey]; exists {
			sourceToDestPortCount = len(srcToDest.ports)
		}
		if destToSrc, exists := prs.singleStore[destToSrcKey]; exists {
			destToSourcePortCount = len(destToSrc.ports)
		}
	} else {
		sourceToDestPortCount = len(prs.curStore[srcToDestKey])
		destToSourcePortCount = len(prs.curStore[destToSrcKey])
	}
	prs.storeMu.RUnlock()

	portCount := max(destToSourcePortCount, sourceToDestPortCount)

	if portCount < prs.portRollupThreshold {
		return NoEphemeralPort
	}

	isEphemeralSource := destToSourcePortCount > sourceToDestPortCount
	// we rollup either source port and destination.
	// we assume that there is no case where both source and destination ports are ephemeral.
	if isEphemeralSource { // rollup ephemeral source port
		return IsEphemeralSourcePort
	}
	return IsEphemeralDestPort
}

// IsEphemeralFromKeysIPv4 gets the ephemeral status of a link based on its keys for IPv4.
func (prs *EndpointPairPortRollupStore) IsEphemeralFromKeysIPv4(srcToDestKey [11]byte, destToSrcKey [11]byte) IsEphemeralStatus {
	prs.storeMu.RLock()
	var sourceToDestPortCount, destToSourcePortCount int

	if prs.useSingleStore {
		if srcToDest, exists := prs.singleStoreIPv4[srcToDestKey]; exists {
			sourceToDestPortCount = len(srcToDest.ports)
		}
		if destToSrc, exists := prs.singleStoreIPv4[destToSrcKey]; exists {
			destToSourcePortCount = len(destToSrc.ports)
		}
	} else {
		sourceToDestPortCount = len(prs.curStoreIPv4[srcToDestKey])
		destToSourcePortCount = len(prs.curStoreIPv4[destToSrcKey])
	}
	prs.storeMu.RUnlock()

	portCount := max(destToSourcePortCount, sourceToDestPortCount)

	if portCount < prs.portRollupThreshold {
		return NoEphemeralPort
	}

	isEphemeralSource := destToSourcePortCount > sourceToDestPortCount
	// we rollup either source port and destination.
	// we assume that there is no case where both source and destination ports are ephemeral.
	if isEphemeralSource { // rollup ephemeral source port
		return IsEphemeralSourcePort
	}
	return IsEphemeralDestPort
}

// GetSourceToDestPortCount returns the number of different destination port for a specific source port
func (prs *EndpointPairPortRollupStore) GetSourceToDestPortCount(sourceAddr []byte, destAddr []byte, sourcePort uint16) uint16 {
	if prs.useFixedSizeKey {
		return prs.GetSourceToDestPortCountIPv4(sourceAddr, destAddr, sourcePort)
	}
	return prs.GetSourceToDestPortCountString(sourceAddr, destAddr, sourcePort)
}

// GetDestToSourcePortCount returns the number of different source port for a specific destination port
func (prs *EndpointPairPortRollupStore) GetDestToSourcePortCount(sourceAddr []byte, destAddr []byte, destPort uint16) uint16 {
	if prs.useFixedSizeKey {
		return prs.GetDestToSourcePortCountIPv4(sourceAddr, destAddr, destPort)
	}
	return prs.GetDestToSourcePortCountString(sourceAddr, destAddr, destPort)
}

// GetPortCountString returns max port count and indicate whether the source or destination is ephemeral (isEphemeralSource) for string keys
func (prs *EndpointPairPortRollupStore) GetPortCountString(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) (uint16, bool) {
	prs.storeMu.RLock()
	defer prs.storeMu.RUnlock()

	var sourceToDestPortCount, destToSourcePortCount uint16

	if prs.useSingleStore {
		srcToDestKey := buildStoreKey(sourceAddr, destAddr, isSourceEndpoint, sourcePort)
		destToSrcKey := buildStoreKey(sourceAddr, destAddr, isDestinationEndpoint, destPort)

		if srcToDest, exists := prs.singleStore[srcToDestKey]; exists {
			sourceToDestPortCount = uint16(len(srcToDest.ports))
		}
		if destToSrc, exists := prs.singleStore[destToSrcKey]; exists {
			destToSourcePortCount = uint16(len(destToSrc.ports))
		}
	} else {
		sourceToDestPortCount = uint16(len(prs.curStore[buildStoreKey(sourceAddr, destAddr, isSourceEndpoint, sourcePort)]))
		destToSourcePortCount = uint16(len(prs.curStore[buildStoreKey(sourceAddr, destAddr, isDestinationEndpoint, destPort)]))
	}

	portCount := common.Max(sourceToDestPortCount, destToSourcePortCount)
	isEphemeralSource := destToSourcePortCount > sourceToDestPortCount
	return portCount, isEphemeralSource
}

// GetSourceToDestPortCountString returns the number of different destination port for a specific source port for string keys
func (prs *EndpointPairPortRollupStore) GetSourceToDestPortCountString(sourceAddr []byte, destAddr []byte, sourcePort uint16) uint16 {
	prs.storeMu.RLock()
	defer prs.storeMu.RUnlock()

	if prs.useSingleStore {
		srcToDestKey := buildStoreKey(sourceAddr, destAddr, isSourceEndpoint, sourcePort)
		if srcToDest, exists := prs.singleStore[srcToDestKey]; exists {
			return uint16(len(srcToDest.ports))
		}
		return 0
	}

	return uint16(len(prs.curStore[buildStoreKey(sourceAddr, destAddr, isSourceEndpoint, sourcePort)]))
}

// GetDestToSourcePortCountString returns the number of different source port for a specific destination port for string keys
func (prs *EndpointPairPortRollupStore) GetDestToSourcePortCountString(sourceAddr []byte, destAddr []byte, destPort uint16) uint16 {
	prs.storeMu.RLock()
	defer prs.storeMu.RUnlock()

	if prs.useSingleStore {
		destToSrcKey := buildStoreKey(sourceAddr, destAddr, isDestinationEndpoint, destPort)
		if destToSrc, exists := prs.singleStore[destToSrcKey]; exists {
			return uint16(len(destToSrc.ports))
		}
		return 0
	}

	return uint16(len(prs.curStore[buildStoreKey(sourceAddr, destAddr, isDestinationEndpoint, destPort)]))
}

// GetPortCountIPv4 returns max port count and indicate whether the source or destination is ephemeral (isEphemeralSource) for IPv4
func (prs *EndpointPairPortRollupStore) GetPortCountIPv4(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) (uint16, bool) {
	prs.storeMu.RLock()
	defer prs.storeMu.RUnlock()

	var sourceToDestPortCount, destToSourcePortCount uint16

	if prs.useSingleStore {
		srcToDestKey := buildIPv4StoreKey(sourceAddr, destAddr, isSourceEndpoint, sourcePort)
		destToSrcKey := buildIPv4StoreKey(sourceAddr, destAddr, isDestinationEndpoint, destPort)

		if srcToDest, exists := prs.singleStoreIPv4[srcToDestKey]; exists {
			sourceToDestPortCount = uint16(len(srcToDest.ports))
		}
		if destToSrc, exists := prs.singleStoreIPv4[destToSrcKey]; exists {
			destToSourcePortCount = uint16(len(destToSrc.ports))
		}
	} else {
		sourceToDestPortCount = uint16(len(prs.curStoreIPv4[buildIPv4StoreKey(sourceAddr, destAddr, isSourceEndpoint, sourcePort)]))
		destToSourcePortCount = uint16(len(prs.curStoreIPv4[buildIPv4StoreKey(sourceAddr, destAddr, isDestinationEndpoint, destPort)]))
	}

	portCount := common.Max(sourceToDestPortCount, destToSourcePortCount)
	isEphemeralSource := destToSourcePortCount > sourceToDestPortCount
	return portCount, isEphemeralSource
}

// GetSourceToDestPortCountIPv4 returns the number of different destination port for a specific source port for IPv4
func (prs *EndpointPairPortRollupStore) GetSourceToDestPortCountIPv4(sourceAddr []byte, destAddr []byte, sourcePort uint16) uint16 {
	prs.storeMu.RLock()
	defer prs.storeMu.RUnlock()

	if prs.useSingleStore {
		srcToDestKey := buildIPv4StoreKey(sourceAddr, destAddr, isSourceEndpoint, sourcePort)
		if srcToDest, exists := prs.singleStoreIPv4[srcToDestKey]; exists {
			return uint16(len(srcToDest.ports))
		}
		return 0
	}

	return uint16(len(prs.curStoreIPv4[buildIPv4StoreKey(sourceAddr, destAddr, isSourceEndpoint, sourcePort)]))
}

// GetDestToSourcePortCountIPv4 returns the number of different source port for a specific destination port for IPv4
func (prs *EndpointPairPortRollupStore) GetDestToSourcePortCountIPv4(sourceAddr []byte, destAddr []byte, destPort uint16) uint16 {
	prs.storeMu.RLock()
	defer prs.storeMu.RUnlock()

	if prs.useSingleStore {
		destToSrcKey := buildIPv4StoreKey(sourceAddr, destAddr, isDestinationEndpoint, destPort)
		if destToSrc, exists := prs.singleStoreIPv4[destToSrcKey]; exists {
			return uint16(len(destToSrc.ports))
		}
		return 0
	}

	return uint16(len(prs.curStoreIPv4[buildIPv4StoreKey(sourceAddr, destAddr, isDestinationEndpoint, destPort)]))
}

// IsEphemeralString checks if source port and destination port are ephemeral for string keys
func (prs *EndpointPairPortRollupStore) IsEphemeralString(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) IsEphemeralStatus {
	srcToDestKey := buildStoreKey(sourceAddr, destAddr, isSourceEndpoint, sourcePort)
	destToSrcKey := buildStoreKey(sourceAddr, destAddr, isDestinationEndpoint, destPort)

	return prs.IsEphemeralFromKeys(srcToDestKey, destToSrcKey)
}

// IsEphemeralIPv4 checks if source port and destination port are ephemeral for IPv4
func (prs *EndpointPairPortRollupStore) IsEphemeralIPv4(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) IsEphemeralStatus {
	srcToDestKey := buildIPv4StoreKey(sourceAddr, destAddr, isSourceEndpoint, sourcePort)
	destToSrcKey := buildIPv4StoreKey(sourceAddr, destAddr, isDestinationEndpoint, destPort)

	return prs.IsEphemeralFromKeysIPv4(srcToDestKey, destToSrcKey)
}

// GetCurrentStoreSize get number of tracked port counters in current store
func (prs *EndpointPairPortRollupStore) GetCurrentStoreSize() int {
	if prs.useFixedSizeKey {
		return prs.GetCurrentStoreSizeIPv4()
	}
	return prs.GetCurrentStoreSizeString()
}

// GetNewStoreSize get number of tracked port counters in new store
func (prs *EndpointPairPortRollupStore) GetNewStoreSize() int {
	if prs.useFixedSizeKey {
		return prs.GetNewStoreSizeIPv4()
	}
	return prs.GetNewStoreSizeString()
}

// GetCurrentStoreSizeString get number of tracked port counters in current store for string keys
func (prs *EndpointPairPortRollupStore) GetCurrentStoreSizeString() int {
	prs.storeMu.RLock()
	defer prs.storeMu.RUnlock()
	if prs.useSingleStore {
		return len(prs.singleStore)
	}
	return len(prs.curStore)
}

// GetNewStoreSizeString get number of tracked port counters in new store for string keys
func (prs *EndpointPairPortRollupStore) GetNewStoreSizeString() int {
	prs.storeMu.RLock()
	defer prs.storeMu.RUnlock()
	if prs.useSingleStore {
		return 0 // single store doesn't have a separate new store
	}
	return len(prs.newStore)
}

// GetCurrentStoreSizeIPv4 get number of tracked port counters in current store for IPv4
func (prs *EndpointPairPortRollupStore) GetCurrentStoreSizeIPv4() int {
	prs.storeMu.RLock()
	defer prs.storeMu.RUnlock()
	if prs.useSingleStore {
		return len(prs.singleStoreIPv4)
	}
	return len(prs.curStoreIPv4)
}

// GetNewStoreSizeIPv4 get number of tracked port counters in new store for IPv4
func (prs *EndpointPairPortRollupStore) GetNewStoreSizeIPv4() int {
	prs.storeMu.RLock()
	defer prs.storeMu.RUnlock()
	if prs.useSingleStore {
		return 0 // single store doesn't have a separate new store
	}
	return len(prs.newStoreIPv4)
}

// UseNewStoreAsCurrentStore sets newStore to curStore and clean up newStore
func (prs *EndpointPairPortRollupStore) UseNewStoreAsCurrentStore() {
	if prs.useFixedSizeKey {
		prs.UseNewStoreAsCurrentStoreIPv4()
	} else {
		prs.UseNewStoreAsCurrentStoreString()
	}
}

// UseNewStoreAsCurrentStoreString sets newStore to curStore and clean up newStore for string keys
func (prs *EndpointPairPortRollupStore) UseNewStoreAsCurrentStoreString() {
	prs.storeMu.Lock()
	defer prs.storeMu.Unlock()

	if prs.useSingleStore {
		// iterate through all curStore entries
		for key, portsAndActive := range prs.singleStore {
			// delete entry for any that are not active and for any that are active mark as inactive for the next cycle
			if portsAndActive.active {
				// mark as inactive for the next cycle
				portsAndActive.active = false
			} else {
				// not seen in the last portRollupThreshold seconds, delete it
				delete(prs.singleStore, key)
			}
		}
		return
	}

	prs.curStore = prs.newStore
	prs.newStore = make(map[string][]uint16)
}

// UseNewStoreAsCurrentStoreIPv4 sets newStore to curStore and clean up newStore for IPv4
func (prs *EndpointPairPortRollupStore) UseNewStoreAsCurrentStoreIPv4() {
	prs.storeMu.Lock()
	defer prs.storeMu.Unlock()

	if prs.useSingleStore {
		// iterate through all curStoreIPv4 entries
		for key, portsAndActive := range prs.singleStoreIPv4 {
			// delete entry for any that are not active and for any that are active mark as inactive for the next cycle
			if portsAndActive.active {
				// mark as inactive for the next cycle
				portsAndActive.active = false
			} else {
				// not seen in the last portRollupThreshold seconds, delete it
				delete(prs.singleStoreIPv4, key)
			}
		}
		return
	}

	prs.curStoreIPv4 = prs.newStoreIPv4
	prs.newStoreIPv4 = make(map[[11]byte][]uint16)
}

// buildStoreKey will use the input data (sourceAddr, destAddr, endpoint type, port)
// and convert it to a key with `string` type. The actual elements of the key are in `[]byte`,
// but we cast them to `string` and concat into a single `string` to be able to use it as map key
// (`[]byte` is mutable and can't be used as map key).
func buildStoreKey(sourceAddr []byte, destAddr []byte, endpointT endpointType, port uint16) string {
	var portPart1, portPart2 = uint8(port >> 8), uint8(port & 0xff)
	return string(sourceAddr) + string(destAddr) + string([]byte{byte(endpointT)}) + string([]byte{portPart1, portPart2})
	// FOR DEBUGGING: You can replace above line with the following one for debugging, it makes the key easier to read
	// return common.IPBytesToString(sourceAddr) + "|" + common.IPBytesToString(destAddr) + "|" + fmt.Sprintf("%d", endpointT) + "|" + fmt.Sprintf("%d", port)
}

// buildIPv4StoreKey will use the input data (sourceAddr, destAddr, endpoint type, port)
// and convert it to a key with `[11]byte` type for IPv4.
func buildIPv4StoreKey(sourceAddr []byte, destAddr []byte, endpointT endpointType, port uint16) [11]byte {
	var key [11]byte
	copy(key[:4], sourceAddr)
	copy(key[4:8], destAddr)
	key[8] = byte(endpointT)
	var portPart1, portPart2 = uint8(port >> 8), uint8(port & 0xff)
	key[9] = portPart1
	key[10] = portPart2
	return key
}

func appendPort(ports []uint16, newPort uint16) []uint16 {
	if slices.Contains(ports, newPort) {
		return ports
	}
	return append(ports, newPort)
}
