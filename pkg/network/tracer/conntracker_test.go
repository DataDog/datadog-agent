// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"net"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"
	"go4.org/netipx"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	netlinktestutil "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	netnsutil "github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
)

func TestConntrackers(t *testing.T) {
	ebpftest.LogLevel(t, "trace")
	t.Run("netlink", func(t *testing.T) {
		runConntrackerTest(t, "netlink", setupNetlinkConntracker)
	})
	t.Run("eBPF", func(t *testing.T) {
		modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled}
		if ebpfCOREConntrackerSupportedOnKernelT(t) {
			modes = append([]ebpftest.BuildMode{ebpftest.CORE}, modes...)
		}
		if !prebuilt.IsDeprecated() && ebpfPrebuiltConntrackerSupportedOnKernelT(t) {
			modes = append([]ebpftest.BuildMode{ebpftest.Prebuilt}, modes...)
		}
		ebpftest.TestBuildModes(t, modes, "", func(t *testing.T) {
			runConntrackerTest(t, "eBPF", setupEBPFConntracker)
		})
	})

	// these tests exercise the __nf_conntrack_confirm alternate probes via normal packet flow
	t.Run("eBPFAlternateProbesNFConntrackConfirm", func(t *testing.T) {
		runAlternateProbesTest(t, func(t *testing.T) {
			runConntrackerTest(t, "eBPFAlternateProbesNFConntrackConfirm", setupEBPFConntracker)
		})
	})

	// these tests exercise the nf_conntrack_hash_check_insert alternate probes via the ctnetlink
	// interface using the conntrack -I CLI
	t.Run("eBPFAlternateProbesNFConntrackHashCheckInsert", func(t *testing.T) {
		if _, err := exec.LookPath("conntrack"); err != nil {
			t.Skip("conntrack CLI not available")
		}

		runAlternateProbesTest(t, func(t *testing.T) {
			runNFConntrackHashCheckInsertTest(t, setupEBPFConntracker)
		})
	})
}

func runConntrackerTest(t *testing.T, name string, createFn func(*testing.T, *config.Config) (netlink.Conntracker, error)) {
	t.Run("IPv4", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := config.New()
		ct, err := createFn(t, cfg)
		require.NoError(t, err)
		defer ct.Close()

		netlinktestutil.SetupDNAT(t)

		testConntracker(t, net.ParseIP("1.1.1.1"), net.ParseIP("2.2.2.2"), ct, cfg)
	})
	t.Run("IPv6", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := config.New()
		ct, err := createFn(t, cfg)
		require.NoError(t, err)
		defer ct.Close()

		netlinktestutil.SetupDNAT6(t)

		testConntracker(t, net.ParseIP("fd00::1"), net.ParseIP("fd00::2"), ct, cfg)
	})
	t.Run("cross namespace - NAT rule on test namespace", func(t *testing.T) {
		mock.NewSystemProbe(t)
		if name == "netlink" {
			if kv >= kernel.VersionCode(5, 19, 0) && kv < kernel.VersionCode(6, 3, 0) {
				// see https://lore.kernel.org/netfilter-devel/CALvGib_xHOVD2+6tKm2Sf0wVkQwut2_z2gksZPcGw30tOvOAAA@mail.gmail.com/T/#u
				t.Skip("skip due to a kernel bug with conntrack netlink events flowing across namespaces")
			}
		}

		cfg := config.New()
		cfg.EnableConntrackAllNamespaces = true
		ct, err := createFn(t, cfg)
		require.NoError(t, err)
		defer ct.Close()

		testConntrackerCrossNamespace(t, ct)
	})
	t.Run("cross namespace - NAT rule on root namespace", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := config.New()
		cfg.EnableConntrackAllNamespaces = true
		ct, err := createFn(t, cfg)
		require.NoError(t, err)
		defer ct.Close()

		testConntrackerCrossNamespaceNATonRoot(t, ct)
	})
}

