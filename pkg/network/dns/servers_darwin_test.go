// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package dns

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scutilDNSOutput is the output of `scutil --dns` on macOS 26.6.2 with
// /etc/resolver/example.org naming 208.67.220.220 on port 443, plus two made
// up resolvers.
const scutilDNSOutput = `DNS configuration

resolver #1
  nameserver[0] : 192.168.64.1
  if_index : 5 (en0)
  flags    : Request A records
  reach    : 0x00020002 (Reachable,Directly Reachable Address)

resolver #2
  domain   : local
  options  : mdns
  timeout  : 5
  flags    : Request A records
  reach    : 0x00000000 (Not Reachable)
  order    : 300000

resolver #3
  domain   : standard.example
  nameserver[0] : 192.0.2.2
  port     : 53

resolver #4
  domain   : corp.example
  port     : 5300
  nameserver[0] : 2001:db8::53
  nameserver[1] : fe80::1%en0
  nameserver[2] : 192.0.2.1
  flags    : Request A records, Request AAAA records
  reach    : 0x00000002 (Reachable)

resolver #5
  domain   : example.org
  nameserver[0] : 208.67.220.220
  port     : 443
  flags    : Request A records
  reach    : 0x00000002 (Reachable)

DNS configuration (for scoped queries)

resolver #1
  nameserver[0] : 192.168.64.1
  if_index : 5 (en0)
  flags    : Scoped, Request A records
  reach    : 0x00020002 (Reachable,Directly Reachable Address)

resolver #2
  domain   : example.org
  nameserver[0] : 208.67.220.220
  port     : 443
  if_index : 5 (en0)
  flags    : Scoped, Request A records
  reach    : 0x00000002 (Reachable)
`

func TestParseDNSServers(t *testing.T) {
	assert.Equal(t, []netip.AddrPort{
		netip.MustParseAddrPort("192.0.2.1:5300"),
		netip.MustParseAddrPort("208.67.220.220:443"),
		netip.MustParseAddrPort("[2001:db8::53]:5300"),
		netip.MustParseAddrPort("[fe80::1]:5300"),
	}, parseDNSServers(scutilDNSOutput))
	assert.Empty(t, parseDNSServers("DNS configuration\n\nresolver #1\n  nameserver[0] : 1.1.1.1\n"))
	assert.Empty(t, parseDNSServers("No DNS configuration available\n"))
}

func TestListDNSServers(t *testing.T) {
	_, err := listDNSServers()
	require.NoError(t, err)
}
