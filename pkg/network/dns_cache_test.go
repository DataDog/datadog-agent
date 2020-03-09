package network

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
)

var disableAutomaticExpiration = 1 * time.Hour

func TestMultipleIPsForSameName(t *testing.T) {
	datadog1 := util.AddressFromString("52.85.98.155")
	datadog2 := util.AddressFromString("52.85.98.143")

	datadogIPs := newTranslation([]byte("datadoghq.com"))
	datadogIPs.add(datadog1)
	datadogIPs.add(datadog2)

	cache := newReverseDNSCache(100, 1*time.Minute, disableAutomaticExpiration)
	cache.Add(datadogIPs, time.Now())

	localhost := util.AddressFromString("127.0.0.1")
	connections := []ConnectionStats{
		{Source: localhost, Dest: datadog1},
		{Source: localhost, Dest: datadog2},
	}

	actual := cache.Get(connections, time.Now())
	expected := map[util.Address][]string{
		datadog1: {"datadoghq.com"},
		datadog2: {"datadoghq.com"},
	}
	assert.Equal(t, expected, actual)
}

func TestMultipleNamesForSameIP(t *testing.T) {
	cache := newReverseDNSCache(100, 1*time.Minute, disableAutomaticExpiration)

	raddr := util.AddressFromString("172.022.116.123")
	tr1 := newTranslation([]byte("i-03e46c9ff42db4abc"))
	tr1.add(raddr)
	tr2 := newTranslation([]byte("ip-172-22-116-123.ec2.internal"))
	tr2.add(raddr)

	now := time.Now()
	cache.Add(tr1, now)
	cache.Add(tr2, now)

	localhost := util.AddressFromString("127.0.0.1")
	connections := []ConnectionStats{{Source: localhost, Dest: raddr}}

	names := cache.Get(connections, now)
	expected := []string{"i-03e46c9ff42db4abc", "ip-172-22-116-123.ec2.internal"}
	assert.ElementsMatch(t, expected, names[raddr])
}

func TestDNSCacheExpiration(t *testing.T) {
	ttl := 100 * time.Millisecond
	cache := newReverseDNSCache(1000, ttl, disableAutomaticExpiration)
	t1 := time.Now()

	laddr1 := util.AddressFromString("127.0.0.1")
	raddr1 := util.AddressFromString("192.168.0.1") // host-a
	hostA := newTranslation([]byte("host-a"))
	hostA.add(raddr1)

	laddr2 := util.AddressFromString("127.0.0.1")
	raddr2 := util.AddressFromString("192.168.0.2") // host-b
	hostB := newTranslation([]byte("host-b"))
	hostB.add(raddr2)

	laddr3 := util.AddressFromString("127.0.0.1")
	raddr3 := util.AddressFromString("192.168.0.3") // host-c
	hostC := newTranslation([]byte("host-c"))
	hostC.add(raddr3)

	cache.Add(hostA, t1)
	cache.Add(hostB, t1)
	cache.Add(hostC, t1)
	assert.Equal(t, 3, cache.Len())

	// All entries should remain present (t2 < t1 + ttl)
	t2 := t1.Add(ttl - 10*time.Millisecond)
	cache.Expire(t2)
	assert.Equal(t, 3, cache.Len())

	// Bump host-a and host-b expiration
	stats := []ConnectionStats{
		{Source: laddr1, Dest: raddr1},
		{Source: laddr2, Dest: raddr2},
	}
	cache.Get(stats, t2)

	// Only IP from host-c should have expired
	t3 := t1.Add(ttl + 10*time.Millisecond)
	cache.Expire(t3)
	assert.Equal(t, 2, cache.Len())

	stats = []ConnectionStats{
		{Source: laddr1, Dest: raddr1},
		{Source: laddr2, Dest: raddr2},
		{Source: laddr3, Dest: raddr3},
	}
	names := cache.Get(stats, t2)
	assert.Contains(t, names[raddr1], "host-a")
	assert.Contains(t, names[raddr2], "host-b")
	assert.Nil(t, names[raddr3])

	// All entries should have expired by now
	t4 := t3.Add(ttl)
	cache.Expire(t4)
	assert.Equal(t, 0, cache.Len())
}

