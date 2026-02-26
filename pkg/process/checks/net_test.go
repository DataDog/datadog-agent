// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	model "github.com/DataDog/agent-payload/v5/process"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/hosttags"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func makeConnection(pid int32) *model.Connection {
	return &model.Connection{
		Pid:   pid,
		Laddr: &model.Addr{},
		Raddr: &model.Addr{},
	}
}

func makeConnections(n int) []*model.Connection {
	conns := make([]*model.Connection, 0)
	for i := 1; i <= n; i++ {
		c := makeConnection(int32(i))
		c.Laddr = &model.Addr{ContainerId: strconv.Itoa(int(c.Pid))}
		c.RouteIdx = int32(-1)

		conns = append(conns, c)
	}
	return conns
}

func TestDNSNameEncoding(t *testing.T) {
	p := makeConnections(5)
	p[0].Raddr.Ip = "1.1.2.1"
	p[1].Raddr.Ip = "1.1.2.2"
	p[2].Raddr.Ip = "1.1.2.3"
	p[3].Raddr.Ip = "1.1.2.4"
	p[4].Raddr.Ip = "1.1.2.5"

	dns := map[string]*model.DNSEntry{
		"1.1.2.1": {Names: []string{"host1.domain.com"}},
		"1.1.2.2": {Names: []string{"host2.domain.com", "host2.domain2.com"}},
		"1.1.2.3": {Names: []string{"host3.domain.com", "host3.domain2.com", "host3.domain3.com"}},
		"1.1.2.4": {Names: []string{"host4.domain.com"}},
		"1.1.2.5": {Names: nil},
	}
	serviceExtractorEnabled := false
	useWindowsServiceName := false
	useImprovedAlgorithm := false
	ex := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
	maxConnsPerMessage := 10
	hostTagsProvider := hosttags.NewHostTagProvider()
	chunks := batchConnections(&HostInfo{}, hostTagsProvider, nil, nil, maxConnsPerMessage, 0, p, dns, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, nil, nil, nil, ex, nil, nil)
	assert.Equal(t, len(chunks), 1)

	chunk := chunks[0]
	conns := chunk.(*model.CollectorConnections)
	dnsParsed := make(map[string]*model.DNSEntry)
	for _, conn := range p {
		ip := conn.Raddr.Ip
		dnsParsed[ip] = &model.DNSEntry{}
		model.IterateDNSV2(conns.EncodedDnsLookups, ip,
			func(_, total int, entry int32) bool {
				host, e := conns.GetDNSNameByOffset(entry)
				assert.Nil(t, e)
				assert.Equal(t, total, len(dns[ip].Names))
				dnsParsed[ip].Names = append(dnsParsed[ip].Names, host)
				return true
			})
	}
	assert.Equal(t, dns, dnsParsed)

}

