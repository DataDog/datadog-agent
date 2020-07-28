package checks

import (
	"fmt"
	"testing"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/stretchr/testify/assert"
)

func makeConnection(pid int32) *model.Connection {
	return &model.Connection{
		Pid:   pid,
		Laddr: &model.Addr{},
		Raddr: &model.Addr{},
	}
}

func TestNetworkConnectionBatching(t *testing.T) {
	p := []*model.Connection{
		makeConnection(1),
		makeConnection(2),
		makeConnection(3),
		makeConnection(4),
	}

	for _, proc := range p {
		proc.Laddr = &model.Addr{ContainerId: fmt.Sprintf("%d", proc.Pid)}
	}

	cfg := config.NewDefaultAgentConfig(false)

	for i, tc := range []struct {
		cur, last      []*model.Connection
		maxSize        int
		expectedTotal  int
		expectedChunks int
	}{
		{
			cur:            []*model.Connection{p[0], p[1], p[2]},
			maxSize:        1,
			expectedTotal:  3,
			expectedChunks: 3,
		},
		{
			cur:            []*model.Connection{p[0], p[1], p[2]},
			maxSize:        2,
			expectedTotal:  3,
			expectedChunks: 2,
		},
		{
			cur:            []*model.Connection{p[0], p[1], p[2], p[3]},
			maxSize:        10,
			expectedTotal:  4,
			expectedChunks: 1,
		},
		{
			cur:            []*model.Connection{p[0], p[1], p[2], p[3]},
			maxSize:        3,
			expectedTotal:  4,
			expectedChunks: 2,
		},
		{
			cur:            []*model.Connection{p[0], p[1], p[2], p[3], p[2], p[3]},
			maxSize:        2,
			expectedTotal:  6,
			expectedChunks: 3,
		},
	} {
		cfg.MaxConnsPerMessage = tc.maxSize
		tm := &model.CollectorConnectionsTelemetry{}
		chunks := batchConnections(cfg, 0, tc.cur, map[string]*model.DNSEntry{}, "nid", tm)

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
				assert.NotNil(t, connections.Telemetry)
			} else {
				assert.Nil(t, connections.Telemetry)
			}
		}
		assert.Equal(t, tc.expectedTotal, total, "total test %d", i)
	}
}