func TestDNSCacheTelemetry(t *testing.T) {
	ttl := 100 * time.Millisecond
	cache := newReverseDNSCache(1000, ttl, disableAutomaticExpiration)
	t1 := time.Now()

	translation := newTranslation([]byte("host-a"))
	translation.add(util.AddressFromString("192.168.0.1"))
	cache.Add(translation, t1)

	expected := map[string]int64{
		"lookups":  0,
		"resolved": 0,
		"ips":      1,
		"added":    1,
		"expired":  0,
	}
	assert.Equal(t, expected, cache.Stats())

	conns := []ConnectionStats{
		{
			Source: util.AddressFromString("127.0.0.1"),
			Dest:   util.AddressFromString("192.168.0.1"),
		},
		{
			Source: util.AddressFromString("127.0.0.1"),
			Dest:   util.AddressFromString("192.168.0.2"),
		},
	}

	// Attempt to resolve IPs
	cache.Get(conns, t1)
	expected = map[string]int64{
		"lookups":  3, // 127.0.0.1, 192.168.0.1, 192.168.0.2
		"resolved": 1, // 192.168.0.1
		"ips":      1,
		"added":    0,
		"expired":  0,
	}
	assert.Equal(t, expected, cache.Stats())

	// Expire IP
	t2 := t1.Add(ttl + 1*time.Millisecond)
	cache.Expire(t2)
	expected = map[string]int64{
		"lookups":  0,
		"resolved": 0,
		"ips":      0,
		"added":    0,
		"expired":  1,
	}
	assert.Equal(t, expected, cache.Stats())
}

func TestDNSCacheMerge(t *testing.T) {
	ttl := 100 * time.Millisecond
	cache := newReverseDNSCache(1000, ttl, disableAutomaticExpiration)

	ts := time.Now()
	conns := []ConnectionStats{
		{
			Source: util.AddressFromString("127.0.0.1"),
			Dest:   util.AddressFromString("192.168.0.1"),
		},
	}

	t1 := newTranslation([]byte("host-b"))
	t1.add(util.AddressFromString("192.168.0.1"))
	cache.Add(t1, ts)
	res := cache.Get(conns, ts)
	assert.Equal(t, []string{"host-b"}, res[util.AddressFromString("192.168.0.1")])

	t2 := newTranslation([]byte("host-a"))
	t2.add(util.AddressFromString("192.168.0.1"))
	cache.Add(t2, ts)

	t3 := newTranslation([]byte("host-b"))
	t3.add(util.AddressFromString("192.168.0.1"))
	cache.Add(t3, ts)

	res = cache.Get(conns, ts)

	assert.Equal(t, []string{"host-a", "host-b"}, res[util.AddressFromString("192.168.0.1")])
}

func BenchmarkDNSCacheGet(b *testing.B) {
	const numIPs = 10000

	// Instantiate cache and add numIPs to it
	var (
		cache   = newReverseDNSCache(numIPs, 100*time.Millisecond, disableAutomaticExpiration)
		added   = make([]util.Address, 0, numIPs)
		addrGen = randomAddressGen()
		now     = time.Now()
	)
	for i := 0; i < numIPs; i++ {
		address := addrGen()
		added = append(added, address)
		translation := newTranslation([]byte("foo.local"))
		translation.add(address)
		cache.Add(translation, now)
	}

	// Benchmark Get operation with different resolve ratios
	for _, ratio := range []float64{0.0, 0.25, 0.50, 0.75, 1.0} {
		b.Run(fmt.Sprintf("ResolveRatio-%f", ratio), func(b *testing.B) {
			stats := payloadGen(100, ratio, added)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = cache.Get(stats, now)
			}
		})
	}
}

func randomAddressGen() func() util.Address {
	b := make([]byte, 4)
	return func() util.Address {
		for {
			if _, err := rand.Read(b); err != nil {
				continue
			}

			return util.V4AddressFromBytes(b)
		}
	}
}

func payloadGen(size int, resolveRatio float64, added []util.Address) []ConnectionStats {
	var (
		addrGen = randomAddressGen()
		stats   = make([]ConnectionStats, size)
	)

	for i := 0; i < size; i++ {
		if rand.Float64() <= resolveRatio {
			stats[i].Source = added[rand.Intn(len(added))]
			stats[i].Dest = added[rand.Intn(len(added))]
			continue
		}

		stats[i].Source = addrGen()
		stats[i].Dest = addrGen()
	}

	return stats
}
