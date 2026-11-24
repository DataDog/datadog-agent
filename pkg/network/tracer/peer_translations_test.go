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

	"github.com/DataDog/datadog-agent/pkg/network"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func ctTuple(sport uint16) netebpf.ConntrackTuple {
	return netebpf.ConntrackTuple{Sport: sport, Dport: 80, Netns: 1}
}

func TestPeerTranslationsExpire(t *testing.T) {
	p := newPeerTranslations(10)
	t0 := time.Unix(1000, 0)
	p.add(ctTuple(1), ctTuple(101), peerKeepShort, t0)
	p.add(ctTuple(2), ctTuple(102), peerKeepLong, t0)

	v, ok := p.get(ctTuple(1), t0.Add(peerKeepShort-time.Millisecond))
	assert.True(t, ok)
	assert.Equal(t, ctTuple(101), v)
	_, ok = p.get(ctTuple(1), t0.Add(peerKeepShort))
	assert.False(t, ok, "short entry outlived its keep")
	_, ok = p.get(ctTuple(2), t0.Add(peerKeepShort))
	assert.True(t, ok)

	// a later add drops what expired
	p.add(ctTuple(3), ctTuple(103), peerKeepShort, t0.Add(peerKeepLong))
	assert.Equal(t, 1, p.len())
}

func TestPeerTranslationsEvictsShortFirst(t *testing.T) {
	p := newPeerTranslations(2)
	t0 := time.Unix(1000, 0)
	p.add(ctTuple(1), ctTuple(101), peerKeepLong, t0)
	p.add(ctTuple(2), ctTuple(102), peerKeepShort, t0.Add(time.Second))

	assert.Equal(t, 1, p.add(ctTuple(3), ctTuple(103), peerKeepShort, t0.Add(2*time.Second)))
	_, ok := p.get(ctTuple(2), t0.Add(2*time.Second))
	assert.False(t, ok, "the short entry should go before the older long one")
	_, ok = p.get(ctTuple(1), t0.Add(2*time.Second))
	assert.True(t, ok)
}

func TestPeerTranslationsRemoveAndReAdd(t *testing.T) {
	p := newPeerTranslations(10)
	t0 := time.Unix(1000, 0)
	p.add(ctTuple(1), ctTuple(101), peerKeepShort, t0)
	p.remove(ctTuple(1))
	_, ok := p.get(ctTuple(1), t0)
	assert.False(t, ok)

	// the first add's queued key must not expire the re-added entry
	p.add(ctTuple(1), ctTuple(201), peerKeepLong, t0.Add(time.Second))
	p.add(ctTuple(2), ctTuple(102), peerKeepShort, t0.Add(peerKeepShort+time.Second))
	v, ok := p.get(ctTuple(1), t0.Add(peerKeepShort+time.Second))
	assert.True(t, ok)
	assert.Equal(t, ctTuple(201), v)
}

func TestPeerTranslationsDisabled(t *testing.T) {
	var none *peerTranslations
	assert.Zero(t, none.add(ctTuple(1), ctTuple(101), peerKeepShort, time.Now()))
	_, ok := none.get(ctTuple(1), time.Now())
	assert.False(t, ok)
	none.remove(ctTuple(1))
	assert.Zero(t, none.len())
}

func TestLocalAddrSetKeepsTwoFetches(t *testing.T) {
	var l localAddrSet
	a, b := util.AddressFromString("10.0.0.1"), util.AddressFromString("10.0.0.2")
	assert.False(t, l.contains(a))

	l.update([]network.ConnectionStats{{ConnectionTuple: network.ConnectionTuple{Source: a}}})
	assert.True(t, l.contains(a))
	l.update([]network.ConnectionStats{{ConnectionTuple: network.ConnectionTuple{Source: b}}})
	assert.True(t, l.contains(a), "the previous fetch's addresses still count")
	assert.True(t, l.contains(b))
	l.update(nil)
	assert.False(t, l.contains(a))
	assert.True(t, l.contains(b))
}