func TestNetworkConnectionBatching(t *testing.T) {
	for i, tc := range []struct {
		cur, last      []*model.Connection
		maxSize        int
		expectedTotal  int
		expectedChunks int
	}{
		{
			cur:            makeConnections(3),
			maxSize:        1,
			expectedTotal:  3,
			expectedChunks: 3,
		},
		{
			cur:            makeConnections(3),
			maxSize:        2,
			expectedTotal:  3,
			expectedChunks: 2,
		},
		{
			cur:            makeConnections(4),
			maxSize:        10,
			expectedTotal:  4,
			expectedChunks: 1,
		},
		{
			cur:            makeConnections(4),
			maxSize:        3,
			expectedTotal:  4,
			expectedChunks: 2,
		},
		{
			cur:            makeConnections(6),
			maxSize:        2,
			expectedTotal:  6,
			expectedChunks: 3,
		},
	} {
		ctm := map[string]int64{}
		rctm := map[string]*model.RuntimeCompilationTelemetry{}
		khfr := model.KernelHeaderFetchResult_FetchNotAttempted
		coretm := map[string]model.COREResult{}
		serviceExtractorEnabled := false
		useWindowsServiceName := false
		useImprovedAlgorithm := false
		ex := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
		hostTagsProvider := hosttags.NewHostTagProvider()
		chunks := batchConnections(&HostInfo{}, hostTagsProvider, nil, nil, tc.maxSize, 0, tc.cur, map[string]*model.DNSEntry{}, "nid", ctm, rctm, khfr, coretm, nil, nil, nil, nil, nil, ex, nil, nil)

		assert.Len(t, chunks, tc.expectedChunks, "len %d", i)
		total := 0
		for i, c := range chunks {
			connections := c.(*model.CollectorConnections)
			total += len(connections.Connections)
			assert.Equal(t, int32(tc.expectedChunks), connections.GroupSize, "group size test %d", i)

			// make sure we could get container and pid mapping for connections
			assert.Equal(t, len(connections.Connections), len(connections.ContainerForPid))
			assert.Equal(t, "nid", connections.NetworkId)
			for _, conn := range connections.Connections {
				assert.Contains(t, connections.ContainerForPid, conn.Pid)
				assert.Equal(t, strconv.Itoa(int(conn.Pid)), connections.ContainerForPid[conn.Pid])
			}

			// ensure only first chunk has telemetry
			if i == 0 {
				assert.NotNil(t, connections.ConnTelemetryMap)
				assert.NotNil(t, connections.CompilationTelemetryByAsset)
			} else {
				assert.Nil(t, connections.ConnTelemetryMap)
				assert.Nil(t, connections.CompilationTelemetryByAsset)
			}
		}
		assert.Equal(t, tc.expectedTotal, total, "total test %d", i)
	}
}

func TestNetworkConnectionBatchingWithDNS(t *testing.T) {
	p := makeConnections(4)

	p[3].Raddr.Ip = "1.1.2.3"
	dns := map[string]*model.DNSEntry{
		"1.1.2.3": {Names: []string{"datacat.edu"}},
	}

	maxConnsPerMessage := 1
	serviceExtractorEnabled := false
	useWindowsServiceName := false
	useImprovedAlgorithm := false
	ex := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
	hostTagsProvider := hosttags.NewHostTagProvider()
	chunks := batchConnections(&HostInfo{}, hostTagsProvider, nil, nil, maxConnsPerMessage, 0, p, dns, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, nil, nil, nil, ex, nil, nil)

	assert.Len(t, chunks, 4)
	total := 0
	for i, c := range chunks {
		connections := c.(*model.CollectorConnections)

		// Only the last chunk should have a DNS mapping
		if i == 3 {
			assert.NotEmpty(t, connections.EncodedDnsLookups)
		} else {
			assert.Empty(t, connections.EncodedDnsLookups)
		}

		total += len(connections.Connections)
		assert.Equal(t, int32(4), connections.GroupSize)

		// make sure we could get container and pid mapping for connections
		assert.Equal(t, len(connections.Connections), len(connections.ContainerForPid))
		assert.Equal(t, "nid", connections.NetworkId)
		for _, conn := range connections.Connections {
			assert.Contains(t, connections.ContainerForPid, conn.Pid)
			assert.Equal(t, strconv.Itoa(int(conn.Pid)), connections.ContainerForPid[conn.Pid])
		}
	}
	assert.Equal(t, 4, total)
}

