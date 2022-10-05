// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package portrollup

import (
	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	"sync"
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

// EndpointPairPortRollupStore contains port rollup states.
// It tracks ports that have been seen so far and help decide whether a port should be rolled up or not.
// We use two stores (curStore, newStore) to be able to clean old tracked ports when they are not seen anymore.
// Adding a port will double write to curStore and newStore. This means a port is tracked for `2 * portRollupThreshold` seconds.
// When IsEphemeral is called, only curStore is used.
// UseNewStoreAsCurrentStore is meant to be called externally to use new store as current store and empty the new store.
type EndpointPairPortRollupStore struct {
	portRollupThreshold int
	curStore            map[string][]uint16
	newStore            map[string][]uint16

	// mutex used to protect access to curStore and newStore
	mu sync.Mutex
}

// NewEndpointPairPortRollupStore create a new *EndpointPairPortRollupStore
func NewEndpointPairPortRollupStore(portRollupThreshold int) *EndpointPairPortRollupStore {
	return &EndpointPairPortRollupStore{
		// curStore and newStore map key is composed of `<SOURCE_IP>|<DESTINATION_IP>`
		// SOURCE_IP and SOURCE_IP are converted from []byte to string.
		// string is used as map key since we can't use []byte as map key.
		curStore: make(map[string][]uint16),
		newStore: make(map[string][]uint16),

		portRollupThreshold: portRollupThreshold,
	}
}

//func (prs *EndpointPairPortRollupStore) getOrCreate(sourceAddr []byte, destAddr []byte) (*portRollupTracker, *portRollupTracker) {
//	key := buildStoreKey(sourceAddr, destAddr)
//	if _, ok := prs.curStore[key]; !ok {
//		prs.curStore[key] = newPortRollupTracker()
//	}
//	if _, ok := prs.newStore[key]; !ok {
//		prs.newStore[key] = newPortRollupTracker()
//	}
//	return prs.curStore[key], prs.newStore[key]
//}

func (prs *EndpointPairPortRollupStore) getPortTracker(sourceAddr []byte, destAddr []byte, endpointT endpointType, port uint16) int {
	return len(prs.curStore[buildStoreKey(sourceAddr, destAddr, endpointT, port)])
}

// Add will record new sourcePort and destPort for a specific sourceAddr and destAddr
func (prs *EndpointPairPortRollupStore) Add(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) {
	prs.AddToStore(prs.curStore, sourceAddr, destAddr, sourcePort, destPort)
	prs.AddToStore(prs.newStore, sourceAddr, destAddr, sourcePort, destPort)
}

// AddToStore will add ports to store
func (prs *EndpointPairPortRollupStore) AddToStore(store map[string][]uint16, sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) {
	prs.mu.Lock()
	defer prs.mu.Unlock()

	srcToDestKey := buildStoreKey(sourceAddr, destAddr, isSourceEndpoint, sourcePort)
	destToSrcKey := buildStoreKey(sourceAddr, destAddr, isDestinationEndpoint, destPort)
	sourceToDestPorts := len(store[srcToDestKey])
	destToSourcePorts := len(store[destToSrcKey])
	if sourceToDestPorts >= prs.portRollupThreshold || destToSourcePorts >= prs.portRollupThreshold {
		return
	}
	store[srcToDestKey] = addPort(store[srcToDestKey], destPort)
	store[destToSrcKey] = addPort(store[destToSrcKey], sourcePort)
}

func addPort(ports []uint16, newPort uint16) []uint16 {
	for _, port := range ports {
		if port == newPort {
			return ports
		}
	}
	return append(ports, newPort)
}

// GetPortCount returns max port count and indicate whether the source or destination is ephemeral (isEphemeralSource)
func (prs *EndpointPairPortRollupStore) GetPortCount(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) (uint16, bool) {
	sourceToDestPortCount := prs.GetSourceToDestPortCount(sourceAddr, destAddr, sourcePort)
	destToSourcePortCount := prs.GetDestToSourcePortCount(sourceAddr, destAddr, destPort)
	portCount := common.MaxUint16(sourceToDestPortCount, destToSourcePortCount)
	isEphemeralSource := destToSourcePortCount > sourceToDestPortCount
	return portCount, isEphemeralSource
}

// IsEphemeral checks if source port and destination port are ephemeral
func (prs *EndpointPairPortRollupStore) IsEphemeral(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) IsEphemeralStatus {
	sourceToDestPortCount := prs.GetSourceToDestPortCount(sourceAddr, destAddr, sourcePort)
	destToSourcePortCount := prs.GetDestToSourcePortCount(sourceAddr, destAddr, destPort)
	portCount := common.MaxUint16(sourceToDestPortCount, destToSourcePortCount)

	if int(portCount) < prs.portRollupThreshold {
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
	prs.mu.Lock()
	defer prs.mu.Unlock()

	return uint16(prs.getPortTracker(sourceAddr, destAddr, isSourceEndpoint, sourcePort))
}

// GetDestToSourcePortCount returns the number of different source port for a specific destination port
func (prs *EndpointPairPortRollupStore) GetDestToSourcePortCount(sourceAddr []byte, destAddr []byte, destPort uint16) uint16 {
	prs.mu.Lock()
	defer prs.mu.Unlock()

	return uint16(prs.getPortTracker(sourceAddr, destAddr, isDestinationEndpoint, destPort))
}

// UseNewStoreAsCurrentStore sets newStore to curStore and clean up newStore
func (prs *EndpointPairPortRollupStore) UseNewStoreAsCurrentStore() {
	prs.mu.Lock()
	defer prs.mu.Unlock()

	prs.curStore = prs.newStore
	prs.newStore = make(map[string][]uint16)
}

// GetCurrentStoreSize get number of tracked port counters in current store
func (prs *EndpointPairPortRollupStore) GetCurrentStoreSize() int {
	prs.mu.Lock()
	defer prs.mu.Unlock()

	return len(prs.curStore)
}

// GetNewStoreSize get number of tracked port counters in new store
func (prs *EndpointPairPortRollupStore) GetNewStoreSize() int {
	prs.mu.Lock()
	defer prs.mu.Unlock()

	return len(prs.newStore)
}

func buildStoreKey(sourceAddr []byte, destAddr []byte, endpointT endpointType, port uint16) string {
	var portPart1, portPart2 = uint8(port >> 8), uint8(port & 0xff)
	return string(sourceAddr) + string(destAddr) + string([]byte{byte(endpointT)}) + string([]byte{portPart1}) + string([]byte{portPart2})

}