func TestNetworkConnectionBatchingWithDNS(t *testing.T) {
	p := []*model.Connection{
		makeConnection(1),
		makeConnection(2),
		makeConnection(3),
		makeConnection(4),
	}

	p[0].Raddr.Ip = "1.1.2.3"
	dns := map[string]*model.DNSEntry{
		"1.1.2.3": {Names: []string{"datacat.edu"}},
	}

	for _, proc := range p {
		proc.Laddr = &model.Addr{ContainerId: fmt.Sprintf("%d", proc.Pid)}
	}

	cfg := config.NewDefaultAgentConfig(false)
	cfg.MaxConnsPerMessage = 1

	chunks := batchConnections(cfg, 0, p, dns, "nid", nil)

	assert.Len(t, chunks, 4)
	total := 0
	for i, c := range chunks {
		connections := c.(*model.CollectorConnections)

		// Only the first chunk should have a DNS mapping!
		if i == 0 {
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

func TestResolveLoopbackConnections(t *testing.T) {
	unresolvedRaddr := map[string]*model.Addr{
		"127.0.0.1:1234": {
			Ip:   "127.0.0.1",
			Port: 1234,
		},
		"127.0.0.1:1235": {
			Ip:   "127.0.0.1",
			Port: 1235,
		},
		"127.0.0.1:1240": {
			Ip:   "127.0.0.1",
			Port: 1240,
		},
		"10.1.1.1:1234": {
			Ip:   "10.1.1.1",
			Port: 1234,
		},
		"10.1.1.1:1235": {
			Ip:   "10.1.1.1",
			Port: 1235,
		},
	}

	unresolvedLaddr := map[string]*model.Addr{
		"127.0.0.1:1240": {
			Ip:   "127.0.0.1",
			Port: 1240,
		},
		"127.0.0.1:1250": {
			Ip:   "127.0.0.1",
			Port: 1250,
		},
	}

	tests := []*model.Connection{
		{
			Pid: 1,
			Laddr: &model.Addr{
				Ip:   "127.0.0.1",
				Port: 1234,
			},
			Raddr: &model.Addr{
				Ip:   "10.1.1.2",
				Port: 1234,
			},
			IpTranslation: &model.IPTranslation{
				ReplDstIP:   "10.1.1.1",
				ReplDstPort: 1234,
				ReplSrcIP:   "10.1.1.2",
				ReplSrcPort: 1234,
			},
			NetNS: 1,
		},
		{
			Pid:   2,
			NetNS: 2,
			Laddr: &model.Addr{
				Ip:   "10.1.1.2",
				Port: 1234,
			},
			Raddr: &model.Addr{
				Ip:   "10.1.1.1",
				Port: 1234,
			},
			IpTranslation: &model.IPTranslation{
				ReplDstIP:   "10.1.1.2",
				ReplDstPort: 1234,
				ReplSrcIP:   "127.0.0.1",
				ReplSrcPort: 1234,
			},
		},
		{
			Pid:   3,
			NetNS: 3,
			Laddr: &model.Addr{
				Ip:   "127.0.0.1",
				Port: 1235,
			},
			Raddr: unresolvedRaddr["127.0.0.1:1234"],
		},
		{
			Pid:   5,
			NetNS: 3,
			Laddr: &model.Addr{
				Ip:   "127.0.0.1",
				Port: 1240,
			},
			Raddr: &model.Addr{
				Ip:   "127.0.0.1",
				Port: 1235,
			},
		},
		{
			Pid:   5,
			NetNS: 4,
			Laddr: &model.Addr{
				Ip:   "127.0.0.1",
				Port: 1240,
			},
			Raddr: unresolvedRaddr["127.0.0.1:1235"],
		},
		{
			Pid:   10,
			NetNS: 10,
			Laddr: unresolvedLaddr["127.0.0.1:1240"],
			Raddr: unresolvedRaddr["10.1.1.1:1235"],
		},
		{
			Pid:   11,
			NetNS: 10,
			Laddr: unresolvedLaddr["127.0.0.1:1250"],
			Raddr: unresolvedRaddr["127.0.0.1:1240"],
		},
		{
			Pid:   20,
			NetNS: 20,
			Laddr: &model.Addr{
				Ip:          "1.2.3.4",
				Port:        1234,
				ContainerId: "baz",
			},
			Raddr: &model.Addr{
				Ip:          "1.2.3.4",
				Port:        1234,
				ContainerId: "bar",
			},
		},
	}

	ctrsByPid := map[int32]string{
		1:  "foo1",
		2:  "foo2",
		3:  "foo3",
		4:  "foo4",
		5:  "foo5",
		20: "baz",
	}

	resolvedByRaddr := map[string]string{
		"10.1.1.2:1234":  "foo2",
		"10.1.1.1:1234":  "foo1",
		"127.0.0.1:1235": "foo3",
		"1.2.3.4:1234":   "bar",
	}

	resolveLoopbackConnections(tests, ctrsByPid)

	for _, te := range tests {
		found := false
		for _, u := range unresolvedLaddr {
			if te.Laddr == u {
				assert.True(t, te.Laddr.ContainerId == "", "laddr container should not be resolved for conn but is: %v", te)
				found = true
				break
			}
		}

		assert.True(t, found || te.Laddr.ContainerId != "", "laddr container should be resolved for conn but is not: %v", te)
		assert.True(t, found || te.Laddr.ContainerId == ctrsByPid[te.Pid])

		found = false
		for _, u := range unresolvedRaddr {
			if te.Raddr == u {
				assert.True(t, te.Raddr.ContainerId == "", "raddr container should not be resolved for conn but is:%v", te)
				found = true
				break
			}
		}

		assert.True(t, found || te.Raddr.ContainerId != "", "raddr container should be resolved for conn but is not: %v", te)
		assert.True(t, found || te.Raddr.ContainerId == resolvedByRaddr[fmt.Sprintf("%s:%d", te.Raddr.Ip, te.Raddr.Port)], "raddr container should be resolved for conn but is not: %v", te)
	}
}