func TestBatchSimilarConnectionsTogether(t *testing.T) {
	p := makeConnections(6)

	p[0].Raddr.Ip = "1.1.2.3"
	p[1].Raddr.Ip = "1.2.3.4"
	p[2].Raddr.Ip = "1.3.4.5"
	p[3].Raddr.Ip = "1.1.2.3"
	p[4].Raddr.Ip = "1.2.3.4"
	p[5].Raddr.Ip = "1.3.4.5"

	maxConnsPerMessage := 2
	serviceExtractorEnabled := false
	useWindowsServiceName := false
	useImprovedAlgorithm := false
	ex := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
	hostTagsProvider := hosttags.NewHostTagProvider()
	chunks := batchConnections(&HostInfo{}, hostTagsProvider, nil, nil, maxConnsPerMessage, 0, p, map[string]*model.DNSEntry{}, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, nil, nil, nil, ex, nil, nil)

	assert.Len(t, chunks, 3)
	total := 0
	for _, c := range chunks {
		connections := c.(*model.CollectorConnections)
		total += len(connections.Connections)
		assert.Equal(t, int32(3), connections.GroupSize)
		assert.Equal(t, 2, len(connections.Connections))

		// make sure the connections with similar remote addresses were grouped together
		rAddr := connections.Connections[0].Raddr.Ip
		for _, cc := range connections.Connections {
			assert.Equal(t, rAddr, cc.Raddr.Ip)
		}

		// make sure the connections with the same remote address are ordered by PID
		lastSeenPID := connections.Connections[0].Pid
		for _, cc := range connections.Connections {
			assert.LessOrEqual(t, lastSeenPID, cc.Pid)
			lastSeenPID = cc.Pid
		}
	}
	assert.Equal(t, 6, total)
}

func indexOf(s string, db []string) int32 {
	for idx, val := range db {
		if val == s {
			return int32(idx)
		}
	}
	return -1
}

func TestNetworkConnectionBatchingWithDomainsByQueryType(t *testing.T) {
	conns := makeConnections(4)

	domains := []string{"foo.com", "bar.com", "baz.com"}
	conns[1].DnsStatsByDomainByQueryType = map[int32]*model.DNSStatsByQueryType{
		0: {
			DnsStatsByQueryType: map[int32]*model.DNSStats{
				int32(dns.TypeA): {
					DnsTimeouts: 1,
				},
			},
		},
	}
	conns[2].DnsStatsByDomainByQueryType = map[int32]*model.DNSStatsByQueryType{
		0: {
			DnsStatsByQueryType: map[int32]*model.DNSStats{
				int32(dns.TypeA): {
					DnsTimeouts: 2,
				},
			},
		},
		2: {
			DnsStatsByQueryType: map[int32]*model.DNSStats{
				int32(dns.TypeA): {
					DnsTimeouts: 3,
				},
			},
		},
	}
	conns[3].DnsStatsByDomainByQueryType = map[int32]*model.DNSStatsByQueryType{
		1: {
			DnsStatsByQueryType: map[int32]*model.DNSStats{
				int32(dns.TypeA): {
					DnsTimeouts: 4,
				},
			},
		},
		2: {
			DnsStatsByQueryType: map[int32]*model.DNSStats{
				int32(dns.TypeA): {
					DnsTimeouts: 5,
				},
			},
		},
	}
	dnsmap := map[string]*model.DNSEntry{}

	maxConnsPerMessage := 1
	serviceExtractorEnabled := false
	useWindowsServiceName := false
	useImprovedAlgorithm := false
	ex := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
	hostTagsProvider := hosttags.NewHostTagProvider()
	chunks := batchConnections(&HostInfo{}, hostTagsProvider, nil, nil, maxConnsPerMessage, 0, conns, dnsmap, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, domains, nil, nil, nil, ex, nil, nil)

	assert.Len(t, chunks, 4)
	total := 0
	for i, c := range chunks {
		connections := c.(*model.CollectorConnections)
		total += len(connections.Connections)

		domaindb, _ := connections.GetDNSNames()

		// verify nothing was put in the DnsStatsByDomain bucket by mistake
		assert.Equal(t, len(connections.Connections[0].DnsStatsByDomain), 0)
		assert.Equal(t, len(connections.Connections[0].DnsStatsByDomainByQueryType), 0)

		switch i {
		case 0:
			assert.Equal(t, len(domaindb), 0)
		case 1:
			assert.Equal(t, len(domaindb), 1)
			assert.Equal(t, domains[0], domaindb[0])

			// check for correctness of the data
			conn := connections.Connections[0]
			//val, ok := conn.DnsStatsByDomainByQueryType[0]
			assert.Equal(t, 1, len(conn.DnsStatsByDomainOffsetByQueryType))
			// we don't know what hte offset will be, but since there's only one
			// the iteration should only happen once
			for off, val := range conn.DnsStatsByDomainOffsetByQueryType {
				// first, verify the hostname is what we expect
				domainstr, err := connections.GetDNSNameByOffset(off)
				assert.Nil(t, err)
				assert.Equal(t, domainstr, domains[0])
				assert.Equal(t, val.DnsStatsByQueryType[int32(dns.TypeA)].DnsTimeouts, uint32(1))
			}

		case 2:
			assert.Equal(t, len(domaindb), 2)
			assert.Contains(t, domaindb, domains[0])
			assert.Contains(t, domaindb, domains[2])
			assert.NotContains(t, domaindb, domains[1])

			conn := connections.Connections[0]
			for off, val := range conn.DnsStatsByDomainOffsetByQueryType {
				// first, verify the hostname is what we expect
				domainstr, err := connections.GetDNSNameByOffset(off)
				assert.Nil(t, err)

				idx := indexOf(domainstr, domains)
				assert.NotEqual(t, -1, idx)

				switch idx {
				case 0:
					assert.Equal(t, val.DnsStatsByQueryType[int32(dns.TypeA)].DnsTimeouts, uint32(2))
				case 2:
					assert.Equal(t, val.DnsStatsByQueryType[int32(dns.TypeA)].DnsTimeouts, uint32(3))
				default:
					assert.True(t, false, fmt.Sprintf("unexpected index %v", idx))
				}
			}

		case 3:
			assert.Equal(t, len(domaindb), 2)
			assert.Contains(t, domaindb, domains[1])
			assert.Contains(t, domaindb, domains[2])
			assert.NotContains(t, domaindb, domains[0])

			conn := connections.Connections[0]
			for off, val := range conn.DnsStatsByDomainOffsetByQueryType {
				// first, verify the hostname is what we expect
				domainstr, err := connections.GetDNSNameByOffset(off)
				assert.Nil(t, err)

				idx := indexOf(domainstr, domains)
				assert.NotEqual(t, -1, idx)

				switch idx {
				case 1:
					assert.Equal(t, val.DnsStatsByQueryType[int32(dns.TypeA)].DnsTimeouts, uint32(4))
				case 2:
					assert.Equal(t, val.DnsStatsByQueryType[int32(dns.TypeA)].DnsTimeouts, uint32(5))
				default:
					assert.True(t, false, fmt.Sprintf("unexpected index %v", idx))
				}
			}
		}
	}
	assert.Equal(t, 4, total)
}

