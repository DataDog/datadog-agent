package network

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

const (
	DNSTimeoutSecs = 10
)

func getSampleDNSKey() DNSKey {
	return DNSKey{
		serverIP:   util.AddressFromString("8.8.8.8"),
		clientIP:   util.AddressFromString("1.1.1.1"),
		clientPort: 1000,
		protocol:   UDP,
	}
}

func testLatency(
	t *testing.T,
	respType DNSPacketType,
	delta time.Duration,
	expectedSuccessLatency uint64,
	expectedFailureLatency uint64,
	expectedTimeouts uint32,
) {
	var d = "abc.com"
	sk := newDNSStatkeeper(DNSTimeoutSecs*time.Second, 10000)
	key := getSampleDNSKey()
	qPkt := dnsPacketInfo{transactionID: 1, pktType: Query, key: key, question: d}
	then := time.Now()
	sk.ProcessPacketInfo(qPkt, then)
	stats := sk.GetAndResetAllStats()
	assert.NotContains(t, stats, key)

	now := then.Add(delta)
	rPkt := dnsPacketInfo{transactionID: 1, key: key, pktType: respType}

	sk.ProcessPacketInfo(rPkt, now)
	stats = sk.GetAndResetAllStats()
	require.Contains(t, stats, key)
	require.Contains(t, stats[key], d)

	assert.Equal(t, expectedSuccessLatency, stats[key][d].DNSSuccessLatencySum)
	assert.Equal(t, expectedFailureLatency, stats[key][d].DNSFailureLatencySum)
	assert.Equal(t, expectedTimeouts, stats[key][d].DNSTimeouts)
}

func TestSuccessLatency(t *testing.T) {
	delta := 10 * time.Microsecond
	testLatency(t, SuccessfulResponse, delta, uint64(delta.Microseconds()), 0, 0)
}

func TestFailureLatency(t *testing.T) {
	delta := 10 * time.Microsecond
	testLatency(t, FailedResponse, delta, 0, uint64(delta.Microseconds()), 0)
}

func TestTimeout(t *testing.T) {
	delta := DNSTimeoutSecs*time.Second + 10*time.Microsecond
	testLatency(t, SuccessfulResponse, delta, 0, 0, 1)
}

func TestExpiredStateRemoval(t *testing.T) {
	sk := newDNSStatkeeper(DNSTimeoutSecs*time.Second, 10000)
	key := getSampleDNSKey()
	var d = "abc.com"
	qPkt1 := dnsPacketInfo{transactionID: 1, pktType: Query, key: key, question: d}
	rPkt1 := dnsPacketInfo{transactionID: 1, key: key, pktType: SuccessfulResponse}
	qPkt2 := dnsPacketInfo{transactionID: 2, pktType: Query, key: key, question: d}
	qPkt3 := dnsPacketInfo{transactionID: 3, pktType: Query, key: key, question: d}
	rPkt3 := dnsPacketInfo{transactionID: 3, key: key, pktType: SuccessfulResponse}

	sk.ProcessPacketInfo(qPkt1, time.Now())
	sk.ProcessPacketInfo(rPkt1, time.Now())

	sk.ProcessPacketInfo(qPkt2, time.Now())
	sk.removeExpiredStates(time.Now().Add(DNSTimeoutSecs * time.Second))

	sk.ProcessPacketInfo(qPkt3, time.Now())
	sk.ProcessPacketInfo(rPkt3, time.Now())

	stats := sk.GetAndResetAllStats()
	require.Contains(t, stats, key)
	require.Contains(t, stats[key], d)

	require.Contains(t, stats[key][d].DNSCountByRcode, uint32(0))
	assert.Equal(t, uint32(2), stats[key][d].DNSCountByRcode[0])
	assert.Equal(t, uint32(1), stats[key][d].DNSTimeouts)
}

func BenchmarkStats(b *testing.B) {
	key := getSampleDNSKey()

	var packets []dnsPacketInfo
	for j := 0; j < MaxStateMapSize*2; j++ {
		qPkt := dnsPacketInfo{pktType: Query, key: key}
		qPkt.transactionID = uint16(j)
		packets = append(packets, qPkt)
	}
	ts := time.Now()

	// Benchmark map size with different number of packets
	for _, numPackets := range []int{MaxStateMapSize / 10, MaxStateMapSize / 2, MaxStateMapSize, MaxStateMapSize * 2} {
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
