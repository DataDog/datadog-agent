// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"fmt"
	"net"
	"runtime"
	"testing"

	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/process/util"

	"github.com/stretchr/testify/assert"
)

var (
	testConn = ConnectionStats{
		Pid:    123,
		Type:   1,
		Family: AFINET,
		Source: util.AddressFromString("192.168.0.1"),
		Dest:   util.AddressFromString("192.168.0.103"),
		SPort:  123,
		DPort:  35000,
		Monotonic: StatCounters{
			SentBytes: 123123,
			RecvBytes: 312312,
		},
	}
)

func TestBeautifyKey(t *testing.T) {
	buf := make([]byte, ConnectionByteKeyMaxLen)
	for _, c := range []ConnectionStats{
		testConn,
		{
			Pid:    345,
			Type:   0,
			Family: AFINET6,
			Source: util.AddressFromNetIP(net.ParseIP("::7f00:35:0:1")),
			Dest:   util.AddressFromNetIP(net.ParseIP("2001:db8::2:1")),
			SPort:  4444,
			DPort:  8888,
			Cookie: 1,
		},
		{
			Pid:       32065,
			Type:      0,
			Family:    AFINET,
			Direction: 2,
			Source:    util.AddressFromString("172.21.148.124"),
			Dest:      util.AddressFromString("130.211.21.187"),
			SPort:     52012,
			DPort:     443,
			Cookie:    2,
		},
	} {
		bk := c.ByteKey(buf)
		expected := fmt.Sprintf(keyFmt, c.Pid, c.Source.String(), c.SPort, c.Dest.String(), c.DPort, c.Family, c.Type)
		assert.Equal(t, expected, BeautifyKey(string(bk)))
	}
}

func TestConnStatsByteKey(t *testing.T) {
	buf := make([]byte, ConnectionByteKeyMaxLen)
	addrA := util.AddressFromString("127.0.0.1")
	addrB := util.AddressFromString("127.0.0.2")

	for _, test := range []struct {
		a ConnectionStats
		b ConnectionStats
	}{
		{ // Port is different
			a: ConnectionStats{Source: addrA, Dest: addrB, Pid: 1},
			b: ConnectionStats{Source: addrA, Dest: addrB},
		},
		{ // Family is different
			a: ConnectionStats{Source: addrA, Dest: addrB, Family: 1},
			b: ConnectionStats{Source: addrA, Dest: addrB},
		},
		{ // Type is different
			a: ConnectionStats{Source: addrA, Dest: addrB, Type: 1},
			b: ConnectionStats{Source: addrA, Dest: addrB},
		},
		{ // Source is different
			a: ConnectionStats{Source: util.AddressFromString("123.255.123.0"), Dest: addrB},
			b: ConnectionStats{Source: addrA, Dest: addrB},
		},
		{ // Dest is different
			a: ConnectionStats{Source: addrA, Dest: util.AddressFromString("129.0.1.2")},
			b: ConnectionStats{Source: addrA, Dest: addrB},
		},
		{ // Source port is different
			a: ConnectionStats{Source: addrA, Dest: addrB, SPort: 1},
			b: ConnectionStats{Source: addrA, Dest: addrB},
		},
		{ // Dest port is different
			a: ConnectionStats{Source: addrA, Dest: addrB, DPort: 1},
			b: ConnectionStats{Source: addrA, Dest: addrB},
		},
		{ // Fields set, but sources are different
			a: ConnectionStats{Pid: 1, Family: 0, Type: 1, Source: addrA, Dest: addrB},
			b: ConnectionStats{Pid: 1, Family: 0, Type: 1, Source: addrB, Dest: addrB},
		},
		{ // Both sources and dest are different
			a: ConnectionStats{Pid: 1, Dest: addrB, Family: 0, Type: 1, Source: addrA},
			b: ConnectionStats{Pid: 1, Dest: addrA, Family: 0, Type: 1, Source: addrB},
		},
		{ // Family and Type are different
			a: ConnectionStats{Pid: 1, Source: addrA, Dest: addrB, Family: 1},
			b: ConnectionStats{Pid: 1, Source: addrA, Dest: addrB, Type: 1},
		},
	} {
		var keyA, keyB string
		keyA = string(test.a.ByteKey(buf))
		keyB = string(test.b.ByteKey(buf))
		assert.NotEqual(t, keyA, keyB)
	}
}

