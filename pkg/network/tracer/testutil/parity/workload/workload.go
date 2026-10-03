// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

// Package workload generates deterministic network traffic for tracer parity
// tests, and records what it knows it sent so that divergences between tracers
// can be attributed (see parity.Oracle).
//
// Each case uses its own ephemeral ports, so every connection it creates is
// attributable to it. Cases record expectations only for values they know
// exactly (bytes, and UDP datagram counts); everything else is left to the
// tracer-vs-tracer comparison.
package workload

import (
	"net"
	"net/netip"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/parity"
)

// Case is a single traffic pattern
type Case struct {
	Name string
	// Run generates the traffic, recording expectations and ports in rec. It
	// must not return until all data has been sent and received, so that the
	// recorded expectations hold when the tracers are read afterwards.
	Run func(tb testing.TB, rec *Recorder)
}

// Expectation is what a case knows about one side of a connection, as the
// tracer would report it: Local is the connection's source, Remote its
// destination.
type Expectation struct {
	Type          network.ConnectionType
	Local, Remote netip.AddrPort
	Want          parity.Expected
}

// Recorder collects the expectations and ports used by cases
type Recorder struct {
	mu           sync.Mutex
	expectations []Expectation
	ports        map[uint16]struct{}
}

// NewRecorder returns an empty Recorder
func NewRecorder() *Recorder {
	return &Recorder{ports: make(map[uint16]struct{})}
}

// Expect records an expectation and marks its ports as belonging to the workload
func (r *Recorder) Expect(e Expectation) {
	e.Local, e.Remote = unmap(e.Local), unmap(e.Remote)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expectations = append(r.expectations, e)
	r.ports[e.Local.Port()] = struct{}{}
	r.ports[e.Remote.Port()] = struct{}{}
}

// ExpectPair records both sides of a connection between client and server:
// the client side sends sent and receives recv, and the server side the reverse
func (r *Recorder) ExpectPair(typ network.ConnectionType, client, server netip.AddrPort, sent, recv parity.Expected) {
	r.Expect(Expectation{Type: typ, Local: client, Remote: server, Want: sent})
	r.Expect(Expectation{Type: typ, Local: server, Remote: client, Want: recv})
}

// UsePorts marks ports as belonging to the workload without recording an
// expectation, e.g. for failed connections whose counters aren't known
func (r *Recorder) UsePorts(ports ...uint16) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range ports {
		r.ports[p] = struct{}{}
	}
}

// Expectations returns the recorded expectations
func (r *Recorder) Expectations() []Expectation {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Expectation(nil), r.expectations...)
}

// Filter returns a parity.Options.Filter that keeps only connections on ports
// used by the workload
func (r *Recorder) Filter() func(c *network.ConnectionStats) bool {
	return func(c *network.ConnectionStats) bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		_, s := r.ports[c.SPort]
		_, d := r.ports[c.DPort]
		return s || d
	}
}

// Oracle returns a parity.Oracle backed by the recorded expectations
func (r *Recorder) Oracle() parity.Oracle {
	type key struct {
		typ           network.ConnectionType
		local, remote netip.AddrPort
	}
	byKey := make(map[key]parity.Expected)
	for _, e := range r.Expectations() {
		byKey[key{e.Type, e.Local, e.Remote}] = e.Want
	}
	return func(c *network.ConnectionStats) (parity.Expected, bool) {
		k := key{
			typ:    c.Type,
			local:  netip.AddrPortFrom(c.Source.Addr.Unmap(), c.SPort),
			remote: netip.AddrPortFrom(c.Dest.Addr.Unmap(), c.DPort),
		}
		want, ok := byKey[k]
		return want, ok
	}
}

// Run runs each case as a subtest and returns the populated Recorder
func Run(t *testing.T, cases []Case) *Recorder {
	rec := NewRecorder()
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			c.Run(t, rec)
		})
	}
	return rec
}

func unmap(ap netip.AddrPort) netip.AddrPort {
	return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())
}

func netipAddrPort(ip string, port uint16) netip.AddrPort {
	return netip.AddrPortFrom(netip.MustParseAddr(ip), port)
}

func addrPort(a net.Addr) netip.AddrPort {
	switch v := a.(type) {
	case *net.TCPAddr:
		return unmap(v.AddrPort())
	case *net.UDPAddr:
		return unmap(v.AddrPort())
	default:
		return netip.MustParseAddrPort(a.String())
	}
}
