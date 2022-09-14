// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package portrollup

import (
	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_endpointPairPortRollupStore_Add(t *testing.T) {
	// Arrange
	IP1 := []byte{10, 10, 10, 10}
	IP2 := []byte{10, 10, 10, 11}
	store := NewEndpointPairPortRollupStore(3)

	// 1/ Add
	store.Add(IP1, IP2, 80, 2001)
	store.Add(IP1, IP2, 80, 2002)
	store.Add(IP1, IP2, 80, 2003)
	store.Add(IP1, IP2, 80, 2004)
	store.Add(IP1, IP2, 3001, 443)
	store.Add(IP1, IP2, 3002, 443)
	store.Add(IP1, IP2, 3003, 443)
	store.Add(IP1, IP2, 3004, 443)
	currentStore, newStore := store.getOrCreate(IP1, IP2)

	assert.Equal(t, uint16(3), store.GetSourceToDestPortCount(IP1, IP2, 80))
	// should only contain first 3 destPort (2001, 2002, 2003), 2004 is not curStore since the threshold is already reached
	assert.Equal(t, map[uint16]bool{uint16(2001): true, uint16(2002): true, uint16(2003): true}, currentStore.sourcePorts[80])
	assert.Equal(t, map[uint16]bool{uint16(2001): true, uint16(2002): true, uint16(2003): true}, newStore.sourcePorts[80])

	for _, destPort := range []uint16{2001, 2002, 2003} {
		assert.Equal(t, uint16(1), store.GetDestToSourcePortCount(IP1, IP2, destPort))
		assert.Equal(t, map[uint16]bool{uint16(80): true}, currentStore.destPorts[destPort])
	}
	// make sure no entry is created for port 2004 in `destPorts`
	assert.Equal(t, uint16(0), store.GetDestToSourcePortCount(IP1, IP2, 2004))
	_, exist := currentStore.destPorts[2004]
	assert.Equal(t, false, exist)

	assert.Equal(t, uint16(3), store.GetDestToSourcePortCount(IP1, IP2, 443))
	// should only contain first 3 destPort (3001, 3002, 3003), 3004 is not curStore since the threshold is already reached
	assert.Equal(t, map[uint16]bool{uint16(3001): true, uint16(3002): true, uint16(3003): true}, currentStore.destPorts[443])
	assert.Equal(t, map[uint16]bool{uint16(3001): true, uint16(3002): true, uint16(3003): true}, newStore.destPorts[443])

}

func Test_endpointPairPortRollupStore_useNewStoreAsCurrentStore(t *testing.T) {
	// Arrange
	IP1 := []byte{10, 10, 10, 10}
	IP2 := []byte{10, 10, 10, 11}
	store := NewEndpointPairPortRollupStore(common.DefaultAggregatorPortRollupThreshold)

	// 1/ Add
	store.Add(IP1, IP2, 80, 2000)
	store.Add(IP1, IP2, 80, 2001)
	store.Add(IP1, IP2, 80, 2002)
	store.Add(IP1, IP2, 3000, 443)
	store.Add(IP1, IP2, 3001, 443)
	assert.Equal(t, uint16(3), store.GetSourceToDestPortCount(IP1, IP2, 80))
	assert.Equal(t, uint16(2), store.GetDestToSourcePortCount(IP1, IP2, 443))

	// 2/ After UseNewStoreAsCurrentStore, thanks to the double write, we should be still able to query ports tracked previously
	store.UseNewStoreAsCurrentStore()
	store.Add(IP1, IP2, 3000, 22)
	store.Add(IP1, IP2, 3001, 22)
	assert.Equal(t, uint16(3), store.GetSourceToDestPortCount(IP1, IP2, 80))
	assert.Equal(t, uint16(2), store.GetDestToSourcePortCount(IP1, IP2, 443))
	assert.Equal(t, uint16(2), store.GetDestToSourcePortCount(IP1, IP2, 22))

	// 3/ After a second UseNewStoreAsCurrentStore, the ports added in 1/ are not present anymore in the tracker, and only port added in 2/ are available
	store.UseNewStoreAsCurrentStore()
	assert.Equal(t, uint16(0), store.GetSourceToDestPortCount(IP1, IP2, 80))
	assert.Equal(t, uint16(0), store.GetDestToSourcePortCount(IP1, IP2, 443))
	assert.Equal(t, uint16(2), store.GetDestToSourcePortCount(IP1, IP2, 22))
}

func TestEndpointPairPortRollupStore_IsEphemeral_IsEphemeralSourcePort(t *testing.T) {
	// Arrange
	IP1 := []byte{10, 10, 10, 10}
	IP2 := []byte{10, 10, 10, 11}
	store := NewEndpointPairPortRollupStore(3)

	store.Add(IP1, IP2, 80, 2001)
	store.Add(IP1, IP2, 80, 2002)
	assert.Equal(t, NoEphemeralPort, store.IsEphemeral(IP1, IP2, 80, 2010))
	store.Add(IP1, IP2, 80, 2003)
	assert.Equal(t, IsEphemeralDestPort, store.IsEphemeral(IP1, IP2, 80, 2010))
}

func TestEndpointPairPortRollupStore_IsEphemeral_IsEphemeralDestPort(t *testing.T) {
	// Arrange
	IP1 := []byte{10, 10, 10, 10}
	IP2 := []byte{10, 10, 10, 11}
	store := NewEndpointPairPortRollupStore(3)

	store.Add(IP1, IP2, 3001, 53)
	store.Add(IP1, IP2, 3002, 53)
	assert.Equal(t, NoEphemeralPort, store.IsEphemeral(IP1, IP2, 3001, 53))
	store.Add(IP1, IP2, 3003, 53)
	assert.Equal(t, IsEphemeralSourcePort, store.IsEphemeral(IP1, IP2, 3004, 53))
}