func TestNetworkConnectionBatchingWithDomains(t *testing.T) {
	conns := makeConnections(4)

	domains := []string{"foo.com", "bar.com", "baz.com"}
	conns[1].DnsStatsByDomain = map[int32]*model.DNSStats{
		0: {
			DnsTimeouts: 1,
		},
	}
	conns[2].DnsStatsByDomain = map[int32]*model.DNSStats{
		0: {
			DnsTimeouts: 2,
		},
		2: {
			DnsTimeouts: 3,
		},
	}
	conns[3].DnsStatsByDomain = map[int32]*model.DNSStats{
		1: {
			DnsTimeouts: 4,
		},
		2: {
			DnsTimeouts: 5,
		},
	}
	dnsmap := map[string]*model.DNSEntry{}

	maxConnsPerMessage := 1
	serviceExtractorEnabled := false
	useWindowsServiceName := false
	useImprovedAlgorithm := false
	ex := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
	hostTagsProvider := hosttags.NewHostTagProvider()
	chunks := batchConnections(&HostInfo{}, hostTagsProvider, nil, nil, maxConnsPerMessage, 0, conns, dnsmap, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, domains, nil, nil, nil, ex, nil, nil)

	assert.Len(t, chunks, 4)
	total := 0
	for i, c := range chunks {
		connections := c.(*model.CollectorConnections)
		total += len(connections.Connections)

		domaindb, _ := connections.GetDNSNames()

		// verify nothing was put in the DnsStatsByDomain bucket by mistake
		assert.Equal(t, len(connections.Connections[0].DnsStatsByDomain), 0)
		// verify nothing was put in the DnsStatsByDomainByQueryType bucket by mistake
		assert.Equal(t, len(connections.Connections[0].DnsStatsByDomainByQueryType), 0)

		switch i {
		case 0:
			assert.Equal(t, len(domaindb), 0)
		case 1:
			assert.Equal(t, len(domaindb), 1)
			assert.Equal(t, domains[0], domaindb[0])

			// check for correctness of the data
			conn := connections.Connections[0]
			// we don't know what hte offset will be, but since there's only one
			// the iteration should only happen once
			for off, val := range conn.DnsStatsByDomainOffsetByQueryType {
				// first, verify the hostname is what we expect
				domainstr, err := connections.GetDNSNameByOffset(off)
				assert.Nil(t, err)
				assert.Equal(t, domainstr, domains[0])
				assert.Equal(t, val.DnsStatsByQueryType[int32(dns.TypeA)].DnsTimeouts, uint32(1))
			}
		case 2:
			assert.Equal(t, len(domaindb), 2)
			assert.Contains(t, domaindb, domains[0])
			assert.Contains(t, domaindb, domains[2])
			assert.NotContains(t, domaindb, domains[1])

			conn := connections.Connections[0]
			for off, val := range conn.DnsStatsByDomainOffsetByQueryType {
				// first, verify the hostname is what we expect
				domainstr, err := connections.GetDNSNameByOffset(off)
				assert.Nil(t, err)

				idx := indexOf(domainstr, domains)
				assert.NotEqual(t, -1, idx)

				switch idx {
				case 0:
					assert.Equal(t, val.DnsStatsByQueryType[int32(dns.TypeA)].DnsTimeouts, uint32(2))
				case 2:
					assert.Equal(t, val.DnsStatsByQueryType[int32(dns.TypeA)].DnsTimeouts, uint32(3))
				default:
					assert.True(t, false, fmt.Sprintf("unexpected index %v", idx))
				}
			}

		case 3:
			assert.Equal(t, len(domaindb), 2)
			assert.Contains(t, domaindb, domains[1])
			assert.Contains(t, domaindb, domains[2])
			assert.NotContains(t, domaindb, domains[0])

			conn := connections.Connections[0]
			for off, val := range conn.DnsStatsByDomainOffsetByQueryType {
				// first, verify the hostname is what we expect
				domainstr, err := connections.GetDNSNameByOffset(off)
				assert.Nil(t, err)

				idx := indexOf(domainstr, domains)
				assert.NotEqual(t, -1, idx)

				switch idx {
				case 1:
					assert.Equal(t, val.DnsStatsByQueryType[int32(dns.TypeA)].DnsTimeouts, uint32(4))
				case 2:
					assert.Equal(t, val.DnsStatsByQueryType[int32(dns.TypeA)].DnsTimeouts, uint32(5))
				default:
					assert.True(t, false, fmt.Sprintf("unexpected index %v", idx))
				}
			}
		}
	}
	assert.Equal(t, 4, total)
}