func runAlternateProbesTest(t *testing.T, testFn func(t *testing.T)) {
	supportedOnKernel, err := ebpfConntrackerAlternateProbesSupportedOnKernel()
	if err != nil || !supportedOnKernel {
		t.Skip("alternate probes for conntracker not supported")
	}

	origVerifyKernelFuncs := verifyKernelFuncs
	t.Cleanup(func() {
		verifyKernelFuncs = origVerifyKernelFuncs
	})

	// mock verifyKernelFuncs to report __nf_conntrack_hash_insert as missing to force the use of alternate probes
	verifyKernelFuncs = func(requiredKernelFuncs ...string) (map[string]struct{}, error) {
		missing := make(map[string]struct{})
		for _, fn := range requiredKernelFuncs {
			if fn == "__nf_conntrack_hash_insert" {
				missing[fn] = struct{}{}
			}
		}
		return missing, nil
	}

	// only test CO-RE and RuntimeCompiled modes since Prebuilt doesn't support alternate probes
	modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled}
	if ebpfCOREConntrackerSupportedOnKernelT(t) {
		modes = append([]ebpftest.BuildMode{ebpftest.CORE}, modes...)
	}
	ebpftest.TestBuildModes(t, modes, "", testFn)
}

func runNFConntrackHashCheckInsertTest(t *testing.T, createFn func(*testing.T, *config.Config) (netlink.Conntracker, error)) {
	t.Run("IPv4", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := config.New()
		ct, err := createFn(t, cfg)
		require.NoError(t, err)
		defer ct.Close()

		netlinktestutil.SetupDNAT(t)

		testNFConntrackHashCheckInsert(t, "10.0.0.100", "2.2.2.2", "1.1.1.1", network.AFINET, ct, cfg)
	})
	t.Run("IPv6", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := config.New()
		ct, err := createFn(t, cfg)
		require.NoError(t, err)
		defer ct.Close()

		netlinktestutil.SetupDNAT6(t)

		testNFConntrackHashCheckInsert(t, "fd00::100", "fd00::2", "fd00::1", network.AFINET6, ct, cfg)
	})
	t.Run("cross namespace - NAT rule on test namespace", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := config.New()
		cfg.EnableConntrackAllNamespaces = true
		ct, err := createFn(t, cfg)
		require.NoError(t, err)
		defer ct.Close()

		testNFConntrackHashCheckInsertCrossNamespace(t, ct)
	})
	t.Run("cross namespace - NAT rule on root namespace", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := config.New()
		cfg.EnableConntrackAllNamespaces = true
		ct, err := createFn(t, cfg)
		require.NoError(t, err)
		defer ct.Close()

		testNFConntrackHashCheckInsertCrossNamespaceNATonRoot(t, ct)
	})
}

func testNFConntrackHashCheckInsert(t *testing.T, srcIP, dstIP, replySrcIP string, family network.ConnectionFamily, ct netlink.Conntracker, cfg *config.Config) {
	curNs, err := netnsutil.GetCurrentIno()
	require.NoError(t, err)

	t.Run("TCP", func(t *testing.T) {
		mock.NewSystemProbe(t)
		srcPort := 54320
		dstPort := 8080

		netlinktestutil.CreateConntrackEntry(t, srcIP, dstIP, srcPort, dstPort, replySrcIP, srcIP, dstPort, srcPort, "tcp")

		var trans *network.IPTranslation
		cs := network.ConnectionTuple{
			Source: util.AddressFromString(srcIP),
			SPort:  uint16(srcPort),
			Dest:   util.AddressFromString(dstIP),
			DPort:  uint16(dstPort),
			Type:   network.TCP,
			Family: family,
			NetNS:  curNs,
		}
		require.Eventually(t, func() bool {
			trans = ct.GetTranslationForConn(&cs)
			return trans != nil
		}, 5*time.Second, 100*time.Millisecond, "timed out waiting for TCP conntrack entry created via ctnetlink for %s", cs.String())

		assert.Equal(t, util.AddressFromString(replySrcIP), trans.ReplSrcIP,
			"expected NAT translation to %s but got %s", replySrcIP, trans.ReplSrcIP)
	})

	t.Run("UDP", func(t *testing.T) {
		mock.NewSystemProbe(t)
		if family == network.AFINET6 && !cfg.CollectUDPv6Conns {
			t.Skip("UDPv6 disabled")
		}

		srcPort := 54321
		dstPort := 8081

		netlinktestutil.CreateConntrackEntry(t, srcIP, dstIP, srcPort, dstPort, replySrcIP, srcIP, dstPort, srcPort, "udp")

		var trans *network.IPTranslation
		cs := network.ConnectionTuple{
			Source: util.AddressFromString(srcIP),
			SPort:  uint16(srcPort),
			Dest:   util.AddressFromString(dstIP),
			DPort:  uint16(dstPort),
			Type:   network.UDP,
			Family: family,
			NetNS:  curNs,
		}
		require.Eventually(t, func() bool {
			trans = ct.GetTranslationForConn(&cs)
			return trans != nil
		}, 5*time.Second, 100*time.Millisecond, "timed out waiting for UDP conntrack entry created via ctnetlink for %s", cs.String())

		assert.Equal(t, util.AddressFromString(replySrcIP), trans.ReplSrcIP,
			"expected NAT translation to %s but got %s", replySrcIP, trans.ReplSrcIP)
	})
}

