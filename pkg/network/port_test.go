// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package network

import (
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"

	netlinktestutil "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var testRootNs uint32

func TestMain(m *testing.M) {
	rootNs, err := util.GetRootNetNamespace("/proc")
	if err != nil {
		log.Critical(err)
		os.Exit(1)
	}
	testRootNs, err = util.GetInoForNs(rootNs)
	if err != nil {
		log.Critical(err)
		os.Exit(1)
	}

	logLevel := os.Getenv("DD_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "warn"
	}
	log.SetupLogger(seelog.Default, logLevel)

	os.Exit(m.Run())
}

func TestReadInitialTCPState(t *testing.T) {
	nsName := netlinktestutil.AddNS(t)
	t.Cleanup(func() {
		err := exec.Command("testdata/teardown_netns.sh").Run()
		assert.NoError(t, err, "failed to teardown netns")
	})

	err := exec.Command("testdata/setup_netns.sh", nsName).Run()
	require.NoError(t, err, "setup_netns.sh failed")

	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()

	l6, err := net.Listen("tcp6", ":0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()

	ports := []uint16{
		getPort(t, l),
		getPort(t, l6),
		34567,
		34568,
	}

	ns, err := netns.GetFromName(nsName)
	require.NoError(t, err)
	defer ns.Close()

	nsIno, err := util.GetInoForNs(ns)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		initialPorts, err := ReadInitialState("/proc", TCP, true, true)
		require.NoError(t, err)
		for _, p := range ports[:2] {
			if _, ok := initialPorts[PortMapping{testRootNs, p}]; !ok {
				t.Errorf("PortMapping(testRootNs) returned false for port %d", p)
				return false
			}
		}
		for _, p := range ports[2:] {
			if _, ok := initialPorts[PortMapping{nsIno, p}]; !ok {
				t.Errorf("PortMapping(test ns) returned false for port %d", p)
				return false
			}
		}

		if _, ok := initialPorts[PortMapping{testRootNs, 999}]; ok {
			t.Errorf("expected PortMapping(testRootNs, 999) to not be in the map, but it was")
			return false
		}
		if _, ok := initialPorts[PortMapping{nsIno, 999}]; ok {
			t.Errorf("expected PortMapping(nsIno, 999) to not be in the map, but it was")
			return false
		}

		return true
	}, 3*time.Second, time.Second, "tcp/tcp6 ports are listening")
}

func TestReadInitialUDPState(t *testing.T) {
	nsName := netlinktestutil.AddNS(t)
	t.Cleanup(func() {
		err := exec.Command("testdata/teardown_netns.sh").Run()
		assert.NoError(t, err, "failed to teardown netns")
	})

	err := exec.Command("testdata/setup_netns.sh", nsName).Run()
	require.NoError(t, err, "setup_netns.sh failed")

	l, err := net.ListenUDP("udp", &net.UDPAddr{})
	require.NoError(t, err)
	defer func() { _ = l.Close() }()

	l6, err := net.ListenUDP("udp6", &net.UDPAddr{})
	require.NoError(t, err)
	defer func() { _ = l.Close() }()

	ports := []uint16{
		getPortUDP(t, l),
		getPortUDP(t, l6),
		34567,
		34568,
	}

	ns, err := netns.GetFromName(nsName)
	require.NoError(t, err)
	defer ns.Close()

	nsIno, err := util.GetInoForNs(ns)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		initialPorts, err := ReadInitialState("/proc", UDP, true, true)
		require.NoError(t, err)
		for _, p := range ports[:2] {
			if _, ok := initialPorts[PortMapping{testRootNs, p}]; !ok {
				t.Errorf("PortMapping(testRootNs, p) returned false for port %d", p)
				return false
			}
		}
		for _, p := range ports[2:] {
			if _, ok := initialPorts[PortMapping{nsIno, p}]; !ok {
				t.Errorf("PortMapping(nsIno, p) returned false for port %d", p)
				return false
			}
		}

		if _, ok := initialPorts[PortMapping{testRootNs, 999}]; ok {
			t.Errorf("expected IsListening(testRootNs, 999) to return false, but returned true")
			return false
		}
		if _, ok := initialPorts[PortMapping{nsIno, 999}]; ok {
			t.Errorf("expected IsListening(testRootNs, 999) to return false, but returned true")
			return false
		}

		return true
	}, 3*time.Second, time.Second, "udp/udp6 ports are listening")
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
