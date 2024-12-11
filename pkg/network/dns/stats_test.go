// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || linux_bpf

package dns

import (
	"fmt"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

const (
	DNSTimeoutSecs = 10
)

func getSampleDNSKey() Key {
	return Key{
		ServerIP:   util.AddressFromString("8.8.8.8"),
		ClientIP:   util.AddressFromString("1.1.1.1"),
		ClientPort: 1000,
		Protocol:   syscall.IPPROTO_UDP,
	}
}

func testLatency(
	t *testing.T,
	respType packetType,
	delta time.Duration,
	expectedSuccessLatency uint64,
	expectedFailureLatency uint64,
	expectedTimeouts uint32,
) {
	var d = ToHostname("abc.com")
	sk := newDNSStatkeeper(DNSTimeoutSecs*time.Second, 10000)
	key := getSampleDNSKey()
	qPkt := dnsPacketInfo{transactionID: 1, pktType: query, key: key, question: d, queryType: TypeA}
	then := time.Now()
	sk.ProcessPacketInfo(qPkt, then)
	stats := sk.GetAndResetAllStats()
	assert.NotContains(t, stats, key)

	now := then.Add(delta)
	rPkt := dnsPacketInfo{transactionID: 1, key: key, pktType: respType, queryType: TypeA}

	sk.ProcessPacketInfo(rPkt, now)
	stats = sk.GetAndResetAllStats()
	require.Contains(t, stats, key)
	require.Contains(t, stats[key], d)

	assert.Equal(t, expectedSuccessLatency, stats[key][d][TypeA].SuccessLatencySum)
	assert.Equal(t, expectedFailureLatency, stats[key][d][TypeA].FailureLatencySum)
	assert.Equal(t, expectedTimeouts, stats[key][d][TypeA].Timeouts)
}

func TestSuccessLatency(t *testing.T) {
	delta := 10 * time.Microsecond
	testLatency(t, successfulResponse, delta, uint64(delta.Microseconds()), 0, 0)
}

func TestFailureLatency(t *testing.T) {
	delta := 10 * time.Microsecond
	testLatency(t, failedResponse, delta, 0, uint64(delta.Microseconds()), 0)
}

func TestTimeout(t *testing.T) {
	delta := DNSTimeoutSecs*time.Second + 10*time.Microsecond
	testLatency(t, successfulResponse, delta, 0, 0, 1)
}

func TestExpiredStateRemoval(t *testing.T) {
	sk := newDNSStatkeeper(DNSTimeoutSecs*time.Second, 10000)
	key := getSampleDNSKey()
	var d = ToHostname("abc.com")
	qPkt1 := dnsPacketInfo{transactionID: 1, pktType: query, key: key, question: d, queryType: TypeA}
	rPkt1 := dnsPacketInfo{transactionID: 1, key: key, pktType: successfulResponse, queryType: TypeA}
	qPkt2 := dnsPacketInfo{transactionID: 2, pktType: query, key: key, question: d, queryType: TypeA}
	qPkt3 := dnsPacketInfo{transactionID: 3, pktType: query, key: key, question: d, queryType: TypeA}
	rPkt3 := dnsPacketInfo{transactionID: 3, key: key, pktType: successfulResponse, queryType: TypeA}

	sk.ProcessPacketInfo(qPkt1, time.Now())
	sk.ProcessPacketInfo(rPkt1, time.Now())

	sk.ProcessPacketInfo(qPkt2, time.Now())
	sk.removeExpiredStates(time.Now().Add(DNSTimeoutSecs * time.Second))

	sk.ProcessPacketInfo(qPkt3, time.Now())
	sk.ProcessPacketInfo(rPkt3, time.Now())

	stats := sk.GetAndResetAllStats()
	require.Contains(t, stats, key)
	require.Contains(t, stats[key], d)

	require.Contains(t, stats[key][d][TypeA].CountByRcode, uint32(0))
	assert.Equal(t, uint32(2), stats[key][d][TypeA].CountByRcode[0])
	assert.Equal(t, uint32(1), stats[key][d][TypeA].Timeouts)
}

func BenchmarkStats(b *testing.B) {
	key := getSampleDNSKey()

	var packets []dnsPacketInfo
	for j := 0; j < maxStateMapSize*2; j++ {
		qPkt := dnsPacketInfo{pktType: query, key: key}
		qPkt.transactionID = uint16(j)
		packets = append(packets, qPkt)
	}
	ts := time.Now()

	// Benchmark map size with different number of packets
	for _, numPackets := range []int{maxStateMapSize / 10, maxStateMapSize / 2, maxStateMapSize, maxStateMapSize * 2} {
		b.Run(fmt.Sprintf("Packets#-%d", numPackets), func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sk := newDNSStatkeeper(1000*time.Second, 10000)
				for j := 0; j < numPackets; j++ {
					sk.ProcessPacketInfo(packets[j], ts)
				}
			}
		})
	}
}
