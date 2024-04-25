// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || linux_bpf

package network

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	netlinktestutil "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/testdns"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"

	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/vishvananda/netns"
)

var kv470 = kernel.VersionCode(4, 7, 0)
var kv = kernel.MustHostVersion()

type gatewayLookupSuite struct {
	suite.Suite
}

func TestGatewayLookupSuite(t *testing.T) {
	suite.Run(t, new(gatewayLookupSuite))
}

func (s *gatewayLookupSuite) TestGatewayLookupNotEnabled() {
	t := s.T()
	t.Run("gateway lookup enabled, not on aws", func(t *testing.T) {
		oldCloud := cloud
		defer func() {
			cloud = oldCloud
		}()
		ctrl := gomock.NewController(t)
		m := NewMockcloudProvider(ctrl)
		m.EXPECT().IsAWS().Return(false)
		cloud = m
		gl := NewGatewayLookup(testRootNSFunc, 100)
		require.Nil(t, gl)
	})

	t.Run("gateway lookup enabled, aws metadata endpoint not enabled", func(t *testing.T) {
		oldCloud := cloud
		defer func() {
			cloud = oldCloud
		}()
		ctrl := gomock.NewController(t)
		m := NewMockcloudProvider(ctrl)
		m.EXPECT().IsAWS().Return(true)
		cloud = m

		clouds := ddconfig.Datadog.Get("cloud_provider_metadata")
		ddconfig.Datadog.SetWithoutSource("cloud_provider_metadata", []string{})
		defer ddconfig.Datadog.SetWithoutSource("cloud_provider_metadata", clouds)

		gl := NewGatewayLookup(testRootNSFunc, 100)
		require.Nil(t, gl)
	})
}

func (s *gatewayLookupSuite) TestGatewayLookupEnabled() {
	t := s.T()
	ctrl := gomock.NewController(t)
	m := NewMockcloudProvider(ctrl)
	oldCloud := cloud
	defer func() {
		cloud = oldCloud
	}()

	m.EXPECT().IsAWS().Return(true)
	cloud = m

	dnsAddr := net.ParseIP("8.8.8.8")
	ifi := ipRouteGet(t, "", dnsAddr.String(), nil)
	ifs, err := net.Interfaces()
	require.NoError(t, err)

	gli := NewGatewayLookup(testRootNSFunc, 100)
	require.NotNil(t, gli)
	t.Cleanup(gli.Close)
	gl, ok := gli.(*gatewayLookup)
	require.True(t, ok)

	gl.subnetForHwAddrFunc = func(hwAddr net.HardwareAddr) (Subnet, error) {
		t.Logf("subnet lookup: %s", hwAddr)
		for _, i := range ifs {
			if hwAddr.String() == i.HardwareAddr.String() {
				return Subnet{Alias: fmt.Sprintf("subnet-%d", i.Index)}, nil
			}
		}

		return Subnet{Alias: "subnet"}, nil
	}

	var clientIP string
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		clientIP, _, _, err = testdns.SendDNSQueries(t, []string{"google.com"}, dnsAddr, "udp")
		assert.NoError(c, err)
	}, 6*time.Second, 100*time.Millisecond, "failed to send dns query")

	cs := &ConnectionStats{
		Source: util.AddressFromNetIP(net.ParseIP(clientIP)),
		Dest:   util.AddressFromNetIP(dnsAddr),
	}

	via := gli.Lookup(cs)

	require.NotNil(t, via, "connection is missing via: %s", cs)
	require.Equal(t, via.Subnet.Alias, fmt.Sprintf("subnet-%d", ifi.Index))
}

