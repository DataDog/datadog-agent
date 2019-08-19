package util

import (
	"net"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

var filters = map[string][]string{
	"172.0.0.1": []string{
		"80",
		"10",
		"443",
	},
	"172.0.1.2": []string{},
	"10.0.0.0/24": []string{
		"8080",
		"8081",
		"10255",
	},
	"*": []string{
		"9000",
	},
	"169.254.170.2": []string{
		"*",
	},
}

func TestParseBlacklist(t *testing.T) {
	config.Datadog.SetDefault("system_probe_config.excluded_source_connections", filters)
	parseBlacklist(config.Datadog.GetStringMapStringSlice("system_probe_config.excluded_source_connections"))

	assert.True(t, IsBlacklistedConnection("source", AddressFromString("172.0.0.1"), uint16(10)))
	assert.True(t, IsBlacklistedConnection("source", AddressFromString("172.0.1.2"), uint16(8080)))
	assert.True(t, IsBlacklistedConnection("source", AddressFromString("10.0.0.5"), uint16(8080)))
	assert.True(t, IsBlacklistedConnection("source", AddressFromString("169.254.170.2"), uint16(5005)))
	assert.True(t, IsBlacklistedConnection("source", AddressFromString("125.0.0.3"), uint16(9000)))

	assert.False(t, IsBlacklistedConnection("source", AddressFromString("10.0.0.256"), uint16(8081)))
	assert.False(t, IsBlacklistedConnection("source", AddressFromString("127.0.0.1"), uint16(443)))
	assert.False(t, IsBlacklistedConnection("invalid", AddressFromString("localhost"), uint16(3333)))
}

func TestNewNetworkFilter(t *testing.T) {
	_, subnet, _ := net.ParseCIDR("10.0.0.0/24")
	connections := []*Connection{
		{
			IP:    "172.0.0.1",
			CIDR:  nil,
			Ports: []string{"80", "10", "443"},
		}, {
			IP:    "172.0.1.2",
			CIDR:  nil,
			Ports: []string{}, // what if nil?
		}, {
			IP:    "10.0.0.0",
			CIDR:  subnet,
			Ports: []string{"8080", "8081", "10255"},
		}, {
			IP:    "",
			CIDR:  nil,
			Ports: []string{"9000"},
		}, {
			IP:    "169.254.170.2",
			CIDR:  nil,
			Ports: []string{"*"},
		},
	}

	config.Datadog.SetDefault("system_probe_config.excluded_source_connections", map[string][]string{})
	s := newNetworkFilter("source")
	assert.Empty(t, s)

	s = newNetworkFilter("invalid")
	assert.Empty(t, s)

	config.Datadog.SetDefault("system_probe_config.excluded_source_connections", filters)
	s = newNetworkFilter("source")
	assert.Equal(t, len(s), len(connections))

	config.Datadog.SetDefault("system_probe_config.excluded_destination_connections", filters)
	d := newNetworkFilter("destination")
	assert.Equal(t, len(d), len(connections))
}