func TestNetworkConnectionBatchingWithRoutes(t *testing.T) {
	conns := makeConnections(8)

	routes := []*model.Route{
		{Subnet: &model.Subnet{Alias: "foo1"}},
		{Subnet: &model.Subnet{Alias: "foo2"}},
		{Subnet: &model.Subnet{Alias: "foo3"}},
		{Subnet: &model.Subnet{Alias: "foo4"}},
		{Subnet: &model.Subnet{Alias: "foo5"}},
	}

	conns[0].RouteIdx = 0
	conns[1].RouteIdx = 1
	conns[2].RouteIdx = 2
	conns[3].RouteIdx = 3
	conns[4].RouteIdx = -1
	conns[5].RouteIdx = 4
	conns[6].RouteIdx = 3
	conns[7].RouteIdx = 2

	maxConnsPerMessage := 4
	serviceExtractorEnabled := false
	useWindowsServiceName := false
	useImprovedAlgorithm := false
	ex := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
	hostTagsProvider := hosttags.NewHostTagProvider()
	chunks := batchConnections(&HostInfo{}, hostTagsProvider, nil, nil, maxConnsPerMessage, 0, conns, nil, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, routes, nil, nil, ex, nil, nil)

	assert.Len(t, chunks, 2)
	total := 0
	for i, c := range chunks {
		connections := c.(*model.CollectorConnections)
		total += len(connections.Connections)
		switch i {
		case 0:
			require.Equal(t, int32(0), connections.Connections[0].RouteIdx)
			require.Equal(t, int32(1), connections.Connections[1].RouteIdx)
			require.Equal(t, int32(2), connections.Connections[2].RouteIdx)
			require.Equal(t, int32(3), connections.Connections[3].RouteIdx)
			require.Len(t, connections.Routes, 4)
			require.Equal(t, routes[0].Subnet.Alias, connections.Routes[0].Subnet.Alias)
			require.Equal(t, routes[1].Subnet.Alias, connections.Routes[1].Subnet.Alias)
			require.Equal(t, routes[2].Subnet.Alias, connections.Routes[2].Subnet.Alias)
			require.Equal(t, routes[3].Subnet.Alias, connections.Routes[3].Subnet.Alias)
		case 1:
			require.Equal(t, int32(-1), connections.Connections[0].RouteIdx)
			require.Equal(t, int32(0), connections.Connections[1].RouteIdx)
			require.Equal(t, int32(1), connections.Connections[2].RouteIdx)
			require.Equal(t, int32(2), connections.Connections[3].RouteIdx)
			require.Len(t, connections.Routes, 3)
			require.Equal(t, routes[4].Subnet.Alias, connections.Routes[0].Subnet.Alias)
			require.Equal(t, routes[3].Subnet.Alias, connections.Routes[1].Subnet.Alias)
			require.Equal(t, routes[2].Subnet.Alias, connections.Routes[2].Subnet.Alias)
		}
	}
	assert.Equal(t, 8, total)
}

