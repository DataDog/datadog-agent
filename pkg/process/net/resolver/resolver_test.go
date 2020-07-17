package resolver

import (
	"net"
	"testing"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/stretchr/testify/assert"
)

func TestLocalResolver(t *testing.T) {
	assert := assert.New(t)

	resolver := &LocalResolver{}
	containers := []*containers.Container{
		{
			ID: "container-1",
			AddressList: []containers.NetworkAddress{
				{
					IP:       net.ParseIP("10.0.2.15"),
					Port:     32769,
					Protocol: "tcp",
				},
				{
					IP:       net.ParseIP("172.17.0.4"),
					Port:     6379,
					Protocol: "tcp",
				},
			},
		},
		{
			ID: "container-2",
			AddressList: []containers.NetworkAddress{
				{
					IP:       net.ParseIP("172.17.0.2"),
					Port:     80,
					Protocol: "tcp",
				},
			},
		},
		{
			ID: "container-3",
			AddressList: []containers.NetworkAddress{
				{
					IP:       net.ParseIP("10.0.2.15"),
					Port:     32769,
					Protocol: "udp",
				},
			},
		},
	}

	// Generate network address => container ID map
	resolver.LoadAddrs(containers)

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
