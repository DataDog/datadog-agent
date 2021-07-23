//+build windows linux_bpf

package dns

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
)

var disableAutomaticExpiration = 1 * time.Hour

func TestMultipleIPsForSameName(t *testing.T) {
	datadog1 := util.AddressFromString("52.85.98.155")
	datadog2 := util.AddressFromString("52.85.98.143")

	datadogIPs := newTranslation("datadoghq.com")
	datadogIPs.add(datadog1, 1*time.Minute)
	datadogIPs.add(datadog2, 1*time.Minute)

	cache := newReverseDNSCache(100, disableAutomaticExpiration)
	cache.Add(datadogIPs)

	localhost := util.AddressFromString("127.0.0.1")
	connections := []util.Address{localhost, datadog1, datadog2}
	actual := cache.Get(connections)
	expected := map[util.Address][]string{
		datadog1: {"datadoghq.com"},
		datadog2: {"datadoghq.com"},
	}
	assert.Equal(t, expected, actual)
}

func TestMultipleNamesForSameIP(t *testing.T) {
	cache := newReverseDNSCache(100, disableAutomaticExpiration)

	raddr := util.AddressFromString("172.022.116.123")
	tr1 := newTranslation("i-03e46c9ff42db4abc")
	tr1.add(raddr, 1*time.Minute)
	tr2 := newTranslation("ip-172-22-116-123.ec2.internal")
	tr2.add(raddr, 1*time.Minute)

	cache.Add(tr1)
	cache.Add(tr2)

	localhost := util.AddressFromString("127.0.0.1")
	connections := []util.Address{localhost, raddr}

	names := cache.Get(connections)
	expected := []string{"i-03e46c9ff42db4abc", "ip-172-22-116-123.ec2.internal"}
	assert.ElementsMatch(t, expected, names[raddr])
}

func TestDNSCacheExpiration(t *testing.T) {
	ttl := 100 * time.Millisecond
	cache := newReverseDNSCache(1000, disableAutomaticExpiration)
	t1 := time.Now()

	laddr1 := util.AddressFromString("127.0.0.1")
	raddr1 := util.AddressFromString("192.168.0.1") // host-a
	hostA := newTranslation("host-a")
	hostA.add(raddr1, ttl+20*time.Millisecond)

	laddr2 := util.AddressFromString("127.0.0.1")
	raddr2 := util.AddressFromString("192.168.0.2") // host-b
	hostB := newTranslation("host-b")
	hostB.add(raddr2, ttl+20*time.Millisecond)

	laddr3 := util.AddressFromString("127.0.0.1")
	raddr3 := util.AddressFromString("192.168.0.3") // host-c
	hostC := newTranslation("host-c")
	hostC.add(raddr3, ttl)

	cache.Add(hostA)
	cache.Add(hostB)
	cache.Add(hostC)
	assert.Equal(t, 3, cache.Len())

	// All entries should remain present (t2 < t1 + ttl)
	t2 := t1.Add(ttl - 10*time.Millisecond)
	cache.Expire(t2)
	assert.Equal(t, 3, cache.Len())

	// Bump host-a and host-b in-use flag
	stats := []util.Address{
		laddr1, raddr1,
		laddr2, raddr2,
	}
	cache.Get(stats)

	// Only IP from host-c should have expired
	t3 := t1.Add(ttl + 10*time.Millisecond)
	cache.Expire(t3)
	assert.Equal(t, 2, cache.Len())

	stats = []util.Address{
		laddr1, raddr1,
		laddr2, raddr2,
		laddr3, raddr3,
	}
	names := cache.Get(stats)
	assert.Contains(t, names[raddr1], "host-a")
	assert.Contains(t, names[raddr2], "host-b")
	assert.Nil(t, names[raddr3])

	// entries should still be around after expiration that are referenced
	t4 := t3.Add(ttl)
	cache.Expire(t4)
	assert.Equal(t, 2, cache.Len())

	// All entries should be allowed to expire now
	cache.Get([]util.Address{})
	cache.Expire(t4)
	assert.Equal(t, 0, cache.Len())
}

func TestDNSCacheTelemetry(t *testing.T) {
	ttl := 100 * time.Millisecond
	cache := newReverseDNSCache(1000, disableAutomaticExpiration)
	t1 := time.Now()

	translation := newTranslation("host-a")
	translation.add(util.AddressFromString("192.168.0.1"), ttl)
	cache.Add(translation)

	expected := map[string]int64{
		"lookups":   0,
		"resolved":  0,
		"ips":       1,
		"added":     1,
		"expired":   0,
		"oversized": 0,
	}
	assert.Equal(t, expected, cache.Stats())

	conns := []util.Address{
		util.AddressFromString("127.0.0.1"),
		util.AddressFromString("192.168.0.1"),
		util.AddressFromString("127.0.0.1"),
		util.AddressFromString("192.168.0.2"),
	}

	// Attempt to resolve IPs
	cache.Get(conns)
	expected = map[string]int64{
		"lookups":   3, // 127.0.0.1, 192.168.0.1, 192.168.0.2
		"resolved":  1, // 192.168.0.1
		"ips":       1,
		"added":     1,
		"expired":   0,
		"oversized": 0,
	}
	assert.Equal(t, expected, cache.Stats())

	// Expire IP
	t2 := t1.Add(ttl + 1*time.Millisecond)
	cache.Get([]util.Address{})
	cache.Expire(t2)
	expected = map[string]int64{
		"lookups":   3,
		"resolved":  1,
		"ips":       0,
		"added":     1,
		"expired":   1,
		"oversized": 0,
	}
	assert.Equal(t, expected, cache.Stats())
}