func setupEBPFConntracker(_ *testing.T, cfg *config.Config) (netlink.Conntracker, error) {
	return NewEBPFConntracker(cfg, nil)
}

func setupNetlinkConntracker(_ *testing.T, cfg *config.Config) (netlink.Conntracker, error) {
	cfg.ConntrackMaxStateSize = 100
	cfg.ConntrackRateLimit = 500
	ct, err := netlink.NewConntracker(cfg, nil)
	time.Sleep(100 * time.Millisecond)
	return ct, err
}

func getPort(t *testing.T, listener net.Listener) uint16 {
	addr := listener.Addr()
	listenerURL := url.URL{Scheme: addr.Network(), Host: addr.String()}
	port, err := strconv.Atoi(listenerURL.Port())
	require.NoError(t, err)
	return uint16(port)
}

func getPortUDP(_ *testing.T, udpConn *net.UDPConn) uint16 {
	return uint16(udpConn.LocalAddr().(*net.UDPAddr).Port)
}

func testConntracker(t *testing.T, serverIP, clientIP net.IP, ct netlink.Conntracker, cfg *config.Config) {
	isIPv6 := false
	if cip, _ := netipx.FromStdIP(clientIP); cip.Is6() {
		isIPv6 = true
	}

	family := network.AFINET
	if isIPv6 {
		family = network.AFINET6
	}

	curNs, err := netnsutil.GetCurrentIno()
	require.NoError(t, err)
	t.Logf("ns: %d", curNs)

	t.Run("TCP", func(t *testing.T) {
		var natPort, nonNatPort int
		srv1 := nettestutil.StartServerTCP(t, serverIP, natPort)
		defer srv1.Close()
		natPort = int(getPort(t, srv1.(net.Listener)))
		srv2 := nettestutil.StartServerTCP(t, serverIP, nonNatPort)
		defer srv2.Close()
		nonNatPort = int(getPort(t, srv2.(net.Listener)))

		localAddr := nettestutil.MustPingTCP(t, clientIP, natPort).LocalAddr().(*net.TCPAddr)
		var trans *network.IPTranslation
		cs := network.ConnectionTuple{
			Source: util.AddressFromNetIP(localAddr.IP),
			SPort:  uint16(localAddr.Port),
			Dest:   util.AddressFromNetIP(clientIP),
			DPort:  uint16(natPort),
			Type:   network.TCP,
			Family: family,
			NetNS:  curNs,
		}
		require.Eventually(t, func() bool {
			trans = ct.GetTranslationForConn(&cs)
			return trans != nil
		}, 5*time.Second, 100*time.Millisecond, "timed out waiting for TCP NAT conntrack entry for %s", cs.String())
		assert.Equal(t, util.AddressFromNetIP(serverIP), trans.ReplSrcIP)

		// now dial TCP directly
		localAddr = nettestutil.MustPingTCP(t, serverIP, nonNatPort).LocalAddr().(*net.TCPAddr)

		cs = network.ConnectionTuple{
			Source: util.AddressFromNetIP(localAddr.IP),
			SPort:  uint16(localAddr.Port),
			Dest:   util.AddressFromNetIP(serverIP),
			DPort:  uint16(nonNatPort),
			Type:   network.TCP,
			NetNS:  curNs,
		}
		trans = ct.GetTranslationForConn(&cs)
		assert.Nil(t, trans)
	})

	t.Run("UDP", func(t *testing.T) {
		if isIPv6 && !cfg.CollectUDPv6Conns {
			t.Skip("UDPv6 disabled")
		}

		var natPort int
		srv3 := nettestutil.StartServerUDP(t, serverIP, natPort)
		defer srv3.Close()
		natPort = int(getPortUDP(t, srv3.(*net.UDPConn)))

		localAddrUDP := nettestutil.MustPingUDP(t, clientIP, natPort).LocalAddr().(*net.UDPAddr)
		var trans *network.IPTranslation
		cs := network.ConnectionTuple{
			Source: util.AddressFromNetIP(localAddrUDP.IP),
			SPort:  uint16(localAddrUDP.Port),
			Dest:   util.AddressFromNetIP(clientIP),
			DPort:  uint16(natPort),
			Type:   network.UDP,
			Family: family,
			NetNS:  curNs,
		}
		require.Eventually(t, func() bool {
			trans = ct.GetTranslationForConn(&cs)
			return trans != nil
		}, 5*time.Second, 100*time.Millisecond, "timed out waiting for UDP NAT conntrack entry for %s", cs.String())
		assert.Equal(t, util.AddressFromNetIP(serverIP), trans.ReplSrcIP)
	})
}

