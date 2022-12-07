// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parser

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// The example below represents the following scenario:
//
// * Two containers (in this example a redis-client and a redis-server) are
//   running on the same host (IP 10.0.2.15)
//
// * The redis-server binds to host port 32769
//   (`docker run --rm -d -p 6379:32769 redis:alpine`)
//
// * The redis-client communicates to redis-server via the host IP/port
//   (`docker run --rm redis:alpine redis-cli -h 10.0.2.15 -p 32769 set foo bar`)
//
// Since the two containers are co-located within the same host and the communication
// is done via the Host IP/Port, network traffic flows through docker-proxy.
// This ends up generating the following flows:
//
// 1) redis-client -> redis-server (via host IP)
// 2) redis-server (via host IP) <- redis-client
// 3) docker-proxy -> redis-server (redundant)
// 4) redis-server <- docker-proxy (redundant)
//
// The purpose of this package is to filter flows like (3) and (4) in order to
// avoid double counting traffic represented similar to flows (1) and (2)
func TestProxyFiltering(t *testing.T) {
	proxyFilter := NewDockerProxy()
	proxyFilter.Extract(processData())

	// (1) This represents the *outgoing* connection from redis client to redis rerver (via host IP)
	// It should be *kept*
	c1 := &model.Connection{
		Pid: 24296,
		Laddr: &model.Addr{
			Ip:   "172.17.0.3",
			Port: 37340,
		},
		Raddr: &model.Addr{
			Ip:   "10.0.2.15",
			Port: 32768,
		},
		Direction: model.ConnectionDirection_outgoing,
	}

	// (2) This represents the *incoming* connection on redis server from redis client (via host IP)
	// It should be *kept*
	c2 := &model.Connection{
		Pid: 23211,
		Laddr: &model.Addr{
			Ip:   "10.0.2.15",
			Port: 32768,
		},
		Raddr: &model.Addr{
			Ip:   "172.17.0.3",
			Port: 37340,
		},
		Direction: model.ConnectionDirection_incoming,
	}

	// (3) This represents the *outgoing* connection from docker-proxy to redis server
	// It should be *dropped*
	c3 := &model.Connection{
		Pid: 23211,
		Laddr: &model.Addr{
			Ip:   "172.17.0.1",
			Port: 34050,
		},
		Raddr: &model.Addr{
			Ip:   "172.17.0.2",
			Port: 6379,
		},
		Direction: model.ConnectionDirection_outgoing,
	}

	// (4) This represents the *incoming* connection on redis server from docker proxy
	// It should be *dropped*
	c4 := &model.Connection{
		Pid: 23233,
		Laddr: &model.Addr{
			Ip:   "172.17.0.2",
			Port: 6379,
		},
		Raddr: &model.Addr{
			Ip:   "172.17.0.1",
			Port: 34050,
		},
		Direction: model.ConnectionDirection_incoming,
	}

	// Filter docker-proxy traffic in place
	payload := &model.Connections{Conns: []*model.Connection{c1, c2, c3, c4}}
	proxyFilter.Filter(payload)
	assert.Len(t, payload.Conns, 2)
	assert.Equal(t, c1, payload.Conns[0])
	assert.Equal(t, c2, payload.Conns[1])
}

func processData() map[int32]*procutil.Process {
	return map[int32]*procutil.Process{
		1: {
			Pid: 1,
			Cmdline: []string{
				"/sbin/init",
			},
		},
		23211: {
			Pid: 23211,
			Cmdline: []string{
				"/usr/bin/docker-proxy",
				"-proto",
				"tcp",
				"-host-ip",
				"0.0.0.0",
				"-host-port",
				"32769",
				"-container-ip",
				"172.17.0.2",
				"-container-port",
				"6379",
			},
		},
	}
}
