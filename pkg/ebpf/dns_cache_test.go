package ebpf

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
)

func TestMultipleIPsForSameName(t *testing.T) {
	datadog1 := util.AddressFromString("52.85.98.155")
	datadog2 := util.AddressFromString("52.85.98.143")

	datadogIPs := newTranslation([]byte("datadoghq.com"))
	datadogIPs.add(datadog1)
	datadogIPs.add(datadog2)

	cache := newReverseDNSCache(100, 1*time.Minute, 5*time.Minute)
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
	cache := newReverseDNSCache(100, 1*time.Minute, 5*time.Minute)

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
	var (
		size             = 1000
		ttl              = 100 * time.Millisecond
		expirationPeriod = 1 * time.Hour // For testing purposes
	)

	cache := newReverseDNSCache(size, ttl, expirationPeriod)
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
