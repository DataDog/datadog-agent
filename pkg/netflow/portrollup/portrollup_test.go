// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package portrollup

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func Test_endpointPairPortRollupStore_Add(t *testing.T) {
	setMockTime()
	defer revertTime()

	// Arrange
	IP1 := []byte{10, 10, 10, 10}
	IP2 := []byte{10, 10, 10, 11}
	store := NewEndpointPairPortRollupStore(3)
	//store.portRollupCache.LastClean = MockTimeNow()

	// 1/ Add
	store.Add(IP1, IP2, 80, 2001)
	store.Add(IP1, IP2, 80, 2002)
	store.Add(IP1, IP2, 80, 2003)
	t0 := timeNow()
	assert.Equal(t, uint16(3), store.GetSourceToDestPortCount(IP1, IP2, 80))
	assert.Equal(t, t0.Add(300*time.Second).UnixNano(), store.portRollupCache.GetExpiration(buildStoreKey(IP1, IP2, isSourceEndpoint, 80)))

	timeNow = func() time.Time {
		return MockTimeNow().Add(1 * time.Minute)
	}
	t1 := timeNow()
	store.Add(IP1, IP2, 80, 2004)
	assert.Equal(t, uint16(3), store.GetSourceToDestPortCount(IP1, IP2, 80)) // no change for port count
	assert.Equal(t, t1.Add(300*time.Second).UnixNano(), store.portRollupCache.GetExpiration(buildStoreKey(IP1, IP2, isSourceEndpoint, 80)))

	store.Add(IP1, IP2, 3001, 443)
	store.Add(IP1, IP2, 3002, 443)
	store.Add(IP1, IP2, 3003, 443)
	assert.Equal(t, uint16(3), store.GetDestToSourcePortCount(IP1, IP2, 443))
	assert.Equal(t, t1.Add(300*time.Second).UnixNano(), store.portRollupCache.GetExpiration(buildStoreKey(IP1, IP2, isDestinationEndpoint, 443)))

	timeNow = func() time.Time {
		return MockTimeNow().Add(2 * time.Minute)
	}
	t2 := timeNow()
	store.Add(IP1, IP2, 3004, 443)
	assert.Equal(t, uint16(3), store.GetDestToSourcePortCount(IP1, IP2, 443))
	assert.Equal(t, t2.Add(300*time.Second).UnixNano(), store.portRollupCache.GetExpiration(buildStoreKey(IP1, IP2, isDestinationEndpoint, 443)))

	assert.Equal(t, uint16(3), store.GetSourceToDestPortCount(IP1, IP2, 80))
	// should only contain first 3 destPort (2001, 2002, 2003), 2004 is not curStore since the threshold is already reached
	for _, destPort := range []uint16{2001, 2002, 2003} {
		assert.Equal(t, uint16(1), store.GetDestToSourcePortCount(IP1, IP2, destPort))
	}
	// make sure no entry is created for port 2004 in `destPorts`
	assert.Equal(t, uint16(0), store.GetDestToSourcePortCount(IP1, IP2, 2004))

	assert.Equal(t, uint16(3), store.GetDestToSourcePortCount(IP1, IP2, 443))
	// should only contain first 3 destPort (3001, 3002, 3003), 3004 is not curStore since the threshold is already reached
	for _, sourcePort := range []uint16{3001, 3002, 3003} {
		assert.Equal(t, uint16(1), store.GetSourceToDestPortCount(IP1, IP2, sourcePort))
	}
	// make sure no entry is created for port 3004 in `destPorts`
	assert.Equal(t, uint16(0), store.GetSourceToDestPortCount(IP1, IP2, 3004))
}

func Test_endpointPairPortRollupStore_deleteAllExpired(t *testing.T) {
	setMockTime()
	defer revertTime()

	// Arrange
	IP1 := []byte{10, 10, 10, 10}
	IP2 := []byte{10, 10, 10, 11}
	store := NewEndpointPairPortRollupStore(common.DefaultAggregatorPortRollupThreshold)
	//store.portRollupCache.LastClean = MockTimeNow()

	t0 := timeNow()
	store.Add(IP1, IP2, 80, 2000)
	assert.Equal(t, uint16(1), store.GetSourceToDestPortCount(IP1, IP2, 80))
	assert.Equal(t, t0.Add(300*time.Second).UnixNano(), store.portRollupCache.GetExpiration(buildStoreKey(IP1, IP2, isSourceEndpoint, 80)))

	timeNow = func() time.Time {
		return MockTimeNow().Add(1 * time.Minute)
	}
	t1 := timeNow()
	store.Add(IP1, IP2, 80, 2001)
	assert.Equal(t, uint16(2), store.GetSourceToDestPortCount(IP1, IP2, 80))
	assert.Equal(t, t1.Add(300*time.Second).UnixNano(), store.portRollupCache.GetExpiration(buildStoreKey(IP1, IP2, isSourceEndpoint, 80)))

	timeNow = func() time.Time {
		return MockTimeNow().Add(10 * time.Minute)
	}
	store.DeleteAllExpired()

	assert.Equal(t, int(0), store.GetRollupTrackerCacheSize())
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
	assert.Equal(t, "\n\n\n\n\n\n\n\v\x00\x00P", buildStoreKey([]byte{10, 10, 10, 10}, []byte{10, 10, 10, 11}, isSourceEndpoint, 80))
	assert.Equal(t, "\x01\x02\x03\x04\x05\x06\a\b\x01\a\xd0", buildStoreKey([]byte{1, 2, 3, 4}, []byte{5, 6, 7, 8}, isDestinationEndpoint, 2000))

	key := buildStoreKey([]byte{255, 10, 10, 10}, []byte{10, 10, 10, 11}, isDestinationEndpoint, 65535)
	assert.Equal(t, "[11111111 00001010 00001010 00001010 00001010 00001010 00001010 00001011 00000001 11111111 11111111]", fmt.Sprintf("%08b", []byte(key)))
}
