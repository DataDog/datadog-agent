// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// netstack setup code based on the tun_tcp_echo sample in gVisor
// Copyright 2018 The gVisor Authors.

//go:build linux_bpf

package tracer

import (
	"bytes"
	"fmt"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/link/fdbased"
	"gvisor.dev/gvisor/pkg/tcpip/link/tun"
	"gvisor.dev/gvisor/pkg/tcpip/network/arp"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/waiter"

	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func setupTap(t *testing.T) string {
	linkName := "tap1234"
	t.Cleanup(func() {
		cmds := []string{
			fmt.Sprintf("ip link del %s", linkName),
		}
		nettestutil.RunCommands(t, cmds, false)
	})

	cmds := []string{
		fmt.Sprintf("ip link del %s", linkName),
	}
	nettestutil.RunCommands(t, cmds, true)

	cmds = []string{
		fmt.Sprintf("ip tuntap add mode tap %s", linkName),
		fmt.Sprintf("ip link set %s up", linkName),
		fmt.Sprintf("ip address add 192.168.1.1/24 dev %s", linkName),
	}
	nettestutil.RunCommands(t, cmds, false)

	return linkName
}

func runConnection(t *testing.T, addrName string, port int) {
	tunName := setupTap(t)

	maddr, err := net.ParseMAC("aa:00:01:01:01:01")
	require.NoError(t, err)

	parsedAddr := net.ParseIP(addrName)
	require.NotNil(t, parsedAddr)

	addrWithPrefix := tcpip.AddrFromSlice(parsedAddr.To4()).WithPrefix()

	netStack := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, arp.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol},
	})

	fd, err := tun.OpenTAP(tunName)
	require.NoError(t, err)

	linkEP, err := fdbased.New(&fdbased.Options{
		FDs:            []int{fd},
		MTU:            1500,
		EthernetHeader: true,
		Address:        tcpip.LinkAddress(maddr),
	})
	require.NoError(t, err)

	tcpErr := netStack.CreateNIC(1, linkEP)
	require.Nil(t, tcpErr)

	protocolAddr := tcpip.ProtocolAddress{
		Protocol:          ipv4.ProtocolNumber,
		AddressWithPrefix: addrWithPrefix,
	}
	tcpErr = netStack.AddProtocolAddress(1, protocolAddr, stack.AddressProperties{})
	require.Nil(t, tcpErr)

	subnet, err := tcpip.NewSubnet(
		tcpip.AddrFromSlice(tcpip.IPv4Zero),
		tcpip.MaskFrom(string(tcpip.IPv4Zero)))
	require.NoError(t, err)

	netStack.SetRouteTable([]tcpip.Route{
		{
			Destination: subnet,
			NIC:         1,
		},
	})

	var wq waiter.Queue
	endpoint, tcpErr := netStack.NewEndpoint(tcp.ProtocolNumber, ipv4.ProtocolNumber, &wq)
	require.Nil(t, tcpErr)
	defer endpoint.Close()

	waitEntry, notifyCh := waiter.NewChannelEntry(waiter.ReadableEvents)
	wq.EventRegister(&waitEntry)
	defer wq.EventUnregister(&waitEntry)

	tcpErr = endpoint.Bind(tcpip.FullAddress{Port: uint16(port)})
	require.Nil(t, tcpErr)

	tcpErr = endpoint.Listen(1)
	require.Nil(t, tcpErr)

	tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%v:%v", addrName, port))
	require.NoError(t, err)

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	require.NoError(t, err)

	for {
		n, _, tcpErr := endpoint.Accept(nil)
		if tcpErr != nil {
			if _, ok := tcpErr.(*tcpip.ErrWouldBlock); ok {
				<-notifyCh
				continue
			}

			require.NotNil(t, tcpErr)
		}

		// Close the client connection to trigger tcp_close() and send a FIN.
		conn.Close()

		// TLSv1.2 Application Data
		tlsAppData := []byte{
			0x17, 0x03, 0x03, 0x00, 0x14, 0x39, 0xfc, 0xee, 0x2c, 0xbb, 0x79, 0x6f,
			0xdd, 0x9a, 0x0a, 0x44, 0xdd, 0x38, 0xc4, 0x4f, 0x50, 0x5a, 0xb6, 0xd2, 0xfe,
		}

		// Send data from the server after the client has closed the connection.
		_, tcpErr = n.Write(bytes.NewReader(tlsAppData), tcpip.WriteOptions{})
		require.Nil(t, tcpErr)

		n.Close()
		break
	}
}

// TestProtocolClassificationCleanup is a regression test for an issue that led
// to the connection_protocol map not being cleaned up properly in the case
// where a TLS Application Data packet from the server is seen in the socket
// filter after the client has closed the socket.
//
// The original issue could not be reproduced if both sides of the connection
// are on the same host (due to the check in delete_protocol_stack()), so run
// the server side in a user space TCP stack to avoid that side of the
// connection from being monitored by NPM's TCP hooks.
func (s *TracerSuite) TestProtocolClassificationCleanup() {
	t := s.T()
	cfg := testConfig()

	if !kprobe.ClassificationSupported(cfg) {
		t.Skip("protocol classification not supported")
	}

	tr := setupTracer(t, cfg)

	serverAddr := "192.168.1.2"
	port := 1000

	runConnection(t, serverAddr, port)

	connMap, err := tr.ebpfTracer.GetMap(probes.ConnectionProtocolMap)
	require.NoError(t, err)

	destAddr, err := netip.ParseAddr(serverAddr)
	require.NoError(t, err)
	addrLow, addrHigh := util.ToLowHighIP(destAddr)

	require.Eventually(t, func() bool {
		var key netebpf.ConnTuple
		value := make([]byte, connMap.ValueSize())
		mapEntries := connMap.Iterate()
		for mapEntries.Next(&key, &value) {
			if key.Daddr_h != addrHigh || key.Daddr_l != addrLow || key.Dport != uint16(port) {
				continue
			}

			t.Log(key)
			return false
		}

		return true
	}, 1*time.Second, 10*time.Millisecond, "connection not removed from map")
}
