// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package resolver

import (
	"testing"

	"github.com/stretchr/testify/assert"

	model "github.com/DataDog/agent-payload/v5/process"
)

func TestLocalResolver(t *testing.T) {
	assert := assert.New(t)

	resolver := &LocalResolver{}
	containers := []*model.Container{
		{
			Id: "container-1",
			Addresses: []*model.ContainerAddr{
				{
					Ip:       "10.0.2.15",
					Port:     32769,
					Protocol: model.ConnectionType_tcp,
				},
				{
					Ip:       "172.17.0.4",
					Port:     6379,
					Protocol: model.ConnectionType_tcp,
				},
			},
		},
		{
			Id: "container-2",
			Addresses: []*model.ContainerAddr{
				{
					Ip:       "172.17.0.2",
					Port:     80,
					Protocol: model.ConnectionType_tcp,
				},
			},
		},
		{
			Id: "container-3",
			Addresses: []*model.ContainerAddr{
				{
					Ip:       "10.0.2.15",
					Port:     32769,
					Protocol: model.ConnectionType_udp,
				},
			},
		},
	}

	// Generate network address => container ID map
	resolver.LoadAddrs(containers, nil)

	connections := &model.Connections{
		Conns: []*model.Connection{
			// connection 0
			{
				Type: model.ConnectionType_tcp,
				Raddr: &model.Addr{
					Ip:   "10.0.2.15",
					Port: 32769,
				},
			},
			// connection 1
			{
				Type: model.ConnectionType_tcp,
				Raddr: &model.Addr{
					Ip:   "172.17.0.4",
					Port: 6379,
				},
			},
			// connection 2
			{
				Type: model.ConnectionType_tcp,
				Raddr: &model.Addr{
					Ip:   "172.17.0.2",
					Port: 80,
				},
			},
			// connection 3
			{
				Type: model.ConnectionType_udp,
				Raddr: &model.Addr{
					Ip:   "10.0.2.15",
					Port: 32769,
				},
			},
		},
	}

	resolver.Resolve(connections)
	assert.Equal("container-1", connections.Conns[0].Raddr.ContainerId)
	assert.Equal("container-1", connections.Conns[1].Raddr.ContainerId)
	assert.Equal("container-2", connections.Conns[2].Raddr.ContainerId)
	assert.Equal("container-3", connections.Conns[3].Raddr.ContainerId)
}

func TestResolveLoopbackConnections(t *testing.T) {
	tests := []struct {
		name            string
		conn            *model.Connection
		expectedLaddrID string
		expectedRaddrID string
	}{
		{
			name: "raddr resolution with nat",
			conn: &model.Connection{
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
					ReplDstIP:   "127.0.0.1",
					ReplDstPort: 1234,
					ReplSrcIP:   "10.1.1.2",
					ReplSrcPort: 1234,
				},
				NetNS:     1,
				Direction: model.ConnectionDirection_incoming,
				IntraHost: true,
			},
			expectedLaddrID: "foo1",
			expectedRaddrID: "foo2",
		},
		{
			name: "raddr resolution with nat to localhost",
			conn: &model.Connection{
				Pid:   2,
				NetNS: 1,
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
				Direction: model.ConnectionDirection_outgoing,
				IntraHost: true,
			},
			expectedLaddrID: "foo2",
			expectedRaddrID: "foo1",
		},
		{
			name: "raddr failed localhost resolution",
			conn: &model.Connection{
				Pid:   3,
				NetNS: 3,
				Laddr: &model.Addr{
					Ip:   "127.0.0.1",
					Port: 1235,
				},
				Raddr: &model.Addr{
					Ip:   "127.0.0.1",
					Port: 1234,
				},
				IntraHost: true,
				Direction: model.ConnectionDirection_incoming,
			},
			expectedLaddrID: "foo3",
			expectedRaddrID: "",
		},
		{
			name: "raddr resolution within same netns (3)",
			conn: &model.Connection{
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
				IntraHost: true,
				Direction: model.ConnectionDirection_outgoing,
			},
			expectedLaddrID: "foo5",
			expectedRaddrID: "foo3",
		},
		{
			name: "raddr failed resolution, known address in different netns",
			conn: &model.Connection{
				Pid:   5,
				NetNS: 4,
				Laddr: &model.Addr{
					Ip:   "127.0.0.1",
					Port: 1240,
				},
				Raddr: &model.Addr{
					Ip:   "127.0.0.1",
					Port: 1235,
				},
				IntraHost: true,
				Direction: model.ConnectionDirection_outgoing,
			},
			expectedLaddrID: "foo5",
			expectedRaddrID: "",
		},
		{
			name: "failed laddr and raddr resolution",
			conn: &model.Connection{
				Pid:   10,
				NetNS: 10,
				Laddr: &model.Addr{
					Ip:   "127.0.0.1",
					Port: 1234,
				},
				Raddr: &model.Addr{
					Ip:   "10.1.1.1",
					Port: 1235,
				},
				Direction: model.ConnectionDirection_outgoing,
				IntraHost: false,
			},
			expectedLaddrID: "",
			expectedRaddrID: "",
		},
		{
			name: "failed resolution: unknown pid for laddr, raddr address in different netns from known address",
			conn: &model.Connection{
				Pid:   11,
				NetNS: 10,
				Laddr: &model.Addr{
					Ip:   "127.0.0.1",
					Port: 1250,
				},
				Raddr: &model.Addr{
					Ip:   "127.0.0.1",
					Port: 1240,
				},
				Direction: model.ConnectionDirection_outgoing,
				IntraHost: true,
			},
			expectedLaddrID: "",
			expectedRaddrID: "",
		},
		{
			name: "localhost resolution within same netns 1/2",
			conn: &model.Connection{
				Pid:   6,
				NetNS: 7,
				Laddr: &model.Addr{
					Ip:   "127.0.0.1",
					Port: 1260,
				},
				Raddr: &model.Addr{
					Ip:   "127.0.0.1",
					Port: 1250,
				},
				Direction: model.ConnectionDirection_outgoing,
				IntraHost: true,
			},
			expectedLaddrID: "foo6",
			expectedRaddrID: "foo7",
		},
		{
			name: "localhost resolution within same netns 2/2",
			conn: &model.Connection{
				Pid:   7,
				NetNS: 7,
				Laddr: &model.Addr{
					Ip:   "127.0.0.1",
					Port: 1250,
				},
				Raddr: &model.Addr{
					Ip:   "127.0.0.1",
					Port: 1260,
				},
				Direction: model.ConnectionDirection_incoming,
				IntraHost: true,
			},
			expectedLaddrID: "foo7",
			expectedRaddrID: "foo6",
		},
	}

	resolver := &LocalResolver{}
	resolver.LoadAddrs(nil, map[int]string{
		1:  "foo1",
		2:  "foo2",
		3:  "foo3",
		4:  "foo4",
		5:  "foo5",
		6:  "foo6",
		7:  "foo7",
		8:  "bar",
		20: "foo20",
		21: "foo21",
	})

	conns := &model.Connections{}
	for _, te := range tests {
		conns.Conns = append(conns.Conns, te.conn)
	}
	resolver.Resolve(conns)

	for _, te := range tests {
		t.Run(te.name, func(t *testing.T) {
			assert.Equal(t, te.expectedLaddrID, te.conn.Laddr.ContainerId, "laddr container id does not match expected value")
			assert.Equal(t, te.expectedRaddrID, te.conn.Raddr.ContainerId, "raddr container id does not match expected value")
		})
	}
}
