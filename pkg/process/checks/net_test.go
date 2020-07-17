package checks

import (
	"fmt"
	"testing"
	"time"

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

	Process.lastCtrIDForPID = map[int32]string{}
	for _, proc := range p {
		Process.lastCtrIDForPID[proc.Pid] = fmt.Sprintf("%d", proc.Pid)
	}
	// update lastRun to indicate that Process check is enabled and ran
	Process.lastRun = time.Now()

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

	Process.lastCtrIDForPID = map[int32]string{}
	for _, proc := range p {
		Process.lastCtrIDForPID[proc.Pid] = fmt.Sprintf("%d", proc.Pid)
	}
	Process.lastRun = time.Now() // Update lastRun to indicate that Process check is enabled and ran

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
