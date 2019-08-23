package util

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

var testSourceFilters = map[string][]string{
	"172.0.0.1":     {"80", "10", "443"},
	"172.0.1.2":     {"*"},
	"*":             {"9000"},
	"::7f00:35:0:0": {"443"},
	"10.0.0.10":     {"3333", "bad-port@#!"},
}

var testDestinationFilters = map[string][]string{
	"10.0.0.0/24":      {"8080", "8081", "10255"},
	"169.254.170.2":    {"5005"},
	"":                 {"1234"},
	"2001:db8::2:1":    {"5001"},
	"2001:db8::2:1/55": {"80"},
	"*":                {"9000"},
}

func TestParseConnectionFilters(t *testing.T) {
	sourceList := ParseConnectionFilters(testSourceFilters)
	destList := ParseConnectionFilters(testDestinationFilters)

	// source
	assert.True(t, IsBlacklistedConnection(sourceList, AddressFromString("172.0.0.1"), uint16(10)))
	assert.False(t, IsBlacklistedConnection(sourceList, AddressFromString("172.0.0.2"), uint16(10)))
	assert.True(t, IsBlacklistedConnection(sourceList, AddressFromString("172.0.1.2"), uint16(10001)))
	assert.True(t, IsBlacklistedConnection(sourceList, AddressFromString("*"), uint16(9000)))
	assert.True(t, IsBlacklistedConnection(sourceList, AddressFromString("::7f00:35:0:0"), uint16(443))) // ipv6
	assert.True(t, IsBlacklistedConnection(sourceList, AddressFromString("0"), uint16(443)))             // == ::7f00:35:0:0
	assert.False(t, IsBlacklistedConnection(sourceList, AddressFromString("10.0.0.10"), uint16(3333)))   // bad port

	// destination
	assert.True(t, IsBlacklistedConnection(destList, AddressFromString("10.0.0.5"), uint16(8080)))
	assert.True(t, IsBlacklistedConnection(destList, AddressFromString("169.254.170.2"), uint16(5005)))
	assert.False(t, IsBlacklistedConnection(destList, AddressFromString(""), uint16(1234)))
	assert.True(t, IsBlacklistedConnection(destList, AddressFromString("2001:db8::2:1"), uint16(5001)))
	assert.True(t, IsBlacklistedConnection(destList, AddressFromString("2001:db8::5:1"), uint16(80)))
	assert.True(t, IsBlacklistedConnection(destList, AddressFromString("123.4.5.6"), uint16(9000)))
}

var sink bool

func BenchmarkIsBlacklistedConnection(b *testing.B) {
	sourceList := ParseConnectionFilters(testSourceFilters)
	destList := ParseConnectionFilters(testDestinationFilters)
	addrs := randIPv4(6)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, addr := range addrs {
			sink = IsBlacklistedConnection(sourceList, addr, uint16(rand.Intn(9999)))
			sink = IsBlacklistedConnection(destList, addr, uint16(rand.Intn(9999)))
		}
	}
}

func randIPv4(count int) (addrs []Address) {
	for i := 0; i < count; i++ {
		r := rand.Intn(999999999) + 999999
		addrs = append(addrs, V4Address(uint32(r)))
	}
	return addrs
}

func randIPv6(count int) (addrs []Address) {
	for i := 0; i < count; i++ {
		r := rand.Intn(999999999) + 999999
		addrs = append(addrs, V6Address(uint64(r), uint64(r)))
	}
	return addrs
}
