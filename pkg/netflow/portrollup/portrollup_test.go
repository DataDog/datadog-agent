// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package portrollup

import (
	"encoding/binary"
	"fmt"
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

	assert.Equal(t, uint16(3), store.GetSourceToDestPortCount(IP1, IP2, 80))
	// should only contain first 3 destPort (2001, 2002, 2003), 2004 is not curStore since the threshold is already reached
	assert.Equal(t, []uint16{2001, 2002, 2003}, store.curStore[buildStoreKey(IP1, IP2, isSourceEndpoint, 80)])
	assert.Equal(t, []uint16{2001, 2002, 2003}, store.newStore[buildStoreKey(IP1, IP2, isSourceEndpoint, 80)])

	for _, destPort := range []uint16{2001, 2002, 2003} {
		assert.Equal(t, uint16(1), store.GetDestToSourcePortCount(IP1, IP2, destPort))
		assert.Equal(t, []uint16{80}, store.curStore[buildStoreKey(IP1, IP2, isDestinationEndpoint, destPort)])
	}
	// make sure no entry is created for port 2004 in `destPorts`
	assert.Equal(t, uint16(0), store.GetDestToSourcePortCount(IP1, IP2, 2004))
	_, exist := store.curStore[buildStoreKey(IP1, IP2, isDestinationEndpoint, 2004)]
	assert.Equal(t, false, exist)

	assert.Equal(t, uint16(3), store.GetDestToSourcePortCount(IP1, IP2, 443))
	// should only contain first 3 destPort (3001, 3002, 3003), 3004 is not curStore since the threshold is already reached
	assert.Equal(t, []uint16{3001, 3002, 3003}, store.curStore[buildStoreKey(IP1, IP2, isDestinationEndpoint, 443)])
	assert.Equal(t, []uint16{3001, 3002, 3003}, store.newStore[buildStoreKey(IP1, IP2, isDestinationEndpoint, 443)])
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

func Test_buildStoreKey(t *testing.T) {
	key1 := buildStoreKey([]byte{10, 10, 10, 10}, []byte{10, 10, 10, 11}, isSourceEndpoint, 80)
	key2 := buildStoreKey([]byte{10, 10, 10, 10}, []byte{10, 10, 10, 11}, isSourceEndpoint, 50000)
	key3 := buildStoreKey([]byte{10, 10, 10, 10}, []byte{10, 10, 10, 11}, isDestinationEndpoint, 80)
	key4 := buildStoreKey([]byte{1, 2, 3, 4}, []byte{5, 6, 7, 8}, isDestinationEndpoint, 2000)
	assert.Equal(t, "4115abb8381ea64d1bebc1a33164eed4", fmt.Sprintf("%x", key1))
	assert.Equal(t, "4115abb8381ea64d1bebc1a33164ee17", fmt.Sprintf("%x", key2))
	assert.Equal(t, "4115abbb2e1ea64d1bebc1a331670fed", fmt.Sprintf("%x", key3))
	assert.Equal(t, "f453633d2d660deed430b56558628ed9", fmt.Sprintf("%x", key4))
	assert.NotEqual(t, key1, key2)
	assert.NotEqual(t, key1, key3)
}

func Test_buildStoreKey_naiveCollisionTest(t *testing.T) {
	keys := make(map[[16]byte]struct{})
	for i := 0; i < 100; i++ {
		for j := 0; j < 100; j++ {
			for k := 0; k < 100; k++ {
				srcAddr := make([]byte, 4)
				binary.LittleEndian.PutUint32(srcAddr, uint32(j))
				dstAddr := make([]byte, 4)
				binary.LittleEndian.PutUint32(dstAddr, uint32(k))
				for _, status := range []endpointType{isSourceEndpoint, isDestinationEndpoint} {
					keys[buildStoreKey(srcAddr, dstAddr, status, uint16(i))] = struct{}{}
				}
			}
		}
	}
	assert.Equal(t, 100*100*100*2, len(keys))
}
