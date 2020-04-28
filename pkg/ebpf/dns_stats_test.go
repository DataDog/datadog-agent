package ebpf

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func testLatency(t *testing.T, isSuccess bool) {
	sk := newDNSStatkeeper(10 * time.Second)
	key := dnsKey{
		serverIP:   util.AddressFromString("8.8.8.8"),
		clientIP:   util.AddressFromString("1.1.1.1"),
		clientPort: 1000,
		protocol:   UDP,
	}
	qPkt := dnsPacketInfo{transactionID: 1, pktType: Query, key: key}
	then := time.Now()
	sk.ProcessPacketInfo(qPkt, then)
	stats := sk.GetAndResetAllStats()
	assert.NotContains(t, stats, key)

	delta := 10 * time.Microsecond
	now := then.Add(delta)
	rPkt := dnsPacketInfo{transactionID: 1, key: key}
	if isSuccess {
		rPkt.pktType = SuccessfulResponse
	} else {
		rPkt.pktType = FailedResponse
	}

	sk.ProcessPacketInfo(rPkt, now)
	stats = sk.GetAndResetAllStats()
	require.Contains(t, stats, key)

	if isSuccess {
		assert.Equal(t, uint64(delta.Nanoseconds()/1000), stats[key].successLatencySum)
		assert.Equal(t, uint64(0), stats[key].failureLatencySum)
	} else {
		assert.Equal(t, uint64(0), stats[key].successLatencySum)
		assert.Equal(t, uint64(delta.Nanoseconds()/1000), stats[key].failureLatencySum)
	}
}

func TestSuccessLatency(t *testing.T) {
	testLatency(t, true)
}

func TestFailureLatency(t *testing.T) {
	testLatency(t, false)
}

func BenchmarkStats(b *testing.B) {
	key := dnsKey{
		serverIP:   util.AddressFromString("8.8.8.8"),
		clientIP:   util.AddressFromString("1.1.1.1"),
		clientPort: 1000,
		protocol:   UDP,
	}
	var packets []dnsPacketInfo
	for j := 0; j < 20000; j++ {
		qPkt := dnsPacketInfo{pktType: Query, key: key}
		qPkt.transactionID = uint16(j)
		packets = append(packets, qPkt)
	}
	ts := time.Now()

	// Benchmark map size with different number of packets
	for _, numPackets := range []int{1000, 5000, 10000, 20000} {
		b.Run(fmt.Sprintf("Packets#-%d", numPackets), func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sk := newDNSStatkeeper(1000 * time.Second)
				for j := 0; j < numPackets; j++ {
					sk.ProcessPacketInfo(packets[j], ts)
				}
			}
		})
	}
}
