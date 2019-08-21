package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var testSourceFilters = map[string][]string{
	"172.0.0.1":     {"80", "10", "443"},
	"172.0.1.2":     {},
	"*":             {"9000"},
	"::7f00:35:0:0": {"443"},
}

var testDestinationFilters = map[string][]string{
	"10.0.0.0/24":      {"8080", "8081", "10255"},
	"169.254.170.2":    {"5005"},
	"":                 {"1234"},
	"2001:db8::2:1":    {"5001"},
	"2001:db8::2:1/55": {"80"},
}

func TestParseBlacklist(t *testing.T) {
	sourceList := ParseBlacklist(testSourceFilters)
	destList := ParseBlacklist(testDestinationFilters)

	// source
	assert.True(t, IsBlacklistedConnection(sourceList, AddressFromString("172.0.0.1"), uint16(10)))
	assert.False(t, IsBlacklistedConnection(sourceList, AddressFromString("172.0.0.2"), uint16(10)))
	assert.False(t, IsBlacklistedConnection(sourceList, AddressFromString("*"), uint16(9000)))
	assert.True(t, IsBlacklistedConnection(sourceList, AddressFromString("::7f00:35:0:0"), uint16(443))) // ipv6
	assert.True(t, IsBlacklistedConnection(sourceList, AddressFromString("0"), uint16(443)))             // == ::7f00:35:0:0

	// destination
	assert.True(t, IsBlacklistedConnection(destList, AddressFromString("10.0.0.5"), uint16(8080)))
	assert.True(t, IsBlacklistedConnection(destList, AddressFromString("169.254.170.2"), uint16(5005)))
	assert.False(t, IsBlacklistedConnection(destList, AddressFromString(""), uint16(1234)))
	assert.True(t, IsBlacklistedConnection(destList, AddressFromString("2001:db8::2:1"), uint16(5001)))
	assert.True(t, IsBlacklistedConnection(destList, AddressFromString("2001:db8::5:1"), uint16(80)))
}
