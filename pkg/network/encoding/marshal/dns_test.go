// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

import (
	"bytes"
	"io"
	"syscall"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/process/util"

	model "github.com/DataDog/agent-payload/v5/process"
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
		config.SystemProbe.SetWithoutSource("system_probe_config.collect_dns_domains", true)
		config.SystemProbe.SetWithoutSource("network_config.enable_dns_by_querytype", false)

		ipc := make(ipCache)
		formatter := newDNSFormatter(payload, ipc)
		in := payload.Conns[0]

		streamer := NewProtoTestStreamer[*model.Connection]()
		builder := model.NewConnectionBuilder(streamer)

		formatter.FormatConnectionDNS(in, builder)
		expected := &model.Connection{
			DnsStatsByDomain: map[int32]*model.DNSStats{
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

		assert.Equal(t, expected, streamer.Unwrap(t, &model.Connection{}))
	})

	t.Run("DNS with collect_domains_enabled=true,enable_dns_by_querytype=true", func(t *testing.T) {
		config.SystemProbe.SetWithoutSource("system_probe_config.collect_dns_domains", true)
		config.SystemProbe.SetWithoutSource("network_config.enable_dns_by_querytype", true)

		ipc := make(ipCache)
		formatter := newDNSFormatter(payload, ipc)
		in := payload.Conns[0]

		streamer := NewProtoTestStreamer[*model.Connection]()
		builder := model.NewConnectionBuilder(streamer)

		formatter.FormatConnectionDNS(in, builder)
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

		assert.Equal(t, expected, streamer.Unwrap(t, &model.Connection{}))
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

	config.SystemProbe.SetWithoutSource("system_probe_config.collect_dns_domains", true)
	config.SystemProbe.SetWithoutSource("network_config.enable_dns_by_querytype", false)

	ipc := make(ipCache)
	formatter := newDNSFormatter(payload, ipc)

	streamer := NewProtoTestStreamer[*model.Connection]()
	formatter.FormatConnectionDNS(payload.Conns[0], model.NewConnectionBuilder(streamer))
	out1 := streamer.Unwrap(t, &model.Connection{})

	streamer.Reset()
	formatter.FormatConnectionDNS(payload.Conns[1], model.NewConnectionBuilder(streamer))
	out2 := streamer.Unwrap(t, &model.Connection{})

	// Only the first connection should be bound to DNS stats in the context of a PID collision
	assert.NotNil(t, out1.DnsStatsByDomain)
	assert.Nil(t, out2.DnsStatsByDomain)
}

var _ io.Writer = &ProtoTestStreamer[*model.Connection]{}

type ProtoTestStreamer[T proto.Message] struct {
	buf *bytes.Buffer
}

func NewProtoTestStreamer[T proto.Message]() *ProtoTestStreamer[T] {
	return &ProtoTestStreamer[T]{
		buf: bytes.NewBuffer(nil),
	}
}

func (p *ProtoTestStreamer[T]) Write(b []byte) (n int, err error) {
	return p.buf.Write(b)
}

// TODO: real generics
func (p *ProtoTestStreamer[T]) Unwrap(t *testing.T, v T) T {
	err := proto.Unmarshal(p.buf.Bytes(), v)
	require.NoError(t, err)
	return v
}

func (p *ProtoTestStreamer[T]) Reset() {
	p.buf.Reset()
}
