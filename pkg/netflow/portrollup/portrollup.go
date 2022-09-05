// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package portrollup

import (
	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	"sync"
)

// EndpointPairPortRollupStore contains port rollup states
type EndpointPairPortRollupStore struct {
	portRollupThreshold int
	curStore            map[string]*portRollupTracker
	newStore            map[string]*portRollupTracker

	// mutex used to protect access to curStore and newStore
	mu sync.Mutex
}

// NewEndpointPairPortRollupStore create a new *EndpointPairPortRollupStore
func NewEndpointPairPortRollupStore(portRollupThreshold int) *EndpointPairPortRollupStore {
	return &EndpointPairPortRollupStore{
		// curStore and newStore map key is composed of `<SOURCE_IP>|<DESTINATION_IP>`
		// SOURCE_IP and SOURCE_IP are converted from []byte to string.
		// string is used as map key since we can't use []byte as map key.
		curStore: make(map[string]*portRollupTracker),
		newStore: make(map[string]*portRollupTracker),

		portRollupThreshold: portRollupThreshold,
	}
}

func (prs *EndpointPairPortRollupStore) getOrCreate(sourceAddr []byte, destAddr []byte) (*portRollupTracker, *portRollupTracker) {
	key := buildStoreKey(sourceAddr, destAddr)
	if _, ok := prs.curStore[key]; !ok {
		prs.curStore[key] = newPortRollupTracker()
	}
	if _, ok := prs.newStore[key]; !ok {
		prs.newStore[key] = newPortRollupTracker()
	}
	return prs.curStore[key], prs.newStore[key]
}

func (prs *EndpointPairPortRollupStore) getPortTracker(sourceAddr []byte, destAddr []byte) *portRollupTracker {
	return prs.curStore[buildStoreKey(sourceAddr, destAddr)]
}

// Add will record new sourcePort and destPort for a specific sourceAddr and destAddr
func (prs *EndpointPairPortRollupStore) Add(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) {
	prs.mu.Lock()
	defer prs.mu.Unlock()

	// Double write to current port tracker and new port tracker
	curTracker, newTracker := prs.getOrCreate(sourceAddr, destAddr)
	curTracker.add(sourcePort, destPort, prs.portRollupThreshold)
	newTracker.add(sourcePort, destPort, prs.portRollupThreshold)
}

// GetPortCount returns max port count and indicate whether the source or destination is ephemeral (isEphemeralSource)
func (prs *EndpointPairPortRollupStore) GetPortCount(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) (uint16, bool) {
	sourceToDestPortCount := prs.GetSourceToDestPortCount(sourceAddr, destAddr, sourcePort)
	destToSourcePortCount := prs.GetDestToSourcePortCount(sourceAddr, destAddr, destPort)
	portCount := common.MaxUint16(sourceToDestPortCount, destToSourcePortCount)
	isEphemeralSource := destToSourcePortCount > sourceToDestPortCount
	return portCount, isEphemeralSource
}

// GetSourceToDestPortCount returns the number of different destination port for a specific source port
func (prs *EndpointPairPortRollupStore) GetSourceToDestPortCount(sourceAddr []byte, destAddr []byte, sourcePort uint16) uint16 {
	prs.mu.Lock()
	defer prs.mu.Unlock()

	tracker := prs.getPortTracker(sourceAddr, destAddr)
	if tracker != nil {
		return tracker.getSourcePortCount(sourcePort)
	}
	return 0
}

// GetDestToSourcePortCount returns the number of different source port for a specific destination port
func (prs *EndpointPairPortRollupStore) GetDestToSourcePortCount(sourceAddr []byte, destAddr []byte, destPort uint16) uint16 {
	prs.mu.Lock()
	defer prs.mu.Unlock()

	tracker := prs.getPortTracker(sourceAddr, destAddr)
	if tracker != nil {
		return tracker.getDestPortCount(destPort)
	}
	return 0
}

// UseNewStoreAsCurrentStore sets newStore to curStore and clean up newStore
func (prs *EndpointPairPortRollupStore) UseNewStoreAsCurrentStore() {
	prs.mu.Lock()
	defer prs.mu.Unlock()

	prs.curStore = prs.newStore
	prs.newStore = make(map[string]*portRollupTracker)
}

func buildStoreKey(sourceAddr []byte, destAddr []byte) string {
	return string(sourceAddr) + "|" + string(destAddr)
}
