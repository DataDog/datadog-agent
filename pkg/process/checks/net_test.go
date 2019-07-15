package checks

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/model"
	"github.com/stretchr/testify/assert"
)

func makeConnection(pid int32) *model.Connection {
	return &model.Connection{Pid: pid}
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

	cfg := config.NewDefaultAgentConfig()

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
		chunks := batchConnections(cfg, 0, tc.cur)

		assert.Len(t, chunks, tc.expectedChunks, "len %d", i)
		total := 0
		for _, c := range chunks {
			connections := c.(*model.CollectorConnections)
			total += len(connections.Connections)
			assert.Equal(t, int32(tc.expectedChunks), connections.GroupSize, "group size test %d", i)

			// make sure we could get container and pid mapping for connections
			assert.Equal(t, len(connections.Connections), len(connections.ContainerForPid))
			for _, conn := range connections.Connections {
				assert.Contains(t, connections.ContainerForPid, conn.Pid)
				assert.Equal(t, fmt.Sprintf("%d", conn.Pid), connections.ContainerForPid[conn.Pid])
			}
		}
		assert.Equal(t, tc.expectedTotal, total, "total test %d", i)
	}
}

func TestCountConnectionsPerPID(t *testing.T) {
	cp1tcp := &model.Connection{Pid: 1, Type: model.ConnectionType_tcp}
	cp2tcp := &model.Connection{Pid: 2, Type: model.ConnectionType_tcp}
	cp2udp := &model.Connection{Pid: 2, Type: model.ConnectionType_udp}
	cp3udp := &model.Connection{Pid: 3, Type: model.ConnectionType_udp}

	for _, tc := range []struct {
		in       []*model.Connection
		expected map[int32]ConnectionsCounts
		desc     string
	}{
		{in: []*model.Connection{}, desc: "empty list"},
		{
			in:       []*model.Connection{cp1tcp, cp1tcp, cp1tcp},
			expected: map[int32]ConnectionsCounts{1: ConnectionsCounts{TCP: 3}},
			desc:     "only one PID, tcp",
		},

		{
			in:       []*model.Connection{cp3udp, cp3udp},
			expected: map[int32]ConnectionsCounts{3: ConnectionsCounts{UDP: 2}},
			desc:     "only one PID, udp",
		},
		{
			in: []*model.Connection{cp1tcp, cp2tcp, cp2udp, cp3udp},
			expected: map[int32]ConnectionsCounts{
				1: ConnectionsCounts{TCP: 1},
				2: ConnectionsCounts{TCP: 1, UDP: 1},
				3: ConnectionsCounts{UDP: 1},
			},
			desc: "multiple PIDs",
		},
	} {
		out := countConnectionsPerPID(tc.in)
		assert.Equal(t, tc.expected, out, tc.desc)
	}
}
