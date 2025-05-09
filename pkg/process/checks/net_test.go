// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	//"bytes"
	"fmt"
	//"io"
	//"net/http"
	//"net/http/httptest"
	//"strings"
	"testing"
	//"time"

	//"github.com/benbjohnson/clock" // Import clock for mocking
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	model "github.com/DataDog/agent-payload/v5/process"

	//workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"

	//"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	//netMarshal "github.com/DataDog/datadog-agent/pkg/network/encoding/marshal"
	//netEncoding "github.com/DataDog/datadog-agent/pkg/network/encoding/unmarshal"
	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	//"github.com/DataDog/datadog-agent/pkg/util"
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
		c.Laddr = &model.Addr{ContainerId: fmt.Sprintf("%d", c.Pid)}
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
	chunks := batchConnections(&HostInfo{}, maxConnsPerMessage, 0, p, dns, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, nil, nil, nil, ex)
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
		chunks := batchConnections(&HostInfo{}, tc.maxSize, 0, tc.cur, map[string]*model.DNSEntry{}, "nid", ctm, rctm, khfr, coretm, nil, nil, nil, nil, nil, ex)

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
				assert.Equal(t, fmt.Sprintf("%d", conn.Pid), connections.ContainerForPid[conn.Pid])
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
	chunks := batchConnections(&HostInfo{}, maxConnsPerMessage, 0, p, dns, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, nil, nil, nil, ex)

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
			assert.Equal(t, fmt.Sprintf("%d", conn.Pid), connections.ContainerForPid[conn.Pid])
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
	chunks := batchConnections(&HostInfo{}, maxConnsPerMessage, 0, p, map[string]*model.DNSEntry{}, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, nil, nil, nil, ex)

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
	chunks := batchConnections(&HostInfo{}, maxConnsPerMessage, 0, conns, dnsmap, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, domains, nil, nil, nil, ex)

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
	chunks := batchConnections(&HostInfo{}, maxConnsPerMessage, 0, conns, dnsmap, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, domains, nil, nil, nil, ex)

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
	chunks := batchConnections(&HostInfo{}, maxConnsPerMessage, 0, conns, nil, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, routes, nil, nil, ex)

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
	chunks := batchConnections(&HostInfo{}, maxConnsPerMessage, 0, conns, nil, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, nil, tags, nil, ex)

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

	expectedTags := []string{"tag0", "process_context:my-server"}

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

	chunks := batchConnections(&HostInfo{}, maxConnsPerMessage, 0, conns, nil, "nid", nil, nil, model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, nil, tags, nil, ex)

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
		serviceTag string
		expected   []string
	}{
		{
			name:       "no tags",
			tagOffsets: nil,
			serviceTag: "",
			expected:   []string{},
		},
		{
			name:       "convert tags only",
			tagOffsets: []uint32{0, 2},
			serviceTag: "",
			expected:   []string{"tag0", "tag2"},
		},
		{
			name:       "convert service tag only",
			tagOffsets: nil,
			serviceTag: "process_context:dogfood",
			expected:   []string{"process_context:dogfood"},
		},
		{
			name:       "convert tags with service tag",
			tagOffsets: []uint32{0, 2},
			serviceTag: "process_context:doge",
			expected:   []string{"tag0", "tag2", "process_context:doge"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, convertAndEnrichWithServiceCtx(tags, tt.tagOffsets, tt.serviceTag))
		})
	}
}

