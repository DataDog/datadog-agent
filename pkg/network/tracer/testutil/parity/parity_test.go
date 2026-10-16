// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package parity

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func tcpConn(sport uint16, sent, recv uint64, closed bool) network.ConnectionStats {
	return network.ConnectionStats{
		ConnectionTuple: network.ConnectionTuple{
			Source:    util.AddressFromString("10.0.0.1"),
			Dest:      util.AddressFromString("10.0.0.2"),
			SPort:     sport,
			DPort:     80,
			Pid:       1234,
			NetNS:     4026531840,
			Type:      network.TCP,
			Family:    network.AFINET,
			Direction: network.OUTGOING,
		},
		Monotonic: network.StatCounters{SentBytes: sent, RecvBytes: recv},
		IsClosed:  closed,
	}
}

var opts = Options{LabelA: "kprobe", LabelB: "fentry", CounterTolerance: 0.05, RTTTolerance: 0.25}

func TestIdentical(t *testing.T) {
	a := []network.ConnectionStats{tcpConn(1000, 100, 200, true), tcpConn(1001, 5, 6, false)}
	b := []network.ConnectionStats{tcpConn(1001, 5, 6, false), tcpConn(1000, 100, 200, true)}

	r := Compare(a, b, opts)
	assert.True(t, r.OK(), r.String())
	assert.Equal(t, 2, r.Matched)
}

func TestOnlyOneSide(t *testing.T) {
	a := []network.ConnectionStats{tcpConn(1000, 1, 1, true), tcpConn(1001, 1, 1, true)}
	b := []network.ConnectionStats{tcpConn(1000, 1, 1, true), tcpConn(1002, 1, 1, true)}

	r := Compare(a, b, opts)
	require.Len(t, r.Divergences, 2, r.String())
	assert.Equal(t, 1, r.Matched)

	kinds := map[Kind]uint16{}
	for _, d := range r.Divergences {
		kinds[d.Kind] = d.Tuple.SPort
	}
	assert.Equal(t, map[Kind]uint16{OnlyA: 1001, OnlyB: 1002}, kinds)
	assert.Contains(t, r.String(), "only-kprobe")
	assert.Contains(t, r.String(), "only-fentry")
}

func TestClosedCountersAreExact(t *testing.T) {
	// 1% apart: within tolerance, but closed connections must match exactly
	a := []network.ConnectionStats{tcpConn(1000, 1000, 0, true)}
	b := []network.ConnectionStats{tcpConn(1000, 990, 0, true)}

	r := Compare(a, b, opts)
	require.Len(t, r.Divergences, 1, r.String())
	d := r.Divergences[0]
	assert.Equal(t, FieldMismatch, d.Kind)
	assert.Equal(t, "SentBytes", d.Field)
	assert.True(t, d.Closed)
	assert.Equal(t, uint64(1000), d.A)
	assert.Equal(t, uint64(990), d.B)
}

func TestActiveCountersUseTolerance(t *testing.T) {
	a := []network.ConnectionStats{tcpConn(1000, 1000, 1000, false)}

	within := []network.ConnectionStats{tcpConn(1000, 960, 1000, false)}
	assert.True(t, Compare(a, within, opts).OK())

	outside := []network.ConnectionStats{tcpConn(1000, 900, 1000, false)}
	r := Compare(a, outside, opts)
	require.Len(t, r.Divergences, 1, r.String())
	assert.Equal(t, "SentBytes", r.Divergences[0].Field)
	assert.False(t, r.Divergences[0].Closed)
}

func TestClosedOnOneSideOnly(t *testing.T) {
	// one side hasn't seen the close yet: counters get tolerance, but IsClosed differs
	a := []network.ConnectionStats{tcpConn(1000, 1000, 0, true)}
	b := []network.ConnectionStats{tcpConn(1000, 980, 0, false)}

	r := Compare(a, b, opts)
	require.Len(t, r.Divergences, 1, r.String())
	assert.Equal(t, "IsClosed", r.Divergences[0].Field)
}

func TestPacketSlack(t *testing.T) {
	a, b := tcpConn(1000, 1, 1, true), tcpConn(1000, 1, 1, true)
	a.Monotonic.SentPackets, b.Monotonic.SentPackets = 84, 85

	// closed counters are exact by default
	r := Compare([]network.ConnectionStats{a}, []network.ConnectionStats{b}, opts)
	require.Len(t, r.Divergences, 1, r.String())
	assert.Equal(t, "SentPackets", r.Divergences[0].Field)

	o := opts
	o.PacketSlack = 1
	assert.True(t, Compare([]network.ConnectionStats{a}, []network.ConnectionStats{b}, o).OK())

	// slack does not apply to other counters, nor beyond its size
	b.Monotonic.SentPackets, b.Monotonic.SentBytes = 86, 2
	r = Compare([]network.ConnectionStats{a}, []network.ConnectionStats{b}, o)
	var names []string
	for _, d := range r.Divergences {
		names = append(names, d.Field)
	}
	assert.ElementsMatch(t, []string{"SentPackets", "SentBytes"}, names, r.String())
}