func TestNetworkConnectionTags(t *testing.T) {
	conns := makeConnections(8)

	tags := []string{
		"tag0",
		"tag1",
		"tag2",
		"tag3",
	}

	conns[0].Tags = []uint32{0}
	// conns[1] contains no tags
	conns[2].Tags = []uint32{0, 2}
	conns[3].Tags = []uint32{1, 2}
	conns[4].Tags = []uint32{1}
	conns[5].Tags = []uint32{2}
	conns[6].Tags = []uint32{3}
	conns[7].Tags = []uint32{2, 3}

	type fakeConn struct {
		tags []string
	}
	expectedTags := []fakeConn{
		{tags: []string{"tag0"}},
		{},
		{tags: []string{"tag0", "tag2"}},
		{tags: []string{"tag1", "tag2"}},
		{tags: []string{"tag1"}},
		{tags: []string{"tag2"}},
		{tags: []string{"tag3"}},
		{tags: []string{"tag2", "tag3"}},
	}
	foundTags := []fakeConn{}

	maxConnsPerMessage := 4
	serviceExtractorEnabled := false
	useWindowsServiceName := false
	useImprovedAlgorithm := false
	ex := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
	hostTagsProvider := hosttags.NewHostTagProvider()
	chunks := batchConnections(&HostInfo{}, hostTagsProvider, nil, nil, maxConnsPerMessage, 0, conns, nil, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, nil, tags, nil, ex, nil, nil)

	assert.Len(t, chunks, 2)
	total := 0
	for _, c := range chunks {
		connections := c.(*model.CollectorConnections)
		total += len(connections.Connections)
		for _, conn := range connections.Connections {
			// conn.Tags must be used between system-probe and the agent only
			assert.Nil(t, conn.Tags)

			foundTags = append(foundTags, fakeConn{tags: connections.GetConnectionsTags(conn.TagsIdx)})
		}
	}

	assert.Equal(t, 8, total)
	require.EqualValues(t, expectedTags, foundTags)
}