func (s *gatewayLookupSuite) TestGatewayLookupSubnetLookupError() {
	t := s.T()
	ctrl := gomock.NewController(t)
	m := NewMockcloudProvider(ctrl)
	oldCloud := cloud
	defer func() {
		cloud = oldCloud
	}()

	m.EXPECT().IsAWS().Return(true)
	cloud = m

	destAddr := net.ParseIP("8.8.8.8")
	destDomain := "google.com"
	// create the tracer without starting it
	gli := NewGatewayLookup(testRootNSFunc, 100)
	require.NotNil(t, gli)
	t.Cleanup(gli.Close)
	gl, ok := gli.(*gatewayLookup)
	require.True(t, ok)

	ifi := ipRouteGet(t, "", destAddr.String(), nil)
	calls := 0
	gl.subnetForHwAddrFunc = func(hwAddr net.HardwareAddr) (Subnet, error) {
		if hwAddr.String() == ifi.HardwareAddr.String() {
			calls++
		}
		return Subnet{}, assert.AnError
	}

	gl.purge()

	var clientIP string
	var err error
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		clientIP, _, _, err = testdns.SendDNSQueries(t, []string{destDomain}, destAddr, "udp")
		assert.NoError(c, err)
	}, 6*time.Second, 100*time.Millisecond, "failed to send dns query")

	cs := &ConnectionStats{
		Source: util.AddressFromNetIP(net.ParseIP(clientIP)),
		Dest:   util.AddressFromNetIP(destAddr),
	}

	via := gli.Lookup(cs)
	require.Nil(t, via)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		clientIP, _, _, err = testdns.SendDNSQueries(t, []string{destDomain}, destAddr, "udp")
		assert.NoError(c, err)
	}, 6*time.Second, 100*time.Millisecond, "failed to send dns query")

	via = gli.Lookup(cs)
	require.Nil(t, via)

	require.Equal(t, 1, calls, "calls to subnetForHwAddrFunc are != 1 for hw addr %s", ifi.HardwareAddr)
}

func (s *gatewayLookupSuite) TestGatewayLookupCrossNamespace() {
	t := s.T()
	ctrl := gomock.NewController(t)
	m := NewMockcloudProvider(ctrl)
	oldCloud := cloud
	defer func() {
		cloud = oldCloud
	}()

	m.EXPECT().IsAWS().Return(true)
	cloud = m

	gli := NewGatewayLookup(testRootNSFunc, 100)
	require.NotNil(t, gli)
	t.Cleanup(gli.Close)
	gl, ok := gli.(*gatewayLookup)
	require.True(t, ok)

	ns1 := netlinktestutil.AddNS(t)
	ns2 := netlinktestutil.AddNS(t)

	// setup two network namespaces
	t.Cleanup(func() {
		testutil.RunCommands(t, []string{
			"ip link del veth1",
			"ip link del veth3",
			"ip link del br0",
		}, true)
	})
	testutil.IptablesSave(t)
	cmds := []string{
		"ip link add br0 type bridge",
		"ip addr add 2.2.2.1/24 broadcast 2.2.2.255 dev br0",
		"ip link add veth1 type veth peer name veth2",
		"ip link set veth1 master br0",
		fmt.Sprintf("ip link set veth2 netns %s", ns1),
		fmt.Sprintf("ip -n %s addr add 2.2.2.2/24 broadcast 2.2.2.255 dev veth2", ns1),
		"ip link add veth3 type veth peer name veth4",
		"ip link set veth3 master br0",
		fmt.Sprintf("ip link set veth4 netns %s", ns2),
		fmt.Sprintf("ip -n %s addr add 2.2.2.3/24 broadcast 2.2.2.255 dev veth4", ns2),
		"ip link set br0 up",
		"ip link set veth1 up",
		fmt.Sprintf("ip -n %s link set veth2 up", ns1),
		"ip link set veth3 up",
		fmt.Sprintf("ip -n %s link set veth4 up", ns2),
		fmt.Sprintf("ip -n %s r add default via 2.2.2.1", ns1),
		fmt.Sprintf("ip -n %s r add default via 2.2.2.1", ns2),
		"iptables -I POSTROUTING 1 -t nat -s 2.2.2.0/24 ! -d 2.2.2.0/24 -j MASQUERADE",
		"iptables -I FORWARD -i br0 -j ACCEPT",
		"iptables -I FORWARD -o br0 -j ACCEPT",
		"sysctl -w net.ipv4.ip_forward=1",
	}
	testutil.RunCommands(t, cmds, false)

	ifs, err := net.Interfaces()
	require.NoError(t, err)
	gl.subnetForHwAddrFunc = func(hwAddr net.HardwareAddr) (Subnet, error) {
		for _, i := range ifs {
			if hwAddr.String() == i.HardwareAddr.String() {
				return Subnet{Alias: fmt.Sprintf("subnet-%s", i.Name)}, nil
			}
		}

		return Subnet{Alias: "subnet"}, nil
	}

	test1Ns, err := netns.GetFromName(ns1)
	require.NoError(t, err)
	defer test1Ns.Close()

	ns1Ino, err := kernel.GetInoForNs(test1Ns)
	require.NoError(t, err)

	// run tcp server in test1 net namespace
	var server *TCPServer
	err = kernel.WithNS(test1Ns, func() error {
		server = NewTCPServerOnAddress("2.2.2.2:0", func(c net.Conn) {})
		return server.Run()
	})
	require.NoError(t, err)
	t.Cleanup(server.Shutdown)

	t.Run("client in root namespace", func(t *testing.T) {
		c, err := net.DialTimeout("tcp", server.address, 2*time.Second)
		require.NoError(t, err)

		// write some data
		_, err = c.Write([]byte("foo"))
		require.NoError(t, err)

		conn := &ConnectionStats{
			Source: util.AddressFromString(c.LocalAddr().String()),
			Dest:   util.AddressFromString(c.RemoteAddr().String()),
			NetNS:  ns1Ino,
		}

		via := gli.Lookup(conn)

		// via should be nil, since traffic is local
		require.Nil(t, via)
	})

	t.Run("client in other namespace", func(t *testing.T) {
		// try connecting to server in test1 namespace
		test2Ns, err := netns.GetFromName(ns2)
		require.NoError(t, err)
		defer test2Ns.Close()

		var c net.Conn
		err = kernel.WithNS(test2Ns, func() error {
			var err error
			c, err = net.DialTimeout("tcp", server.address, 2*time.Second)
			return err
		})
		require.NoError(t, err)
		defer c.Close()

		// write some data
		_, err = c.Write([]byte("foo"))
		require.NoError(t, err)

		// get namespace inode
		ns2Ino, err := kernel.GetInoForNs(test2Ns)
		require.NoError(t, err)

		// create a connection
		conn := &ConnectionStats{
			Source: util.AddressFromString(c.LocalAddr().String()),
			Dest:   util.AddressFromString(c.RemoteAddr().String()),
			NetNS:  ns2Ino,
		}

		t.Logf("conn1: %+v", conn)

		via := gl.Lookup(conn)

		// traffic is local, so Via field should not be set
		require.Nil(t, via)

		// try connecting to something outside
		dnsAddr := net.ParseIP("8.8.8.8")
		var clientIP string
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			kernel.WithNS(test2Ns, func() error {
				clientIP, _, _, err = testdns.SendDNSQueries(t, []string{"google.com"}, dnsAddr, "udp")
				return nil
			})
			assert.NoError(c, err)
		}, 6*time.Second, 100*time.Millisecond, "failed to send dns query")

		iif := ipRouteGet(t, "", clientIP, nil)
		ifi := ipRouteGet(t, dnsAddr.String(), dnsAddr.String(), iif)

		conn = &ConnectionStats{
			Source: util.AddressFromString(clientIP),
			Dest:   util.AddressFromString(dnsAddr.String()),
			NetNS:  ns2Ino,
		}

		t.Logf("conn2: %+v", conn)
		t.Log("starting execution!!!")
		via = gl.Lookup(conn)

		require.NotNil(t, via)
		require.Equal(t, fmt.Sprintf("subnet-%s", ifi.Name), via.Subnet.Alias)

	})
}

