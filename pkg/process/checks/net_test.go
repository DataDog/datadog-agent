package checks

import (
	"fmt"
	"testing"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestNetworkConnectionBatching(t *testing.T) {
	cfg := config.NewDefaultAgentConfig(false)

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
		cfg.MaxConnsPerMessage = tc.maxSize
		ctm := &model.CollectorConnectionsTelemetry{}
		rctm := map[string]*model.RuntimeCompilationTelemetry{}
		chunks := batchConnections(cfg, 0, tc.cur, map[string]*model.DNSEntry{}, "nid", ctm, rctm, nil, nil)

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
				assert.NotNil(t, connections.ConnTelemetry)
				assert.NotNil(t, connections.CompilationTelemetryByAsset)
			} else {
				assert.Nil(t, connections.ConnTelemetry)
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

	cfg := config.NewDefaultAgentConfig(false)
	cfg.MaxConnsPerMessage = 1

	chunks := batchConnections(cfg, 0, p, dns, "nid", nil, nil, nil, nil)

	assert.Len(t, chunks, 4)
	total := 0
	for i, c := range chunks {
		connections := c.(*model.CollectorConnections)

		// Only the last chunk should have a DNS mapping
		if i == 3 {
			assert.NotEmpty(t, connections.EncodedDNS)
		} else {
			assert.Empty(t, connections.EncodedDNS)
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

	cfg := config.NewDefaultAgentConfig(false)
	cfg.MaxConnsPerMessage = 2

	chunks := batchConnections(cfg, 0, p, map[string]*model.DNSEntry{}, "nid", nil, nil, nil, nil)

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

func TestNetworkConnectionBatchingWithDomains(t *testing.T) {
	conns := makeConnections(4)

	domains := []string{"foo.com", "bar.com", "baz.com"}
	conns[1].DnsStatsByDomain = map[int32]*model.DNSStats{
		0: {DnsTimeouts: 1},
	}
	conns[2].DnsStatsByDomain = map[int32]*model.DNSStats{
		0: {DnsTimeouts: 1},
		2: {DnsTimeouts: 1},
	}
	conns[3].DnsStatsByDomain = map[int32]*model.DNSStats{
		1: {DnsTimeouts: 1},
		2: {DnsTimeouts: 1},
	}
	dns := map[string]*model.DNSEntry{}

	cfg := config.NewDefaultAgentConfig(false)
	cfg.MaxConnsPerMessage = 1

	chunks := batchConnections(cfg, 0, conns, dns, "nid", nil, nil, domains, nil)

	assert.Len(t, chunks, 4)
	total := 0
	for i, c := range chunks {
		connections := c.(*model.CollectorConnections)
		total += len(connections.Connections)
		switch i {
		case 0:
			assert.Equal(t, []string{"", "", ""}, connections.Domains)
		case 1:
			assert.Equal(t, []string{"foo.com", "", ""}, connections.Domains)
		case 2:
			assert.Equal(t, []string{"foo.com", "", "baz.com"}, connections.Domains)
		case 3:
			assert.Equal(t, []string{"", "bar.com", "baz.com"}, connections.Domains)
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

	cfg := config.NewDefaultAgentConfig(false)
	cfg.MaxConnsPerMessage = 4

	chunks := batchConnections(cfg, 0, conns, nil, "nid", nil, nil, nil, routes)

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
