// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"net/netip"

	"github.com/DataDog/datadog-agent/pkg/network"
)

type darwinTupleKey struct {
	source netip.Addr
	dest   netip.Addr
	sport  uint16
	dport  uint16
	family network.ConnectionFamily
	typ    network.ConnectionType
}

type darwinTupleBinding struct {
	cookie uint64
	tuple  network.ConnectionTuple
}

type darwinTupleMatch struct {
	cookie           uint64
	matched          bool
	ambiguous        bool
	packetReversed   bool
	orientationKnown bool
}

// darwinTupleIndex correlates packet tuples with authoritative NStat tuples.
// It retains all cookies for a tuple so shared or loopback-dual tuples are
// rejected as ambiguous instead of choosing an arbitrary connection.
type darwinTupleIndex struct {
	byTuple  map[darwinTupleKey]map[uint64]struct{}
	bindings map[uint64]darwinTupleBinding
}

func newDarwinTupleIndex() *darwinTupleIndex {
	return &darwinTupleIndex{
		byTuple:  make(map[darwinTupleKey]map[uint64]struct{}),
		bindings: make(map[uint64]darwinTupleBinding),
	}
}

func (i *darwinTupleIndex) add(conn *network.ConnectionStats) {
	i.remove(conn.Cookie)
	key := makeDarwinTupleKey(conn.ConnectionTuple)
	cookies := i.byTuple[key]
	if cookies == nil {
		cookies = make(map[uint64]struct{})
		i.byTuple[key] = cookies
	}
	cookies[conn.Cookie] = struct{}{}
	i.bindings[conn.Cookie] = darwinTupleBinding{
		cookie: conn.Cookie,
		tuple:  conn.ConnectionTuple,
	}
}

func (i *darwinTupleIndex) remove(cookie uint64) {
	binding, ok := i.bindings[cookie]
	if !ok {
		return
	}
	key := makeDarwinTupleKey(binding.tuple)
	delete(i.bindings, cookie)
	if cookies := i.byTuple[key]; cookies != nil {
		delete(cookies, cookie)
		if len(cookies) == 0 {
			delete(i.byTuple, key)
		}
	}
}

func (i *darwinTupleIndex) match(packet network.ConnectionTuple) darwinTupleMatch {
	forward := i.byTuple[makeDarwinTupleKey(packet)]
	reverse := i.byTuple[makeDarwinTupleKey(reverseDarwinTuple(packet))]
	candidates := make(map[uint64]struct{}, len(forward)+len(reverse))
	for cookie := range forward {
		candidates[cookie] = struct{}{}
	}
	for cookie := range reverse {
		candidates[cookie] = struct{}{}
	}
	if len(candidates) == 0 {
		return darwinTupleMatch{}
	}
	if len(candidates) != 1 {
		return darwinTupleMatch{ambiguous: true}
	}
	var cookie uint64
	for candidate := range candidates {
		cookie = candidate
	}
	_, forwardMatch := forward[cookie]
	_, reverseMatch := reverse[cookie]
	return darwinTupleMatch{
		cookie:           cookie,
		matched:          true,
		packetReversed:   reverseMatch && !forwardMatch,
		orientationKnown: forwardMatch != reverseMatch,
	}
}

// matchExact returns the unique connection whose tuple has the requested
// orientation. Unlike match, it does not include reverse-oriented candidates.
// This is used to pair the two socket views of an intra-host connection.
func (i *darwinTupleIndex) matchExact(tuple network.ConnectionTuple) darwinTupleMatch {
	candidates := i.byTuple[makeDarwinTupleKey(tuple)]
	if len(candidates) == 0 {
		return darwinTupleMatch{}
	}
	if len(candidates) != 1 {
		return darwinTupleMatch{ambiguous: true}
	}
	for cookie := range candidates {
		return darwinTupleMatch{
			cookie:           cookie,
			matched:          true,
			orientationKnown: true,
		}
	}
	return darwinTupleMatch{}
}

func makeDarwinTupleKey(tuple network.ConnectionTuple) darwinTupleKey {
	return darwinTupleKey{
		source: normalizeDarwinAddress(tuple.Source.Addr),
		dest:   normalizeDarwinAddress(tuple.Dest.Addr),
		sport:  tuple.SPort,
		dport:  tuple.DPort,
		family: tuple.Family,
		typ:    tuple.Type,
	}
}

func reverseDarwinTuple(tuple network.ConnectionTuple) network.ConnectionTuple {
	tuple.Source, tuple.Dest = tuple.Dest, tuple.Source
	tuple.SPort, tuple.DPort = tuple.DPort, tuple.SPort
	return tuple
}

func normalizeDarwinAddress(addr netip.Addr) netip.Addr {
	if addr.IsValid() {
		return addr.Unmap()
	}
	return addr
}
