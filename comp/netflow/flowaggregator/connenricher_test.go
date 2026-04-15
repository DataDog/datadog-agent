// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package flowaggregator

import (
	"net"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

func TestNormalizeIP(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected [16]byte
	}{
		{
			name:  "IPv4",
			input: "192.168.1.1",
			expected: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff,
				192, 168, 1, 1,
			},
		},
		{
			name:  "IPv6",
			input: "::1",
			expected: [16]byte{
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1,
			},
		},
		{
			name:     "invalid",
			input:    "not-an-ip",
			expected: [16]byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeIP(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeIPBytes(t *testing.T) {
	// IPv4 bytes
	ipv4 := net.ParseIP("10.0.0.1").To4()
	result := normalizeIPBytes(ipv4)
	expected := [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 10, 0, 0, 1}
	assert.Equal(t, expected, result)

	// IPv6 bytes
	ipv6 := net.ParseIP("::1").To16()
	result = normalizeIPBytes(ipv6)
	expected = [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	assert.Equal(t, expected, result)
}

func TestConnectionTypeToProtocol(t *testing.T) {
	assert.Equal(t, uint8(6), connectionTypeToProtocol(model.ConnectionType_tcp))
	assert.Equal(t, uint8(17), connectionTypeToProtocol(model.ConnectionType_udp))
	assert.Equal(t, uint8(0), connectionTypeToProtocol(model.ConnectionType(99)))
}

func TestDirectionString(t *testing.T) {
	assert.Equal(t, "incoming", directionString(model.ConnectionDirection_incoming))
	assert.Equal(t, "outgoing", directionString(model.ConnectionDirection_outgoing))
	assert.Equal(t, "local", directionString(model.ConnectionDirection_local))
	assert.Equal(t, "unspecified", directionString(model.ConnectionDirection_unspecified))
}

func TestConnEnricherBuildIndexAndLookup(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, "abc123"),
		"test",
		[]string{"service:trainer", "env:testbed"},
		nil, nil, nil,
	)
	fakeTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, "def456"),
		"test",
		[]string{"service:pokedex", "env:testbed"},
		nil, nil, nil,
	)

	enricher := &ConnEnricher{
		exactIndex: make(map[connKey]*connMeta),
		noSrcPort:  make(map[connKey]*connMeta),
		noDstPort:  make(map[connKey]*connMeta),
		tagger:     fakeTagger,
		logger:     logmock.New(t),
	}

	conns := &model.Connections{
		Conns: []*model.Connection{
			{
				Pid: 1234,
				Laddr: &model.Addr{
					Ip:          "172.20.0.10",
					Port:        45678,
					ContainerId: "abc123",
				},
				Raddr: &model.Addr{
					Ip:          "172.21.0.10",
					Port:        8080,
					ContainerId: "def456",
				},
				Type:      model.ConnectionType_tcp,
				Direction: model.ConnectionDirection_outgoing,
			},
		},
	}

	enricher.buildIndex(conns)

	assert.Len(t, enricher.exactIndex, 1)
	assert.Len(t, enricher.noSrcPort, 1)
	assert.Len(t, enricher.noDstPort, 1)

	// Exact lookup
	srcIP := net.ParseIP("172.20.0.10").To4()
	dstIP := net.ParseIP("172.21.0.10").To4()
	meta := enricher.Lookup(srcIP, dstIP, 45678, 8080, 6)
	assert.NotNil(t, meta)
	assert.Equal(t, "abc123", meta.SrcContainerID)
	assert.Equal(t, "def456", meta.DstContainerID)
	assert.Equal(t, "trainer", meta.SrcService)
	assert.Equal(t, "pokedex", meta.DstService)
	assert.Equal(t, "outgoing", meta.Direction)
	assert.Contains(t, meta.SrcTags, "service:trainer")
	assert.Contains(t, meta.DstTags, "service:pokedex")

	// Ephemeral source port lookup
	meta = enricher.Lookup(srcIP, dstIP, -1, 8080, 6)
	assert.NotNil(t, meta)
	assert.Equal(t, "abc123", meta.SrcContainerID)

	// Ephemeral dest port lookup
	meta = enricher.Lookup(srcIP, dstIP, 45678, -1, 6)
	assert.NotNil(t, meta)
	assert.Equal(t, "def456", meta.DstContainerID)

	// Lookup miss
	otherIP := net.ParseIP("10.0.0.1").To4()
	meta = enricher.Lookup(otherIP, dstIP, 45678, 8080, 6)
	assert.Nil(t, meta)
}

