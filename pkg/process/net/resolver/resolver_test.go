package resolver

import (
	"fmt"
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
		{
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
		},
		{
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
		},
	}

	resolver := &LocalResolver{}
	resolver.LoadAddrs(
		[]*containers.Container{
			{
				ID:   "foo1",
				Pids: []int32{1},
			},
			{
				ID:   "foo2",
				Pids: []int32{2},
			},
			{
				ID:   "foo3",
				Pids: []int32{3},
			},
			{
				ID:   "foo4",
				Pids: []int32{4},
			},
			{
				ID:   "foo5",
				Pids: []int32{5},
			},
			{
				ID:   "foo6",
				Pids: []int32{6},
			},
			{
				ID:   "foo7",
				Pids: []int32{7},
			},
			{
				ID:   "bar",
				Pids: []int32{20},
			},
		},
	)

	resolvedByRaddr := map[string]string{
		"10.1.1.2:1234":  "foo2",
		"10.1.1.1:1234":  "foo1",
		"127.0.0.1:1235": "foo3",
		"1.2.3.4:1234":   "bar",
		"127.0.0.1:1250": "foo7",
		"127.0.0.1:1260": "foo6",
	}

	resolver.Resolve(&model.Connections{Conns: tests})

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
		assert.True(t, found || te.Laddr.ContainerId == resolver.ctrForPid[te.Pid])

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