func testConntrackerCrossNamespace(t *testing.T, ct netlink.Conntracker) {
	ns := netlinktestutil.SetupCrossNsDNAT(t)

	closer := nettestutil.StartServerTCPNs(t, net.ParseIP("2.2.2.4"), 8080, ns)
	laddr := nettestutil.MustPingTCP(t, net.ParseIP("2.2.2.4"), 80).LocalAddr().(*net.TCPAddr)
	defer closer.Close()

	testNs, err := netns.GetFromName(ns)
	require.NoError(t, err)
	defer testNs.Close()
	testIno, err := netnsutil.GetInoForNs(testNs)
	require.NoError(t, err)
	t.Logf("test ns: %d", testIno)

	var trans *network.IPTranslation
	cs := network.ConnectionTuple{
		Source: util.AddressFromNetIP(laddr.IP),
		SPort:  uint16(laddr.Port),
		Dest:   util.AddressFromString("2.2.2.4"),
		DPort:  uint16(80),
		Type:   network.TCP,
		NetNS:  testIno,
	}
	require.Eventually(t, func() bool {
		trans = ct.GetTranslationForConn(&cs)
		return trans != nil
	}, 5*time.Second, 100*time.Millisecond, "timed out waiting for conntrack entry for %s", cs.String())

	assert.Equal(t, uint16(8080), trans.ReplSrcPort)
}

func testConntrackerCrossNamespaceNATonRoot(t *testing.T, ct netlink.Conntracker) {
	ns := netlinktestutil.SetupVethPair(t)

	// SetupDNAT sets up a NAT translation from 3.3.3.3 to 1.1.1.1
	netlinktestutil.SetupDNAT(t)

	// Setup TCP server on root namespace
	srv := nettestutil.StartServerTCP(t, net.ParseIP("1.1.1.1"), 0)
	defer srv.Close()

	port := srv.(net.Listener).Addr().(*net.TCPAddr).Port

	// Now switch to the test namespace and make a request to the root namespace server
	var laddr *net.TCPAddr
	var testIno uint32
	done := make(chan struct{})
	go func() {
		var err error
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		originalNS, _ := netns.Get()
		defer originalNS.Close()

		testNS, err := netns.GetFromName(ns)
		require.NoError(t, err)

		testIno, err = netnsutil.GetInoForNs(testNS)
		require.NoError(t, err)

		defer netns.Set(originalNS)
		defer close(done)
		netns.Set(testNS)
		laddr = nettestutil.MustPingTCP(t, net.ParseIP("3.3.3.3"), port).LocalAddr().(*net.TCPAddr)
	}()
	<-done

	require.NotNil(t, laddr)

	var trans *network.IPTranslation
	cs := network.ConnectionTuple{
		Source: util.AddressFromNetIP(laddr.IP),
		SPort:  uint16(laddr.Port),
		Dest:   util.AddressFromString("3.3.3.3"),
		DPort:  uint16(port),
		Type:   network.TCP,
		NetNS:  testIno,
	}
	require.Eventually(t, func() bool {
		trans = ct.GetTranslationForConn(&cs)
		return trans != nil
	}, 5*time.Second, 100*time.Millisecond, "timed out waiting for conntrack entry for %s", cs.String())

	assert.Equal(t, util.AddressFromString("1.1.1.1"), trans.ReplSrcIP)
}