func TestRTTTolerance(t *testing.T) {
	a, b := tcpConn(1000, 1, 1, true), tcpConn(1000, 1, 1, true)
	a.RTT, b.RTT = 1000, 800
	assert.True(t, Compare([]network.ConnectionStats{a}, []network.ConnectionStats{b}, opts).OK())

	b.RTT = 500
	r := Compare([]network.ConnectionStats{a}, []network.ConnectionStats{b}, opts)
	require.Len(t, r.Divergences, 1, r.String())
	assert.Equal(t, "RTT", r.Divergences[0].Field)
}

func TestExactFields(t *testing.T) {
	a, b := tcpConn(1000, 1, 1, true), tcpConn(1000, 1, 1, true)
	a.ProtocolStack = protocols.Stack{Application: protocols.HTTP}
	a.TCPFailures = map[uint16]uint32{111: 2}
	b.TCPFailures = map[uint16]uint32{111: 1}

	r := Compare([]network.ConnectionStats{a}, []network.ConnectionStats{b}, opts)
	var names []string
	for _, d := range r.Divergences {
		names = append(names, d.Field)
	}
	assert.ElementsMatch(t, []string{"ProtocolStack", "TCPFailures"}, names, r.String())
}

func TestTupleReuse(t *testing.T) {
	// the same tuple used twice; pairs are matched in LastUpdateEpoch order
	first, second := tcpConn(1000, 10, 0, true), tcpConn(1000, 20, 0, true)
	first.LastUpdateEpoch, second.LastUpdateEpoch = 1, 2
	firstB, secondB := first, second
	firstB.LastUpdateEpoch, secondB.LastUpdateEpoch = 11, 12

	a := []network.ConnectionStats{second, first}
	b := []network.ConnectionStats{firstB, secondB}
	r := Compare(a, b, opts)
	assert.True(t, r.OK(), r.String())
	assert.Equal(t, 2, r.Matched)

	// side B missed the second use of the tuple
	r = Compare(a, []network.ConnectionStats{firstB}, opts)
	require.Len(t, r.Divergences, 1, r.String())
	assert.Equal(t, OnlyA, r.Divergences[0].Kind)
	assert.Equal(t, 1, r.Divergences[0].Occurrence)
}

func TestIgnoreAndFilter(t *testing.T) {
	a := []network.ConnectionStats{tcpConn(1000, 100, 0, true), tcpConn(1001, 1, 1, true)}
	b := []network.ConnectionStats{tcpConn(1000, 99, 0, true)}

	o := opts
	o.Ignore = []string{"SentBytes"}
	o.Filter = func(c *network.ConnectionStats) bool { return c.SPort != 1001 }
	r := Compare(a, b, o)
	assert.True(t, r.OK(), r.String())
	assert.Equal(t, 1, r.Matched)
}

func TestOracleAttribution(t *testing.T) {
	a := []network.ConnectionStats{tcpConn(1000, 100, 50, true)}
	b := []network.ConnectionStats{tcpConn(1000, 90, 40, true)}

	o := opts
	o.Oracle = func(c *network.ConnectionStats) (Expected, bool) {
		// sent bytes: kprobe right; recv bytes: neither right
		return Expected{"SentBytes": 100, "RecvBytes": 60}, true
	}
	r := Compare(a, b, o)
	got := map[string]Attribution{}
	for _, d := range r.Divergences {
		got[d.Field] = d.Attribution
	}
	assert.Equal(t, map[string]Attribution{"SentBytes": BWrong, "RecvBytes": BothWrong}, got, r.String())
	assert.Contains(t, r.String(), "[fentry-wrong]")
}

func TestFieldsCoverStatCounters(t *testing.T) {
	// guard against StatCounters growing a field that the comparator silently skips
	names := map[string]bool{}
	for _, n := range Fields() {
		names[n] = true
	}
	st := reflect.TypeOf(network.StatCounters{})
	for i := range st.NumField() {
		n := st.Field(i).Name
		assert.True(t, names[n], "StatCounters.%s is not compared", n)
	}
}