func TestByteKeyNAT(t *testing.T) {
	buf := make([]byte, ConnectionByteKeyMaxLen)
	for _, test := range []struct {
		a           ConnectionStats
		b           ConnectionStats
		shouldMatch bool
	}{
		{
			a: ConnectionStats{
				Source: util.AddressFromString("127.0.0.1"),
				Dest:   util.AddressFromString("127.0.0.2"),
			},
			b: ConnectionStats{
				Source: util.AddressFromString("127.0.0.1"),
				Dest:   util.AddressFromString("127.0.0.2"),
			},
			shouldMatch: true,
		},
		{
			a: ConnectionStats{
				Source: util.AddressFromString("127.0.0.1"),
				Dest:   util.AddressFromString("127.0.0.2"),
			},
			b: ConnectionStats{
				Source: util.AddressFromString("127.0.0.3"),
				Dest:   util.AddressFromString("127.0.0.4"),
			},
			shouldMatch: false,
		},
		{
			a: ConnectionStats{
				Source: util.AddressFromString("127.0.0.1"),
				Dest:   util.AddressFromString("127.0.0.2"),
			},
			b: ConnectionStats{
				Source: util.AddressFromString("127.0.0.1"),
				Dest:   util.AddressFromString("127.0.0.2"),
				IPTranslation: &IPTranslation{
					ReplSrcIP: util.AddressFromString("1.1.1.1"),
					ReplDstIP: util.AddressFromString("2.2.2.2"),
				},
			},
			shouldMatch: false,
		},
		{
			a: ConnectionStats{
				Source: util.AddressFromString("127.0.0.1"),
				Dest:   util.AddressFromString("127.0.0.2"),
				IPTranslation: &IPTranslation{
					ReplSrcIP: util.AddressFromString("1.1.1.1"),
					ReplDstIP: util.AddressFromString("2.2.2.2"),
				},
			},
			b: ConnectionStats{
				Source: util.AddressFromString("127.0.0.1"),
				Dest:   util.AddressFromString("127.0.0.2"),
				IPTranslation: &IPTranslation{
					ReplSrcIP: util.AddressFromString("3.3.3.3"),
					ReplDstIP: util.AddressFromString("4.4.4.4"),
				},
			},
			shouldMatch: false,
		},
		{
			a: ConnectionStats{
				Source: util.AddressFromString("127.0.0.1"),
				Dest:   util.AddressFromString("127.0.0.2"),
				IPTranslation: &IPTranslation{
					ReplSrcIP: util.AddressFromString("1.1.1.1"),
					ReplDstIP: util.AddressFromString("2.2.2.2"),
				},
			},
			b: ConnectionStats{
				Source: util.AddressFromString("127.0.0.3"),
				Dest:   util.AddressFromString("127.0.0.4"),
				IPTranslation: &IPTranslation{
					ReplSrcIP: util.AddressFromString("1.1.1.1"),
					ReplDstIP: util.AddressFromString("2.2.2.2"),
				},
			},
			shouldMatch: true,
		},
	} {
		var keyA, keyB string
		keyA = string(test.a.ByteKeyNAT(buf))
		keyB = string(test.b.ByteKeyNAT(buf))
		actual := keyA == keyB
		assert.Equalf(t, test.shouldMatch, actual,
			"a: %s\nb:%s\nshouldMatch: %v\ngot: %v", test.a, test.b, test.shouldMatch, actual,
		)
	}
}

func TestIsExpired(t *testing.T) {
	// 10mn
	var timeout uint64 = 600000000000
	for _, tc := range []struct {
		stats      ConnectionStats
		latestTime uint64
		expected   bool
	}{
		{
			ConnectionStats{LastUpdateEpoch: 101},
			100,
			false,
		},
		{
			ConnectionStats{LastUpdateEpoch: 100},
			101,
			false,
		},
		{
			ConnectionStats{LastUpdateEpoch: 100},
			101 + timeout,
			true,
		},
	} {
		assert.Equal(t, tc.expected, tc.stats.IsExpired(tc.latestTime, timeout))
	}
}

func TestKeyTuplesFromConn(t *testing.T) {
	sourceAddress := util.AddressFromString("1.2.3.4")
	sourcePort := uint16(1234)
	destinationAddress := util.AddressFromString("5.6.7.8")
	destinationPort := uint16(5678)

	connectionStats := ConnectionStats{
		Source: sourceAddress,
		SPort:  sourcePort,
		Dest:   destinationAddress,
		DPort:  destinationPort,
	}
	keyTuples := HTTPKeyTuplesFromConn(connectionStats)

	assert.Len(t, keyTuples, 2, "Expected different number of key tuples")
	assert.True(t, slices.ContainsFunc(keyTuples, func(keyTuple http.KeyTuple) bool {
		sourceAddressLow, sourceAddressHigh := util.ToLowHigh(sourceAddress)
		destinationAddressLow, destinationAddressHigh := util.ToLowHigh(destinationAddress)
		return (keyTuple.SrcIPLow == sourceAddressLow) && (keyTuple.SrcIPHigh == sourceAddressHigh) &&
			(keyTuple.DstIPLow == destinationAddressLow) && (keyTuple.DstIPHigh == destinationAddressHigh) &&
			(keyTuple.SrcPort == sourcePort) && (keyTuple.DstPort == destinationPort)
	}), "Missing original connection")
	assert.True(t, slices.ContainsFunc(keyTuples, func(keyTuple http.KeyTuple) bool {
		sourceAddressLow, sourceAddressHigh := util.ToLowHigh(sourceAddress)
		destinationAddressLow, destinationAddressHigh := util.ToLowHigh(destinationAddress)
		return (keyTuple.SrcIPLow == destinationAddressLow) && (keyTuple.SrcIPHigh == destinationAddressHigh) &&
			(keyTuple.DstIPLow == sourceAddressLow) && (keyTuple.DstIPHigh == sourceAddressHigh) &&
			(keyTuple.SrcPort == destinationPort) && (keyTuple.DstPort == sourcePort)
	}), "Missing flipped connection")
}

