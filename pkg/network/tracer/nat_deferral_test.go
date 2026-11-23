// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package tracer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

func natTuple(sport uint16) network.ConnectionTuple {
	return network.ConnectionTuple{SPort: sport, DPort: 80, Type: network.TCP}
}

func TestDeferredTranslationDeletesMinAge(t *testing.T) {
	d := newDeferredTranslationDeletes(10, 10*time.Second)
	t0 := time.Unix(1000, 0)
	for i, at := range []time.Time{t0, t0.Add(3 * time.Second), t0.Add(8 * time.Second)} {
		_, queued := d.add(natTuple(uint16(i+1)), at)
		require.True(t, queued)
	}

	assert.Empty(t, d.expire(t0.Add(9*time.Second), nil))
	assert.Equal(t, []network.ConnectionTuple{natTuple(1)}, d.expire(t0.Add(10*time.Second), nil))
	// any number of calls in between can't expire an entry before it's old enough
	for range 5 {
		assert.Empty(t, d.expire(t0.Add(12*time.Second), nil))
	}
	assert.Equal(t, []network.ConnectionTuple{natTuple(2), natTuple(3)}, d.expire(t0.Add(time.Minute), nil))
}

func TestDeferredTranslationDeletesEvictsOldestOverCap(t *testing.T) {
	d := newDeferredTranslationDeletes(2, 10*time.Second)
	t0 := time.Unix(1000, 0)
	_, _ = d.add(natTuple(1), t0)
	_, _ = d.add(natTuple(2), t0.Add(time.Second))

	evicted, queued := d.add(natTuple(3), t0.Add(2*time.Second))
	require.True(t, queued)
	require.NotNil(t, evicted)
	assert.Equal(t, natTuple(1), *evicted)
	assert.Equal(t, []network.ConnectionTuple{natTuple(2), natTuple(3)}, d.expire(t0.Add(time.Minute), nil))
}

func TestDeferredTranslationDeletesWasActive(t *testing.T) {
	d := newDeferredTranslationDeletes(10, time.Second)
	c := natTuple(1)
	assert.False(t, d.wasActive(newTranslationKey(&c)))
	d.expire(time.Now(), map[translationKey]struct{}{newTranslationKey(&c): {}})
	assert.True(t, d.wasActive(newTranslationKey(&c)))
}

func TestDeferredTranslationDeletesDisabled(t *testing.T) {
	var none *deferredTranslationDeletes
	_, queued := none.add(natTuple(1), time.Now())
	assert.False(t, queued, "a nil queue must make the caller delete now")
	assert.Nil(t, none.expire(time.Now(), nil))

	d := newDeferredTranslationDeletes(10, time.Second)
	d.setMax(0)
	_, queued = d.add(natTuple(1), time.Now())
	assert.False(t, queued, "max 0 must make the caller delete now")
}

func TestNATDeferralCap(t *testing.T) {
	assert.Equal(t, 8192, natDeferralCap(&config.Config{ConntrackMaxStateSize: 131072, MaxTrackedConnections: 65536}))
	assert.Equal(t, 1024, natDeferralCap(&config.Config{ConntrackMaxStateSize: 4096, MaxTrackedConnections: 4096}))
}