func TestConnEnricherSkipsEmptyContainers(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	enricher := &ConnEnricher{
		exactIndex: make(map[connKey]*connMeta),
		noSrcPort:  make(map[connKey]*connMeta),
		noDstPort:  make(map[connKey]*connMeta),
		tagger:     fakeTagger,
		logger:     logmock.New(t),
	}

	conns := &model.Connections{
		Conns: []*model.Connection{
			{
				Laddr: &model.Addr{Ip: "1.2.3.4", Port: 1111},
				Raddr: &model.Addr{Ip: "5.6.7.8", Port: 2222},
				Type:  model.ConnectionType_tcp,
			},
		},
	}

	enricher.buildIndex(conns)
	assert.Len(t, enricher.exactIndex, 0)
}

func TestAddConnEnrichment(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, "src-container"),
		"test",
		[]string{"service:web", "env:prod"},
		nil, nil, nil,
	)

	enricher := &ConnEnricher{
		exactIndex: make(map[connKey]*connMeta),
		noSrcPort:  make(map[connKey]*connMeta),
		noDstPort:  make(map[connKey]*connMeta),
		tagger:     fakeTagger,
		logger:     logmock.New(t),
	}

	conns := &model.Connections{
		Conns: []*model.Connection{
			{
				Laddr: &model.Addr{
					Ip:          "172.20.0.10",
					Port:        45678,
					ContainerId: "src-container",
				},
				Raddr: &model.Addr{
					Ip:          "172.21.0.10",
					Port:        8080,
					ContainerId: "dst-container",
				},
				Type:      model.ConnectionType_tcp,
				Direction: model.ConnectionDirection_outgoing,
			},
		},
	}
	enricher.buildIndex(conns)

	flow := &common.Flow{
		SrcAddr:    net.ParseIP("172.20.0.10").To4(),
		DstAddr:    net.ParseIP("172.21.0.10").To4(),
		SrcPort:    45678,
		DstPort:    8080,
		IPProtocol: 6,
	}

	addConnEnrichment(flow, enricher)

	assert.NotNil(t, flow.AdditionalFields)
	assert.Equal(t, true, flow.AdditionalFields["ebpf.matched"])
	assert.Equal(t, "src-container", flow.AdditionalFields["ebpf.src.container_id"])
	assert.Equal(t, "dst-container", flow.AdditionalFields["ebpf.dst.container_id"])
	assert.Equal(t, "web", flow.AdditionalFields["ebpf.src.service"])
	assert.Equal(t, "outgoing", flow.AdditionalFields["ebpf.direction"])
}

func TestAddConnEnrichmentNilEnricher(t *testing.T) {
	flow := &common.Flow{
		SrcAddr:    net.ParseIP("172.20.0.10").To4(),
		DstAddr:    net.ParseIP("172.21.0.10").To4(),
		SrcPort:    45678,
		DstPort:    8080,
		IPProtocol: 6,
	}

	addConnEnrichment(flow, nil)
	assert.Nil(t, flow.AdditionalFields)
}

func TestAddConnEnrichmentNoMatch(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	enricher := &ConnEnricher{
		exactIndex: make(map[connKey]*connMeta),
		noSrcPort:  make(map[connKey]*connMeta),
		noDstPort:  make(map[connKey]*connMeta),
		tagger:     fakeTagger,
		logger:     logmock.New(t),
	}

	flow := &common.Flow{
		SrcAddr:    net.ParseIP("172.20.0.10").To4(),
		DstAddr:    net.ParseIP("172.21.0.10").To4(),
		SrcPort:    45678,
		DstPort:    8080,
		IPProtocol: 6,
	}

	addConnEnrichment(flow, enricher)
	assert.Nil(t, flow.AdditionalFields)
}