func TestKeyTuplesFromConnNAT(t *testing.T) {
	sourceAddress := util.AddressFromString("1.2.3.4")
	sourcePort := uint16(1234)
	destinationAddress := util.AddressFromString("5.6.7.8")
	destinationPort := uint16(5678)

	natSourceAddress := util.AddressFromString("10.20.30.40")
	natSourcePort := uint16(4321)
	natDestinationAddress := util.AddressFromString("50.60.70.80")
	natDestinationPort := uint16(8765)

	connectionStats := ConnectionStats{
		Source: sourceAddress,
		Dest:   destinationAddress,
		SPort:  sourcePort,
		DPort:  destinationPort,
		IPTranslation: &IPTranslation{
			ReplSrcIP:   natSourceAddress,
			ReplDstIP:   natDestinationAddress,
			ReplSrcPort: natSourcePort,
			ReplDstPort: natDestinationPort,
		},
	}
	keyTuples := HTTPKeyTuplesFromConn(connectionStats)

	// Expecting 2 non NAT'd keys and 2 NAT'd keys
	assert.Len(t, keyTuples, 4, "Expected different number of key tuples")

	assert.True(t, slices.ContainsFunc(keyTuples, func(keyTuple http.KeyTuple) bool {
		sourceAddressLow, sourceAddressHigh := util.ToLowHigh(sourceAddress)
		destinationAddressLow, destinationAddressHigh := util.ToLowHigh(destinationAddress)
		return (keyTuple.SrcIPLow == sourceAddressLow) && (keyTuple.SrcIPHigh == sourceAddressHigh) &&
			(keyTuple.DstIPLow == destinationAddressLow) && (keyTuple.DstIPHigh == destinationAddressHigh) &&
			(keyTuple.SrcPort == sourcePort) && (keyTuple.DstPort == destinationPort)
	}), "Missing original connection")
	assert.True(t, slices.ContainsFunc(keyTuples, func(keyTuple http.KeyTuple) bool {
		sourceAddressLow, sourceAddressHigh := util.ToLowHigh(sourceAddress)
		destinationAddressLow, destinationAddressHigh := util.ToLowHigh(destinationAddress)
		return (keyTuple.SrcIPLow == destinationAddressLow) && (keyTuple.SrcIPHigh == destinationAddressHigh) &&
			(keyTuple.DstIPLow == sourceAddressLow) && (keyTuple.DstIPHigh == sourceAddressHigh) &&
			(keyTuple.SrcPort == destinationPort) && (keyTuple.DstPort == sourcePort)
	}), "Missing flipped connection")

	assert.True(t, slices.ContainsFunc(keyTuples, func(keyTuple http.KeyTuple) bool {
		sourceAddressLow, sourceAddressHigh := util.ToLowHigh(natSourceAddress)
		destinationAddressLow, destinationAddressHigh := util.ToLowHigh(natDestinationAddress)
		return (keyTuple.SrcIPLow == sourceAddressLow) && (keyTuple.SrcIPHigh == sourceAddressHigh) &&
			(keyTuple.DstIPLow == destinationAddressLow) && (keyTuple.DstIPHigh == destinationAddressHigh) &&
			(keyTuple.SrcPort == natSourcePort) && (keyTuple.DstPort == natDestinationPort)
	}), "Missing NAT'd connection")
	assert.True(t, slices.ContainsFunc(keyTuples, func(keyTuple http.KeyTuple) bool {
		sourceAddressLow, sourceAddressHigh := util.ToLowHigh(natSourceAddress)
		destinationAddressLow, destinationAddressHigh := util.ToLowHigh(natDestinationAddress)
		return (keyTuple.SrcIPLow == destinationAddressLow) && (keyTuple.SrcIPHigh == destinationAddressHigh) &&
			(keyTuple.DstIPLow == sourceAddressLow) && (keyTuple.DstIPHigh == sourceAddressHigh) &&
			(keyTuple.SrcPort == natDestinationPort) && (keyTuple.DstPort == natSourcePort)
	}), "Missing flipped NAT'd connection")
}

func BenchmarkByteKey(b *testing.B) {
	buf := make([]byte, ConnectionByteKeyMaxLen)
	addrA := util.AddressFromString("127.0.0.1")
	addrB := util.AddressFromString("127.0.0.2")
	c := ConnectionStats{Pid: 1, Dest: addrB, Family: 0, Type: 1, Source: addrA}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.ByteKey(buf)
	}
	runtime.KeepAlive(buf)
}