// TestConnectionsCheckConditionalRun tests the logic that decides whether to perform a full data fetch.
//func TestConnectionsCheckConditionalRun(t *testing.T) {
//	mockClock := clock.NewMock() // Mock the clock
//	wmeta := workloadmetamock.New(t)
//	config := configmock.New(t)
//	sysprobeYamlConfig := configmock.NewSystemProbe(t)
//
//	// Configure intervals
//	checkInterval := 5 * time.Second // How often Run is called *externally* by the runner
//	fullRunInterval := 30 * time.Second
//	config.SetWithoutSource("process_config.intervals.connections_check", checkInterval)
//	config.SetWithoutSource("process_config.connections_full_run_interval", fullRunInterval)
//	sysprobeYamlConfig.SetWithoutSource("system_probe_config.enabled", true)
//
//	// Variables to control mock server responses
//	capacityStatus := http.StatusNoContent // Default: Not near capacity
//
//	// Create sample *internal* connection data
//	internalConnections := &network.Connections{
//		BufferedData: network.BufferedData{
//			Conns: []network.ConnectionStats{
//				{
//					Pid:    123,
//					Type:   network.TCP,
//					Family: network.AFINET,
//					Source: util.AddressFromString("1.1.1.1"),
//					Dest:   util.AddressFromString("2.2.2.2"),
//					SPort:  1234,
//					DPort:  80,
//				},
//			},
//		},
//	}
//
//	// Marshal the internal representation to get the bytes for the mock server
//	marshaler := netMarshal.GetMarshaler(netEncoding.ContentTypeProtobuf)
//	buf := new(bytes.Buffer)
//	modeler := netMarshal.NewConnectionsModeler(internalConnections) // Use internalConnections here
//	err := marshaler.Marshal(internalConnections, buf, modeler)      // Use internalConnections here
//	modeler.Close()                                                  // Close the modeler to release resources
//	require.NoError(t, err, "Failed to marshal internalConnections")
//	connBody := buf.Bytes() // connBody now holds the *model.Connections bytes
//
//	// Mock System Probe Server
//	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		path := r.URL.Path
//		switch {
//		case strings.HasSuffix(path, "/register"):
//			w.WriteHeader(http.StatusOK)
//		case strings.HasSuffix(path, "/network_id"):
//			_, _ = io.WriteString(w, "test-network-id")
//		case strings.HasSuffix(path, "/connections/check_capacity"):
//			w.WriteHeader(capacityStatus) // Use the variable 'capacityStatus' here
//		case strings.HasSuffix(path, "/connections"):
//			w.Header().Set("Content-type", netEncoding.ContentTypeProtobuf)
//			_, _ = w.Write(connBody)
//		default:
//			w.WriteHeader(http.StatusNotFound)
//		}
//	}))
//	defer server.Close()
//
//	sysprobeAddr := server.Listener.Addr().String()
//	sysProbeCfg := &SysProbeConfig{SystemProbeAddress: sysprobeAddr, MaxConnsPerMessage: 100}
//	// Use sysprobeYamlConfig for system probe settings, nil for actual sysconfig (as it's not directly used here)
//	check := NewConnectionsCheck(config, sysprobeYamlConfig, nil, wmeta, nil) // npCollector not needed for this test
//	hostInfo := &HostInfo{HostName: "test-host"}
//	err = check.Init(sysProbeCfg, hostInfo, false)
//	require.NoError(t, err)
//	// Override clock used by resolver *after* Init completes by accessing the exported field
//	check.localresolver.Clock = mockClock
//
//	// 1. Initial run: Should always be a full run because lastFullRunTime starts at zero.
//	t.Run("initial run", func(t *testing.T) {
//		capacityStatus = http.StatusNoContent // Ensure capacity is OK
//		mockClock.Set(time.Now())             // Use current time for the first run
//		result, err := check.Run(func() int32 { return 1 }, nil)
//		require.NoError(t, err)
//		require.NotNil(t, result, "Initial run should perform full collection")
//		payloads := result.Payloads()
//		require.NotEmpty(t, payloads, "Initial run should yield payloads")
//		assert.Equal(t, mockClock.Now(), check.lastFullRunTime, "lastFullRunTime should be updated on initial run")
//	})
//
//	lastRunTime := check.lastFullRunTime // Capture time after the first full run
//
//	// 2. Skipped run: Interval hasn't passed, capacity is OK.
//	t.Run("skipped run", func(t *testing.T) {
//		capacityStatus = http.StatusNoContent // Ensure capacity is OK
//		// Advance time by less than the guaranteed run interval
//		mockClock.Add(check.guaranteedRunInterval / 2)
//		result, err := check.Run(func() int32 { return 2 }, nil)
//		require.NoError(t, err)
//		require.NotNil(t, result, "Skipped run result should not be nil")
//		payloads := result.Payloads()
//		require.Empty(t, payloads, "Run should be skipped (no payloads)")
//		assert.Equal(t, lastRunTime, check.lastFullRunTime, "lastFullRunTime should NOT be updated on skipped run")
//	})
//
//	// 3. Capacity-triggered run: Interval hasn't passed, but capacity is HIGH.
//	t.Run("capacity-triggered run", func(t *testing.T) {
//		capacityStatus = http.StatusOK // Set capacity to HIGH
//		// Still within guaranteedRunInterval relative to last *full* run
//		mockClock.Add(check.guaranteedRunInterval / 2)
//		result, err := check.Run(func() int32 { return 3 }, nil)
//		require.NoError(t, err)
//		require.NotNil(t, result, "Capacity-triggered run should perform full collection")
//		payloads := result.Payloads()
//		require.NotEmpty(t, payloads, "Capacity-triggered run should yield payloads")
//		assert.Equal(t, mockClock.Now(), check.lastFullRunTime, "lastFullRunTime should be updated on capacity-triggered run")
//	})
//
//	lastRunTime = check.lastFullRunTime // Capture time after the capacity-triggered full run
//
//	// 4. Time-triggered run: Interval *has* passed, capacity is OK.
//	t.Run("time-triggered run", func(t *testing.T) {
//		capacityStatus = http.StatusNoContent // Ensure capacity is OK
//		// Advance time past the guaranteedRunInterval relative to the last full run
//		mockClock.Set(lastRunTime.Add(check.guaranteedRunInterval + time.Second))
//		result, err := check.Run(func() int32 { return 4 }, nil)
//		require.NoError(t, err)
//		require.NotNil(t, result, "Time-triggered run should perform full collection")
//		payloads := result.Payloads()
//		require.NotEmpty(t, payloads, "Time-triggered run should yield payloads")
//		assert.Equal(t, mockClock.Now(), check.lastFullRunTime, "lastFullRunTime should be updated on time-triggered run")
//	})
//
//	lastRunTime = check.lastFullRunTime // Capture time after the time-triggered full run
//
//	// 5. Capacity check fails, time interval not met -> skip run.
//	t.Run("capacity check fails - skip", func(t *testing.T) {
//		capacityStatus = http.StatusInternalServerError // Simulate capacity check failure
//		// Advance time, but still within guaranteedRunInterval
//		mockClock.Add(check.guaranteedRunInterval / 2)
//		result, err := check.Run(func() int32 { return 5 }, nil)
//		require.NoError(t, err) // The check itself shouldn't error, just log a warning internally
//		require.NotNil(t, result, "Run result should not be nil even if capacity check fails")
//		payloads := result.Payloads()
//		require.Empty(t, payloads, "Run should be skipped when capacity check fails and time interval not met")
//		assert.Equal(t, lastRunTime, check.lastFullRunTime, "lastFullRunTime should NOT be updated when skipped due to failed capacity check")
//	})
//
//	// 6. Capacity check fails, but time interval IS met -> run anyway.
//	t.Run("capacity check fails - time trigger", func(t *testing.T) {
//		capacityStatus = http.StatusInternalServerError // Simulate capacity check failure
//		// Advance time past the guaranteedRunInterval relative to the last full run
//		mockClock.Set(lastRunTime.Add(check.guaranteedRunInterval + time.Second))
//		result, err := check.Run(func() int32 { return 6 }, nil)
//		require.NoError(t, err) // The check itself shouldn't error
//		require.NotNil(t, result, "Run should perform full collection based on time, despite capacity check failure")
//		payloads := result.Payloads()
//		require.NotEmpty(t, payloads, "Run should yield payloads when triggered by time, despite capacity check failure")
//		assert.Equal(t, mockClock.Now(), check.lastFullRunTime, "lastFullRunTime should be updated on time-triggered run even if capacity check fails")
//	})
//}
//
//// Create sample internal connection data
//func createSampleInternalConnections() *network.Connections {
//	return &network.Connections{
//		BufferedData: network.BufferedData{
//			Conns: []network.ConnectionStats{
//				{
//					Pid:    123,
//					Type:   network.TCP,
//					Family: network.AFINET,
//					Source: util.AddressFromString("1.1.1.1"),
//					Dest:   util.AddressFromString("2.2.2.2"),
//					SPort:  1234,
//					DPort:  80,
//				},
//			},
//		},
//	}
//}
//
//// Marshal the internal representation to get the bytes for the mock server
//func marshalInternalConnections(t *testing.T, connections *network.Connections) []byte {
//	marshaler := netMarshal.GetMarshaler(netEncoding.ContentTypeProtobuf)
//	buf := new(bytes.Buffer)
//	modeler := netMarshal.NewConnectionsModeler(connections)
//	err := marshaler.Marshal(connections, buf, modeler)
//	modeler.Close() // Close the modeler to release resources
//	require.NoError(t, err, "Failed to marshal internalConnections")
//	return buf.Bytes() // connBody now holds the *model.Connections bytes
//}
//
//// Mock System Probe Server
//func mockSystemProbeServer(t *testing.T, connBody []byte) *httptest.Server {
//	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		path := r.URL.Path
//		switch {
//		case strings.HasSuffix(path, "/register"):
//			w.WriteHeader(http.StatusOK)
//		case strings.HasSuffix(path, "/network_id"):
//			_, _ = io.WriteString(w, "test-network-id")
//		case strings.HasSuffix(path, "/connections/check_capacity"):
//			w.WriteHeader(http.StatusNoContent)
//		case strings.HasSuffix(path, "/connections"):
//			w.Header().Set("Content-type", netEncoding.ContentTypeProtobuf)
//			_, _ = w.Write(connBody)
//		default:
//			w.WriteHeader(http.StatusNotFound)
//		}
//	}))
//	return server
//}
