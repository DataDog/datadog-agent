// +build linux_bpf
// +build !android

package netlink

import (
	"fmt"
	"net"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	ct "github.com/florianl/go-conntrack"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

func TestConntrackExists(t *testing.T) {
	defer testutil.TeardownCrossNsDNAT(t)
	testutil.SetupCrossNsDNAT(t)

	tcpCloser := nettestutil.StartServerTCPNs(t, net.ParseIP("2.2.2.4"), 8080, "test")
	defer tcpCloser.Close()

	udpCloser := nettestutil.StartServerUDPNs(t, net.ParseIP("2.2.2.4"), 8080, "test")
	defer udpCloser.Close()

	tcpConn := nettestutil.PingTCP(t, net.ParseIP("2.2.2.4"), 80)
	defer tcpConn.Close()

	udpConn := nettestutil.PingUDP(t, net.ParseIP("2.2.2.4"), 80)
	defer udpConn.Close()

	testNs, err := netns.GetFromName("test")
	require.NoError(t, err)
	defer testNs.Close()

	ctrks := map[int]Conntrack{}
	defer func() {
		for _, ctrk := range ctrks {
			ctrk.Close()
		}
	}()

	tcpLaddr := tcpConn.LocalAddr().(*net.TCPAddr)
	udpLaddr := udpConn.LocalAddr().(*net.UDPAddr)
	// test a combination of (tcp, udp) x (root ns, test ns)
	testConntrackExists(t, tcpLaddr.IP.String(), tcpLaddr.Port, "tcp", testNs, ctrks)
	testConntrackExists(t, udpLaddr.IP.String(), udpLaddr.Port, "udp", testNs, ctrks)
}

func testConntrackExists(t *testing.T, laddrIP string, laddrPort int, proto string, testNs netns.NsHandle, ctrks map[int]Conntrack) {
	rootNs, err := util.GetRootNetNamespace("/proc")
	require.NoError(t, err)
	defer rootNs.Close()

	var ipProto uint8 = unix.IPPROTO_UDP
	if proto == "tcp" {
		ipProto = unix.IPPROTO_TCP
	}
	tests := []struct {
		desc   string
		c      Con
		exists bool
		ns     int
	}{
		{
			desc: fmt.Sprintf("net ns 0, origin exists, proto %s", proto),
			c: Con{
				Con: ct.Con{
					Origin: newIPTuple(laddrIP, "2.2.2.4", uint16(laddrPort), 80, ipProto),
				},
			},
			exists: true,
			ns:     int(rootNs),
		},
		{
			desc: fmt.Sprintf("net ns 0, reply exists, proto %s", proto),
			c: Con{
				Con: ct.Con{
					Reply: newIPTuple("2.2.2.4", laddrIP, 80, uint16(laddrPort), ipProto),
				},
			},
			exists: true,
			ns:     int(rootNs),
		},
		{
			desc: fmt.Sprintf("net ns 0, origin does not exist, proto %s", proto),
			c: Con{
				Con: ct.Con{
					Origin: newIPTuple(laddrIP, "2.2.2.3", uint16(laddrPort), 80, ipProto),
				},
			},
			exists: false,
			ns:     int(rootNs),
		},
		{
			desc: fmt.Sprintf("net ns %d, origin exists, proto %s", int(testNs), proto),
			c: Con{
				Con: ct.Con{
					Origin: newIPTuple(laddrIP, "2.2.2.4", uint16(laddrPort), 80, ipProto),
				},
			},
			exists: true,
			ns:     int(testNs),
		},
		{
			desc: fmt.Sprintf("net ns %d, reply exists, proto %s", int(testNs), proto),
			c: Con{
				Con: ct.Con{
					Reply: newIPTuple("2.2.2.4", laddrIP, 8080, uint16(laddrPort), ipProto),
				},
			},
			exists: true,
			ns:     int(testNs),
		},
		{
			desc: fmt.Sprintf("net ns %d, origin does not exist, proto %s", int(testNs), proto),
			c: Con{
				Con: ct.Con{
					Origin: newIPTuple(laddrIP, "2.2.2.3", uint16(laddrPort), 80, ipProto),
				},
			},
			exists: false,
			ns:     int(testNs),
		},
	}

	for _, te := range tests {
		t.Run(te.desc, func(t *testing.T) {
			ctrk, ok := ctrks[te.ns]
			if !ok {
				var err error
				ctrk, err = NewConntrack(te.ns)
				require.NoError(t, err)

				ctrks[te.ns] = ctrk
			}

			ok, err := ctrk.Exists(&te.c)
			require.NoError(t, err)
			require.Equal(t, te.exists, ok)
		})
	}
}
