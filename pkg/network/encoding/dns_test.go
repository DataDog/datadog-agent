// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"syscall"
	"testing"

	"github.com/DataDog/agent-payload/v5/process"
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestFormatConnectionDNS(t *testing.T) {
	payload := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{
					Source:    util.AddressFromString("10.1.1.1"),
					Dest:      util.AddressFromString("8.8.8.8"),
					SPort:     1000,
					DPort:     53,
					Type:      network.UDP,
					Family:    network.AFINET6,
					Direction: network.LOCAL,
				},
			},
		},
		DNSStats: dns.StatsByKeyByNameByType{
			dns.Key{
				ClientIP:   util.AddressFromString("10.1.1.1"),
				ServerIP:   util.AddressFromString("8.8.8.8"),
				ClientPort: uint16(1000),
				Protocol:   syscall.IPPROTO_UDP,
			}: map[dns.Hostname]map[dns.QueryType]dns.Stats{
				dns.ToHostname("foo.com"): {
					dns.TypeA: {
						Timeouts:          0,
						SuccessLatencySum: 0,
						FailureLatencySum: 0,
						CountByRcode:      map[uint32]uint32{0: 1},
					},
				},
			},
		},
	}

	t.Run("DNS with collect_domains_enabled=true,enable_dns_by_querytype=false", func(t *testing.T) {
		config.SystemProbe.Set("system_probe_config.collect_dns_domains", true)
		config.SystemProbe.Set("network_config.enable_dns_by_querytype", false)

		ipc := make(ipCache)
		formatter := newDNSFormatter(payload, ipc)
		in := payload.Conns[0]
		out := new(model.Connection)

		formatter.FormatConnectionDNS(in, out)
		expected := &model.Connection{
			DnsStatsByDomain: map[int32]*process.DNSStats{
				0: {
					DnsTimeouts:          0,
					DnsSuccessLatencySum: 0,
					DnsFailureLatencySum: 0,
					DnsCountByRcode: map[uint32]uint32{
						0: 1,
					},
				},
			},
			DnsStatsByDomainByQueryType:       nil,
			DnsStatsByDomainOffsetByQueryType: nil,
		}

		assert.Equal(t, expected, out)
	})

	t.Run("DNS with collect_domains_enabled=true,enable_dns_by_querytype=true", func(t *testing.T) {
		config.SystemProbe.Set("system_probe_config.collect_dns_domains", true)
		config.SystemProbe.Set("network_config.enable_dns_by_querytype", true)

		ipc := make(ipCache)
		formatter := newDNSFormatter(payload, ipc)
		in := payload.Conns[0]
		out := new(model.Connection)

		formatter.FormatConnectionDNS(in, out)
		expected := &model.Connection{
			DnsStatsByDomain: nil,
			DnsStatsByDomainByQueryType: map[int32]*model.DNSStatsByQueryType{
				0: {
					DnsStatsByQueryType: map[int32]*model.DNSStats{
						int32(dns.TypeA): {
							DnsTimeouts:          0,
							DnsSuccessLatencySum: 0,
							DnsFailureLatencySum: 0,
							DnsCountByRcode:      map[uint32]uint32{0: 1},
						},
					},
				},
			},
			DnsStatsByDomainOffsetByQueryType: nil,
		}

		assert.Equal(t, expected, out)
	})
}

func TestDNSPIDCollision(t *testing.T) {
	payload := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{
					Source:    util.AddressFromString("10.1.1.1"),
					Dest:      util.AddressFromString("8.8.8.8"),
					Pid:       1,
					SPort:     1000,
					DPort:     53,
					Type:      network.UDP,
					Family:    network.AFINET6,
					Direction: network.LOCAL,
				},
				{
					Source:    util.AddressFromString("10.1.1.1"),
					Dest:      util.AddressFromString("8.8.8.8"),
					Pid:       2,
					SPort:     1000,
					DPort:     53,
					Type:      network.UDP,
					Family:    network.AFINET6,
					Direction: network.LOCAL,
				},
			},
		},
		DNSStats: dns.StatsByKeyByNameByType{
			dns.Key{
				ClientIP:   util.AddressFromString("10.1.1.1"),
				ServerIP:   util.AddressFromString("8.8.8.8"),
				ClientPort: uint16(1000),
				Protocol:   syscall.IPPROTO_UDP,
			}: map[dns.Hostname]map[dns.QueryType]dns.Stats{
				dns.ToHostname("foo.com"): {
					dns.TypeA: {
						Timeouts:          0,
						SuccessLatencySum: 0,
						FailureLatencySum: 0,
						CountByRcode:      map[uint32]uint32{0: 1},
					},
				},
			},
		},
	}

	config.SystemProbe.Set("system_probe_config.collect_dns_domains", true)
	config.SystemProbe.Set("network_config.enable_dns_by_querytype", false)

	ipc := make(ipCache)
	formatter := newDNSFormatter(payload, ipc)
	out1 := new(model.Connection)
	out2 := new(model.Connection)
	formatter.FormatConnectionDNS(payload.Conns[0], out1)
	formatter.FormatConnectionDNS(payload.Conns[1], out2)

	// Only the first connection should be bound to DNS stats in the context of a PID collision
	assert.NotNil(t, out1.DnsStatsByDomain)
	assert.Nil(t, out2.DnsStatsByDomain)
}