func TestNetworkConnectionTagsWithService(t *testing.T) {
	conns := makeConnections(1)
	tags := []string{"tag0"}
	conns[0].Tags = []uint32{0}

	// Have to be sorted with the usage of tags encoder v3
	expectedTags := []string{"process_context:my-server", "tag0"}

	procsByPid := map[int32]*procutil.Process{
		conns[0].Pid: {
			Pid:     conns[0].Pid,
			Cmdline: []string{"./my-server.sh"},
		},
	}
	mockConfig := configmock.NewSystemProbe(t)
	mockConfig.SetWithoutSource("system_probe_config.process_service_inference.enabled", true)

	maxConnsPerMessage := 1
	serviceExtractorEnabled := mockConfig.GetBool("system_probe_config.process_service_inference.enabled")
	useWindowsServiceName := mockConfig.GetBool("system_probe_config.process_service_inference.use_windows_service_name")
	useImprovedAlgorithm := mockConfig.GetBool("system_probe_config.process_service_inference.use_improved_algorithm")
	ex := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
	ex.Extract(procsByPid)

	hostTagsProvider := hosttags.NewHostTagProvider()
	chunks := batchConnections(&HostInfo{}, hostTagsProvider, nil, nil, maxConnsPerMessage, 0, conns, nil, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, nil, tags, nil, ex, nil, nil)

	assert.Len(t, chunks, 1)
	connections := chunks[0].(*model.CollectorConnections)
	assert.Len(t, connections.Connections, 1)
	require.EqualValues(t, expectedTags, connections.GetConnectionsTags(connections.Connections[0].TagsIdx))
}

func TestConvertAndEnrichWithServiceTags(t *testing.T) {
	tags := []string{"tag0", "tag1", "tag2"}

	tests := []struct {
		name       string
		tagOffsets []uint32
		serviceTag []string
		expected   []string
	}{
		{
			name:       "no tags",
			tagOffsets: nil,
			serviceTag: nil,
			expected:   nil,
		},
		{
			name:       "convert tags only",
			tagOffsets: []uint32{0, 2},
			serviceTag: nil,
			expected:   []string{"tag0", "tag2"},
		},
		{
			name:       "convert service tag only",
			tagOffsets: nil,
			serviceTag: []string{"process_context:dogfood"},
			expected:   []string{"process_context:dogfood"},
		},
		{
			name:       "convert tags with service tag",
			tagOffsets: []uint32{0, 2},
			serviceTag: []string{"process_context:doge"},
			expected:   []string{"tag0", "tag2", "process_context:doge"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, convertAndEnrichWithServiceCtx(tags, tt.tagOffsets, tt.serviceTag...))
		})
	}
}