func testNFConntrackHashCheckInsertCrossNamespace(t *testing.T, ct netlink.Conntracker) {
	ns := netlinktestutil.SetupCrossNsDNAT(t)

	testNs, err := netns.GetFromName(ns)
	require.NoError(t, err)
	defer testNs.Close()
	testIno, err := netnsutil.GetInoForNs(testNs)
	require.NoError(t, err)

	// Create a conntrack entry in the test namespace via CLI
	// The NAT rule translates 2.2.2.4:80 -> 2.2.2.4:8080
	srcIP := "10.0.0.50"
	dstIP := "2.2.2.4"
	srcPort := 54400
	dstPort := 80
	replySrcPort := 8080

	// Create conntrack entry in the test namespace
	err = netnsutil.WithNS(testNs, func() error {
		netlinktestutil.CreateConntrackEntry(t, srcIP, dstIP, srcPort, dstPort, dstIP, srcIP, replySrcPort, srcPort, "tcp")
		return nil
	})
	require.NoError(t, err)

	var trans *network.IPTranslation
	cs := network.ConnectionTuple{
		Source: util.AddressFromString(srcIP),
		SPort:  uint16(srcPort),
		Dest:   util.AddressFromString(dstIP),
		DPort:  uint16(dstPort),
		Type:   network.TCP,
		NetNS:  testIno,
	}
	require.Eventually(t, func() bool {
		trans = ct.GetTranslationForConn(&cs)
		return trans != nil
	}, 5*time.Second, 100*time.Millisecond, "timed out waiting for conntrack entry for %s", cs.String())

	assert.Equal(t, uint16(replySrcPort), trans.ReplSrcPort)
}

func testNFConntrackHashCheckInsertCrossNamespaceNATonRoot(t *testing.T, ct netlink.Conntracker) {
	_ = netlinktestutil.SetupVethPair(t)

	// SetupDNAT sets up a NAT translation from 3.3.3.3 to 1.1.1.1
	netlinktestutil.SetupDNAT(t)

	curNs, err := netnsutil.GetCurrentIno()
	require.NoError(t, err)

	// Create a conntrack entry via CLI in the root namespace
	// simulating a connection from the test namespace through NAT
	srcIP := "2.2.2.3" // IP in the veth pair (root namespace side)
	srcPort := 54500
	dstIP := "3.3.3.3"
	dstPort := 8080
	replySrcIP := "1.1.1.1"

	netlinktestutil.CreateConntrackEntry(t, srcIP, dstIP, srcPort, dstPort, replySrcIP, srcIP, dstPort, srcPort, "tcp")

	var trans *network.IPTranslation
	cs := network.ConnectionTuple{
		Source: util.AddressFromString(srcIP),
		SPort:  uint16(srcPort),
		Dest:   util.AddressFromString(dstIP),
		DPort:  uint16(dstPort),
		Type:   network.TCP,
		NetNS:  curNs,
	}
	require.Eventually(t, func() bool {
		trans = ct.GetTranslationForConn(&cs)
		return trans != nil
	}, 5*time.Second, 100*time.Millisecond, "timed out waiting for conntrack entry for %s", cs.String())

	assert.Equal(t, util.AddressFromString(replySrcIP), trans.ReplSrcIP)
}

func TestCiliumConntrackerEnabledByDefault(t *testing.T) {
	cfg := config.New()
	assert.True(t, cfg.EnableCiliumLBConntracker)

	// most unit tests environments don't have cilium enabled (including CI)
	// so this should not load the cilium conntracker even if it enabled, without
	// any error
	clb, err := newCiliumLoadBalancerConntracker(cfg)
	assert.NoError(t, err)
	assert.Nil(t, clb)
}