func TestDNSCacheMerge(t *testing.T) {
	ttl := 100 * time.Millisecond
	cache := newReverseDNSCache(1000, disableAutomaticExpiration)

	conns := []util.Address{
		util.AddressFromString("127.0.0.1"),
		util.AddressFromString("192.168.0.1"),
	}

	t1 := newTranslation("host-b")
	t1.add(util.AddressFromString("192.168.0.1"), ttl)
	cache.Add(t1)
	res := cache.Get(conns)
	assert.Equal(t, []string{"host-b"}, res[util.AddressFromString("192.168.0.1")])

	t2 := newTranslation("host-a")
	t2.add(util.AddressFromString("192.168.0.1"), ttl)
	cache.Add(t2)

	t3 := newTranslation("host-b")
	t3.add(util.AddressFromString("192.168.0.1"), ttl)
	cache.Add(t3)

	res = cache.Get(conns)

	assert.Equal(t, []string{"host-a", "host-b"}, res[util.AddressFromString("192.168.0.1")])
}

func TestDNSCacheMerge_MixedCaseNames(t *testing.T) {
	ttl := 100 * time.Millisecond
	cache := newReverseDNSCache(1000, disableAutomaticExpiration)

	conns := []util.Address{
		util.AddressFromString("192.168.0.1"),
	}

	tr := newTranslation("host.name.com")
	tr.add(util.AddressFromString("192.168.0.1"), ttl)
	cache.Add(tr)

	tr = newTranslation("host.NaMe.com")
	tr.add(util.AddressFromString("192.168.0.1"), ttl)
	cache.Add(tr)

	tr = newTranslation("HOST.NAME.CoM")
	tr.add(util.AddressFromString("192.168.0.1"), ttl)
	cache.Add(tr)

	res := cache.Get(conns)
	assert.Equal(t, []string{"host.name.com"}, res[util.AddressFromString("192.168.0.1")])
}

func TestGetOversizedDNS(t *testing.T) {
	cache := newReverseDNSCache(1000, time.Minute)
	cache.maxDomainsPerIP = 10
	addr := util.AddressFromString("192.168.0.1")
	exp := time.Now().Add(1 * time.Hour)

	for i := 0; i < 5; i++ {
		cache.Add(&translation{
			dns: fmt.Sprintf("%d.host.com", i),
			ips: map[util.Address]time.Time{addr: exp},
		})
	}

	conns := []util.Address{addr}

	result := cache.Get(conns)
	assert.Len(t, result[addr], 5)
	assert.Len(t, cache.data[addr].names, 5)

	for i := 5; i < 100; i++ {
		cache.Add(&translation{
			dns: fmt.Sprintf("%d.host.com", i),
			ips: map[util.Address]time.Time{addr: exp},
		})
	}

	conns = []util.Address{addr}
	result = cache.Get(conns)
	assert.Len(t, result[addr], 0)
	assert.Len(t, cache.data[addr].names, 10)
}

func BenchmarkDNSCacheGet(b *testing.B) {
	const numIPs = 10000

	// Instantiate cache and add numIPs to it
	var (
		cache   = newReverseDNSCache(numIPs, disableAutomaticExpiration)
		added   = make([]util.Address, 0, numIPs)
		addrGen = randomAddressGen()
	)
	for i := 0; i < numIPs; i++ {
		address := addrGen()
		added = append(added, address)
		translation := newTranslation("foo.local")
		translation.add(address, 100*time.Millisecond)
		cache.Add(translation)
	}

	// Benchmark Get operation with different resolve ratios
	for _, ratio := range []float64{0.0, 0.25, 0.50, 0.75, 1.0} {
		b.Run(fmt.Sprintf("ResolveRatio-%f", ratio), func(b *testing.B) {
			stats := payloadGen(100, ratio, added)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = cache.Get(stats)
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

func payloadGen(size int, resolveRatio float64, added []util.Address) []util.Address {
	var (
		addrGen = randomAddressGen()
		stats   = make([]util.Address, size)
	)

	for i := 0; i < size; i++ {
		if rand.Float64() <= resolveRatio {
			stats[i] = added[rand.Intn(len(added))]
			continue
		}

		stats[i] = addrGen()
	}

	return stats
}

func newTranslation(domain string) *translation {
	return &translation{
		dns: strings.ToLower(domain),
		ips: make(map[util.Address]time.Time),
	}
}