func TestNetworkConnectionProcessTags(t *testing.T) {
	conns := makeConnections(4)

	// Set up mock tagger
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	// Configure process tags for specific PIDs
	pid1EntityID := taggertypes.NewEntityID(taggertypes.Process, strconv.Itoa(int(conns[0].Pid)))
	pid2EntityID := taggertypes.NewEntityID(taggertypes.Process, strconv.Itoa(int(conns[1].Pid)))
	pid3EntityID := taggertypes.NewEntityID(taggertypes.Process, strconv.Itoa(int(conns[2].Pid)))
	// Intentionally leave conns[3].Pid without tags to test empty case

	fakeTagger.SetTags(pid1EntityID, "process", nil, nil, []string{"env:prod", "service:web"}, nil)
	fakeTagger.SetTags(pid2EntityID, "process", nil, nil, []string{"env:staging", "team:backend"}, nil)
	fakeTagger.SetTags(pid3EntityID, "process", nil, nil, []string{"env:dev"}, nil)

	// Create process tag provider using the mock tagger
	processTagProvider := func(pid int32) ([]string, error) {
		if pid <= 0 {
			return nil, nil
		}
		processEntityID := taggertypes.NewEntityID(taggertypes.Process, strconv.Itoa(int(pid)))
		return fakeTagger.Tag(processEntityID, taggertypes.HighCardinality)
	}

	maxConnsPerMessage := 2
	serviceExtractorEnabled := false
	useWindowsServiceName := false
	useImprovedAlgorithm := false
	ex := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
	hostTagsProvider := hosttags.NewHostTagProvider()
	chunks := batchConnections(&HostInfo{}, hostTagsProvider, nil, processTagProvider, maxConnsPerMessage, 0, conns, nil, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, nil, nil, nil, ex, nil, nil)

	assert.Len(t, chunks, 2)

	// Verify first chunk (connections 0 and 1)
	connections0 := chunks[0].(*model.CollectorConnections)
	assert.Len(t, connections0.Connections, 2)

	// Check tags for first connection (PID 1)
	conn0Tags := connections0.GetConnectionsTags(connections0.Connections[0].TagsIdx)
	expectedTags0 := []string{"env:prod", "service:web"}
	assert.ElementsMatch(t, expectedTags0, conn0Tags)

	// Check tags for second connection (PID 2)
	conn1Tags := connections0.GetConnectionsTags(connections0.Connections[1].TagsIdx)
	expectedTags1 := []string{"env:staging", "team:backend"}
	assert.ElementsMatch(t, expectedTags1, conn1Tags)

	// Verify second chunk (connections 2 and 3)
	connections1 := chunks[1].(*model.CollectorConnections)
	assert.Len(t, connections1.Connections, 2)

	// Check tags for third connection (PID 3)
	conn2Tags := connections1.GetConnectionsTags(connections1.Connections[0].TagsIdx)
	expectedTags2 := []string{"env:dev"}
	assert.ElementsMatch(t, expectedTags2, conn2Tags)

	// Check tags for fourth connection (PID 4, no tags configured)
	conn3Tags := connections1.GetConnectionsTags(connections1.Connections[1].TagsIdx)
	assert.Empty(t, conn3Tags, "Connection with no configured process tags should have no tags")
}

func Test_getDNSNameForIP(t *testing.T) {
	tests := []struct {
		name     string
		conns    *model.Connections
		ip       string
		expected string
	}{
		{
			name: "IP exists with single DNS name",
			conns: &model.Connections{
				Dns: map[string]*model.DNSEntry{
					"1.2.3.4": {
						Names: []string{"example.com"},
					},
				},
			},
			ip:       "1.2.3.4",
			expected: "example.com",
		},
		{
			name: "IP exists with multiple DNS names - returns first",
			conns: &model.Connections{
				Dns: map[string]*model.DNSEntry{
					"1.2.3.4": {
						Names: []string{"example.com", "example.org", "example.net"},
					},
				},
			},
			ip:       "1.2.3.4",
			expected: "example.com",
		},
		{
			name: "IP exists but DNSEntry has no names",
			conns: &model.Connections{
				Dns: map[string]*model.DNSEntry{
					"1.2.3.4": {
						Names: []string{},
					},
				},
			},
			ip:       "1.2.3.4",
			expected: "",
		},
		{
			name: "IP exists but DNSEntry is nil",
			conns: &model.Connections{
				Dns: map[string]*model.DNSEntry{
					"1.2.3.4": nil,
				},
			},
			ip:       "1.2.3.4",
			expected: "",
		},
		{
			name: "IP does not exist in DNS map",
			conns: &model.Connections{
				Dns: map[string]*model.DNSEntry{
					"1.2.3.4": {
						Names: []string{"example.com"},
					},
				},
			},
			ip:       "5.6.7.8",
			expected: "",
		},
		{
			name: "DNS map is nil",
			conns: &model.Connections{
				Dns: nil,
			},
			ip:       "1.2.3.4",
			expected: "",
		},
		{
			name: "DNS map is empty",
			conns: &model.Connections{
				Dns: map[string]*model.DNSEntry{},
			},
			ip:       "1.2.3.4",
			expected: "",
		},
		{
			name: "IPv6 address with DNS name",
			conns: &model.Connections{
				Dns: map[string]*model.DNSEntry{
					"2001:db8::1": {
						Names: []string{"ipv6.example.com"},
					},
				},
			},
			ip:       "2001:db8::1",
			expected: "ipv6.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getDNSNameForIP(tt.conns, tt.ip)
			assert.Equal(t, tt.expected, result)
		})
	}
}