func ipRouteGet(t *testing.T, from, dest string, iif *net.Interface) *net.Interface {
	ipRouteGetOut := regexp.MustCompile(`dev\s+([^\s/]+)`)

	args := []string{"route", "get"}
	if len(from) > 0 {
		args = append(args, "from", from)
	}
	args = append(args, dest)
	if iif != nil {
		args = append(args, "iif", iif.Name)
	}
	cmd := exec.Command("ip", args...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "ip command returned error, output: %s", out)
	t.Log(strings.Join(cmd.Args, " "))
	t.Log(string(out))

	matches := ipRouteGetOut.FindSubmatch(out)
	require.Len(t, matches, 2, string(out))
	dev := string(matches[1])
	ifi, err := net.InterfaceByName(dev)
	require.NoError(t, err)
	return ifi
}

func testRootNSFunc() (netns.NsHandle, error) {
	return netns.GetFromPid(os.Getpid())
}

type TCPServer struct {
	address   string
	network   string
	onMessage func(c net.Conn)
	ln        net.Listener
}

func NewTCPServer(onMessage func(c net.Conn)) *TCPServer {
	return NewTCPServerOnAddress("127.0.0.1:0", onMessage)
}

func NewTCPServerOnAddress(addr string, onMessage func(c net.Conn)) *TCPServer {
	return &TCPServer{
		address:   addr,
		onMessage: onMessage,
	}
}

func (t *TCPServer) Run() error {
	networkType := "tcp"
	if t.network != "" {
		networkType = t.network
	}
	ln, err := net.Listen(networkType, t.address)
	if err != nil {
		return err
	}
	t.ln = ln
	t.address = ln.Addr().String()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go t.onMessage(conn)
		}
	}()

	return nil
}

func (t *TCPServer) Shutdown() {
	if t.ln != nil {
		_ = t.ln.Close()
		t.ln = nil
	}
}
