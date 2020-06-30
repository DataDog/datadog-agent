package network

import (
	"math/rand"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
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
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("172.0.0.1"), SPort: uint16(10), Type: TCP}))
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("*"), SPort: uint16(9000), Type: TCP})) // only port 9000
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("10.0.1.24"), SPort: uint16(9000), Type: TCP}))
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("::7f00:35:0:0"), SPort: uint16(443), Type: TCP})) // ipv6
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("0"), SPort: uint16(443), Type: TCP}))             // 0 == ::7f00:35:0:0
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("10.0.0.10"), SPort: uint16(6666), Type: TCP}))    // port wildcard
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("10.0.0.10"), SPort: uint16(33), Type: TCP}))
	assert.False(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("10.0.0.25"), SPort: uint16(30), Type: TCP})) // bad config
	assert.False(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("123.ABCD"), SPort: uint16(30), Type: TCP}))  // bad IP config

	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("172.0.0.2"), SPort: uint16(100), Type: TCP}))
	assert.False(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("::7f00:35:0:1"), SPort: uint16(100), Type: TCP})) // invalid port
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("10.0.0.11"), SPort: uint16(6666), Type: TCP}))     // port wildcard
	assert.False(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("10.0.0.26"), SPort: uint16(30), Type: TCP}))      // invalid port range
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("10.0.0.1"), SPort: uint16(100), Type: TCP}))       // tcp wildcard
	assert.False(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("10.0.0.2"), SPort: uint16(53363), Type: UDP}))
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("10.0.0.2"), SPort: uint16(53361), Type: TCP}))
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: util.AddressFromString("10.0.0.2"), SPort: uint16(53361), Type: UDP}))

	// destination
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Dest: util.AddressFromString("10.0.0.5"), DPort: uint16(8080), Type: TCP}))
	assert.False(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Dest: util.AddressFromString("10.0.0.5"), DPort: uint16(80), Type: TCP}))
	assert.False(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Dest: util.AddressFromString(""), DPort: uint16(1234), Type: TCP}))
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Dest: util.AddressFromString("2001:db8::2:1"), DPort: uint16(5001), Type: TCP})) // ipv6
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Dest: util.AddressFromString("2001:db8::5:1"), DPort: uint16(80), Type: TCP}))   // ipv6 CIDR
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Dest: util.AddressFromString("1.1.1.1"), DPort: uint16(11211), Type: TCP}))
	assert.False(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Dest: util.AddressFromString("*"), DPort: uint16(30), Type: TCP}))

	assert.False(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Dest: util.AddressFromString("2001:db8::2:2"), DPort: uint16(3333), Type: UDP})) // invalid config
	assert.False(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Dest: util.AddressFromString("10.0.0.3/24"), DPort: uint16(80), Type: TCP}))     // invalid config
	assert.True(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Dest: util.AddressFromString("10.0.0.4"), DPort: uint16(1234), Type: TCP}))       // port wildcard
	assert.False(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Dest: util.AddressFromString("2001:db8::2:2"), DPort: uint16(8082), Type: TCP})) // invalid config
	assert.False(t, IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Dest: util.AddressFromString("10.0.0.5"), DPort: uint16(0), Type: TCP}))         // invalid port
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
			sink = IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: addr, SPort: uint16(rand.Intn(9999)), Type: TCP})
			sink = IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Dest: addr, DPort: uint16(rand.Intn(9999)), Type: TCP})
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
			sink = IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Source: addr, SPort: uint16(rand.Intn(9999)), Type: TCP})
			sink = IsBlacklistedConnection(sourceList, destList, &ConnectionStats{Dest: addr, DPort: uint16(rand.Intn(9999)), Type: TCP})
		}
	}
}

func randIPv4(count int) (addrs []util.Address) {
	for i := 0; i < count; i++ {
		r := rand.Intn(999999999) + 999999
		addrs = append(addrs, util.V4Address(uint32(r)))
	}
	return addrs
}

func randIPv6(count int) (addrs []util.Address) {
	for i := 0; i < count; i++ {
		r := rand.Intn(999999999) + 999999
		addrs = append(addrs, util.V6Address(uint64(r), uint64(r)))
	}
	return addrs
}
