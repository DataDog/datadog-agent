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
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func natTuple(sport uint16) network.ConnectionTuple {
	return network.ConnectionTuple{
		Source: util.AddressFromString("10.0.0.1"),
		Dest:   util.AddressFromString("10.96.0.10"),
		SPort:  sport,
		DPort:  80,
		Type:   network.TCP,
	}
}

func TestDeferredTranslationDeletesMinAge(t *testing.T) {
	d := newDeferredTranslationDeletes(10, 10*time.Second)
	t0 := time.Unix(1000, 0)
	for i, at := range []time.Time{t0, t0.Add(3 * time.Second), t0.Add(8 * time.Second)} {
		_, queued := d.add(natTuple(uint16(40000+i)), at)
		require.True(t, queued)
	}

	assert.Empty(t, d.expired(t0.Add(9*time.Second)))
	assert.Equal(t, []network.ConnectionTuple{natTuple(40000)}, d.expired(t0.Add(10*time.Second)))
	assert.Equal(t, []network.ConnectionTuple{natTuple(40001), natTuple(40002)}, d.expired(t0.Add(20*time.Second)))
	assert.Empty(t, d.expired(t0.Add(60*time.Second)))
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
	assert.Equal(t, []network.ConnectionTuple{natTuple(2), natTuple(3)}, d.expired(t0.Add(time.Minute)))
}

func TestDeferredTranslationDeletesDisabled(t *testing.T) {
	var none *deferredTranslationDeletes
	_, queued := none.add(natTuple(1), time.Now())
	assert.False(t, queued, "a nil queue must make the caller delete now")

	d := newDeferredTranslationDeletes(10, time.Second)
	d.setMax(0)
	_, queued = d.add(natTuple(1), time.Now())
	assert.False(t, queued, "max 0 must make the caller delete now")
}

func TestDeferredTranslationDeletesActive(t *testing.T) {
	d := newDeferredTranslationDeletes(10, time.Second)
	c := natTuple(1)
	assert.False(t, d.isActive(newTranslationKey(&c)))
	d.setActive(map[translationKey]struct{}{newTranslationKey(&c): {}})
	assert.True(t, d.isActive(newTranslationKey(&c)))
}

func TestNATDeferralCap(t *testing.T) {
	assert.Equal(t, 8192, natDeferralCap(&config.Config{ConntrackMaxStateSize: 131072, MaxTrackedConnections: 65536}))
	assert.Equal(t, 1024, natDeferralCap(&config.Config{ConntrackMaxStateSize: 4096, MaxTrackedConnections: 4096}))
}
