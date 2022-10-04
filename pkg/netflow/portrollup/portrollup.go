// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package portrollup

import (
	"github.com/DataDog/datadog-agent/pkg/netflow/common"
)

// EphemeralPort port number is represented by `-1` internally
const EphemeralPort int32 = -1

// defaultPortRollupCacheEntryExpirationMin default expiration for rollup cache entry in minute
const defaultPortRollupCacheEntryExpirationMin uint8 = 5

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
// When IsEphemeral is called, only curStore is used.
type EndpointPairPortRollupStore struct {
	portRollupThreshold int
	portRollupCache     *PortCache
}

// NewEndpointPairPortRollupStore create a new *EndpointPairPortRollupStore
func NewEndpointPairPortRollupStore(portRollupThreshold int) *EndpointPairPortRollupStore {
	return &EndpointPairPortRollupStore{
		portRollupCache:     NewCache(defaultPortRollupCacheEntryExpirationMin),
		portRollupThreshold: portRollupThreshold,
	}
}

func (prs *EndpointPairPortRollupStore) getPortCount(sourceAddr []byte, destAddr []byte, endpointT endpointType, port uint16) uint8 {
	return prs.portRollupCache.Get(buildStoreKey(sourceAddr, destAddr, endpointT, port))
}

// Add will record new sourcePort and destPort for a specific sourceAddr and destAddr
func (prs *EndpointPairPortRollupStore) Add(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) {
	prs.AddToStore(sourceAddr, destAddr, sourcePort, destPort)
}

// AddToStore will add ports to store
func (prs *EndpointPairPortRollupStore) AddToStore(sourceAddr []byte, destAddr []byte, sourcePort uint16, destPort uint16) {
	srcToDestKey := buildStoreKey(sourceAddr, destAddr, isSourceEndpoint, sourcePort)
	destToSrcKey := buildStoreKey(sourceAddr, destAddr, isDestinationEndpoint, destPort)
	sourceToDestPorts := int(prs.portRollupCache.Get(srcToDestKey))
	destToSourcePorts := int(prs.portRollupCache.Get(destToSrcKey))
	if sourceToDestPorts >= prs.portRollupThreshold {
		// TODO: TESTME
		prs.portRollupCache.RefreshExpiration(srcToDestKey)
	} else if destToSourcePorts >= prs.portRollupThreshold {
		// TODO: TESTME
		prs.portRollupCache.RefreshExpiration(destToSrcKey)
	} else {
		prs.portRollupCache.Increment(srcToDestKey)
		prs.portRollupCache.Increment(destToSrcKey)
	}
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
	// TODO: use uint8
	return uint16(prs.getPortCount(sourceAddr, destAddr, isSourceEndpoint, sourcePort))
}

// GetDestToSourcePortCount returns the number of different source port for a specific destination port
func (prs *EndpointPairPortRollupStore) GetDestToSourcePortCount(sourceAddr []byte, destAddr []byte, destPort uint16) uint16 {
	return uint16(prs.getPortCount(sourceAddr, destAddr, isDestinationEndpoint, destPort))
}

// GetRollupTrackerCacheSize get rollup tracker cache size
func (prs *EndpointPairPortRollupStore) GetRollupTrackerCacheSize() int {
	return prs.portRollupCache.ItemCount()
}

// CleanExpired clean expired cache entries
func (prs *EndpointPairPortRollupStore) CleanExpired() {
	prs.portRollupCache.DeleteAllExpired()
}

func buildStoreKey(sourceAddr []byte, destAddr []byte, endpointT endpointType, port uint16) string {
	var portPart1, portPart2 = uint8(port >> 8), uint8(port & 0xff)
	return string(sourceAddr) + string(destAddr) + string([]byte{byte(endpointT)}) + string([]byte{portPart1}) + string([]byte{portPart2})
}
