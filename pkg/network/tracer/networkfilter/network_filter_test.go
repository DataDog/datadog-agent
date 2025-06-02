// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package networkfilter

import (
	"math/rand"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

var testSourceFilters = map[string][]string{
	"172.0.0.1":     {"80", "10", "443"},
	"*":             {"9000"}, // only port 9000
	"::7f00:35:0:0": {"443"},  // ipv6
	"10.0.0.10":     {"3333", "*"},
	"10.0.0.25":     {"30", "ABCD"}, // invalid config
	"123.ABCD":      {"*"},          // invalid config

	"172.0.0.2":     {"80", "10", "443", "53361-53370", "100-100"},
	"::7f00:35:0:1": {"65536"}, // invalid port
	"10.0.0.11":     {"3333", "*", "53361-53370"},
	"10.0.0.26":     {"30", "53361-53360"}, // invalid port range
	"10.0.0.1":      {"tcp *", "53361-53370"},
	"10.0.0.2":      {"tcp 53361-53500", "udp 119", "udp 53361"},
}

var testDestinationFilters = map[string][]string{
	"10.0.0.0/24":      {"8080", "8081", "10255"},
	"":                 {"1234"}, // invalid config
	"2001:db8::2:1":    {"5001"},
	"1.1.1.1":          {"udp *", "tcp 11211"},
	"2001:db8::2:1/55": {"80"},
	"*":                {"*"}, // invalid config

	"2001:db8::2:2":    {"3333 udp"}, // /invalid config
	"10.0.0.3/24":      {"30-ABC"},   // invalid port range
	"10.0.0.4":         {"udp *", "*"},
	"2001:db8::2:2/55": {"8080-8082-8085"}, // invalid config
	"10.0.0.5":         {"0-1"},            // invalid port 0
}

func TestParseConnectionFilters(t *testing.T) {
	sourceList := ParseConnectionFilters(testSourceFilters)
	destList := ParseConnectionFilters(testDestinationFilters)

	// source
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.MustParseAddrPort("172.0.0.1:10"), Type: model.ConnectionType_tcp}))
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.MustParseAddrPort("1.2.3.4:9000"), Type: model.ConnectionType_tcp})) // only port 9000
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.MustParseAddrPort("10.0.1.24:9000"), Type: model.ConnectionType_tcp}))
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.MustParseAddrPort("[::7f00:35:0:0]:443"), Type: model.ConnectionType_tcp})) // ipv6
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.MustParseAddrPort("[::]:443"), Type: model.ConnectionType_tcp}))            // 0 == ::7f00:35:0:0
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.MustParseAddrPort("10.0.0.10:6666"), Type: model.ConnectionType_tcp}))      // port wildcard
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.MustParseAddrPort("10.0.0.10:33"), Type: model.ConnectionType_tcp}))

	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.MustParseAddrPort("172.0.0.2:100"), Type: model.ConnectionType_tcp}))
	assert.False(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.MustParseAddrPort("[::7f00:35:0:1]:100"), Type: model.ConnectionType_tcp})) // invalid port
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.MustParseAddrPort("10.0.0.11:6666"), Type: model.ConnectionType_tcp}))       // port wildcard
	assert.False(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.MustParseAddrPort("10.0.0.26:30"), Type: model.ConnectionType_tcp}))        // invalid port range
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.MustParseAddrPort("10.0.0.1:100"), Type: model.ConnectionType_tcp}))         // tcp wildcard
	assert.False(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.MustParseAddrPort("10.0.0.2:53363"), Type: model.ConnectionType_udp}))
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.MustParseAddrPort("10.0.0.2:53361"), Type: model.ConnectionType_tcp}))
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.MustParseAddrPort("10.0.0.2:53361"), Type: model.ConnectionType_udp}))

	// destination
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Dest: netip.MustParseAddrPort("10.0.0.5:8080"), Type: model.ConnectionType_tcp}))
	assert.False(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Dest: netip.MustParseAddrPort("10.0.0.5:80"), Type: model.ConnectionType_tcp}))
	assert.False(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Dest: netip.MustParseAddrPort("0.0.0.0:1234"), Type: model.ConnectionType_tcp}))
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Dest: netip.MustParseAddrPort("[2001:db8::2:1]:5001"), Type: model.ConnectionType_tcp})) // ipv6
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Dest: netip.MustParseAddrPort("[2001:db8::5:1]:80"), Type: model.ConnectionType_tcp}))   // ipv6 CIDR
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Dest: netip.MustParseAddrPort("1.1.1.1:11211"), Type: model.ConnectionType_tcp}))
	assert.False(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Dest: netip.MustParseAddrPort("1.2.3.4:30"), Type: model.ConnectionType_tcp}))

	assert.False(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Dest: netip.MustParseAddrPort("[2001:db8::2:2]:3333"), Type: model.ConnectionType_udp})) // invalid config
	assert.False(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Dest: netip.MustParseAddrPort("10.0.0.3:80"), Type: model.ConnectionType_tcp}))          // invalid config
	assert.True(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Dest: netip.MustParseAddrPort("10.0.0.4:1234"), Type: model.ConnectionType_tcp}))         // port wildcard
	assert.False(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Dest: netip.MustParseAddrPort("[2001:db8::2:2]:8082"), Type: model.ConnectionType_tcp})) // invalid config
	assert.False(t, IsExcludedConnection(sourceList, destList, FilterableConnection{Dest: netip.MustParseAddrPort("10.0.0.5:0"), Type: model.ConnectionType_tcp}))           // invalid port
}

var sink bool

func BenchmarkIsBlacklistedConnectionIPv4(b *testing.B) {
	sourceList := ParseConnectionFilters(testSourceFilters)
	destList := ParseConnectionFilters(testDestinationFilters)
	addrs := randIPv4(6)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, addr := range addrs {
			sink = IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.AddrPortFrom(addr, uint16(rand.Intn(9999))), Type: model.ConnectionType_tcp})
			sink = IsExcludedConnection(sourceList, destList, FilterableConnection{Dest: netip.AddrPortFrom(addr, uint16(rand.Intn(9999))), Type: model.ConnectionType_tcp})
		}
	}

}

func BenchmarkIsBlacklistedConnectionIPv6(b *testing.B) {
	sourceList := ParseConnectionFilters(testSourceFilters)
	destList := ParseConnectionFilters(testDestinationFilters)
	addrs := randIPv6(6)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, addr := range addrs {
			sink = IsExcludedConnection(sourceList, destList, FilterableConnection{Source: netip.AddrPortFrom(addr, uint16(rand.Intn(9999))), Type: model.ConnectionType_tcp})
			sink = IsExcludedConnection(sourceList, destList, FilterableConnection{Dest: netip.AddrPortFrom(addr, uint16(rand.Intn(9999))), Type: model.ConnectionType_tcp})
		}
	}
}

func randIPv4(count int) (addrs []netip.Addr) {
	for i := 0; i < count; i++ {
		r := rand.Intn(999999999) + 999999
		addrs = append(addrs, util.V4Address(uint32(r)).Addr)
	}
	return addrs
}

func randIPv6(count int) (addrs []netip.Addr) {
	for i := 0; i < count; i++ {
		r := rand.Intn(999999999) + 999999
		addrs = append(addrs, util.V6Address(uint64(r), uint64(r)).Addr)
	}
	return addrs
}
