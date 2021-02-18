package testutil

import (
	"net"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	natPort    = 5432
	nonNatPort = 9876
)

func TestConntracker(t *testing.T, serverIP, clientIP net.IP, ct netlink.Conntracker) {
	srv1 := StartServerTCP(t, serverIP, natPort)
	defer srv1.Close()
	srv2 := StartServerTCP(t, serverIP, nonNatPort)
	defer srv2.Close()
	srv3 := StartServerUDP(t, serverIP, natPort)
	defer srv3.Close()

	localAddr := PingTCP(t, clientIP, natPort).LocalAddr().(*net.TCPAddr)
	time.Sleep(1 * time.Second)

	netns, err := util.GetCurrentIno()
	require.NoError(t, err)

	trans := ct.GetTranslationForConn(
		network.ConnectionStats{
			Source: util.AddressFromNetIP(localAddr.IP),
			SPort:  uint16(localAddr.Port),
			Dest:   util.AddressFromNetIP(clientIP),
			DPort:  uint16(natPort),
			Type:   network.TCP,
			NetNS:  uint32(netns),
		},
	)
	require.NotNil(t, trans)
	assert.Equal(t, util.AddressFromNetIP(serverIP), trans.ReplSrcIP)

	localAddrUDP := PingUDP(t, clientIP, natPort).LocalAddr().(*net.UDPAddr)
	time.Sleep(time.Second)
	trans = ct.GetTranslationForConn(
		network.ConnectionStats{
			Source: util.AddressFromNetIP(localAddrUDP.IP),
			SPort:  uint16(localAddrUDP.Port),
			Dest:   util.AddressFromNetIP(clientIP),
			DPort:  uint16(natPort),
			Type:   network.UDP,
			NetNS:  uint32(netns),
		},
	)
	require.NotNil(t, trans)
	assert.Equal(t, util.AddressFromNetIP(serverIP), trans.ReplSrcIP)

	// now dial TCP directly
	localAddr = PingTCP(t, serverIP, nonNatPort).LocalAddr().(*net.TCPAddr)
	time.Sleep(time.Second)

	trans = ct.GetTranslationForConn(
		network.ConnectionStats{
			Source: util.AddressFromNetIP(localAddr.IP),
			SPort:  uint16(localAddr.Port),
			Dest:   util.AddressFromNetIP(serverIP),
			DPort:  uint16(nonNatPort),
			Type:   network.TCP,
			NetNS:  uint32(netns),
		},
	)
	assert.Nil(t, trans)
}
