// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package netlink

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/mdlayher/netlink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	natPort    = 5432
	nonNatPort = 9876
)

// keep this test for netlink only, because eBPF listens to all namespaces all the time.
func TestConnTrackerCrossNamespaceAllNsDisabled(t *testing.T) {
	ns := testutil.SetupCrossNsDNAT(t)

	cfg := config.New()
	cfg.ConntrackMaxStateSize = 100
	cfg.ConntrackRateLimit = 500
	cfg.EnableConntrackAllNamespaces = false
	ct, err := NewConntracker(cfg, nil)
	require.NoError(t, err)
	time.Sleep(time.Second)

	closer := nettestutil.StartServerTCPNs(t, net.ParseIP("2.2.2.4"), 8080, ns)
	laddr := nettestutil.MustPingTCP(t, net.ParseIP("2.2.2.4"), 80).LocalAddr().(*net.TCPAddr)
	defer closer.Close()

	testNs, err := netns.GetFromName(ns)
	require.NoError(t, err)
	defer testNs.Close()

	testIno, err := kernel.GetInoForNs(testNs)
	require.NoError(t, err)

	time.Sleep(time.Second)
	trans := ct.GetTranslationForConn(
		&network.ConnectionTuple{
			Source: util.AddressFromNetIP(laddr.IP),
			SPort:  uint16(laddr.Port),
			Dest:   util.AddressFromString("2.2.2.4"),
			DPort:  uint16(80),
			Type:   network.TCP,
			NetNS:  testIno,
		},
	)

	assert.Nil(t, trans)
}

// This test generates a dump of netlink messages in test_data/message_dump
// In order to execute this test, run go test with `-args netlink_dump`
func TestMessageDump(t *testing.T) {
	skipUnless(t, "netlink_dump")

	testutil.SetupDNAT(t)

	f, err := os.CreateTemp("", "message_dump")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	defer f.Close()

	testMessageDump(t, f, net.ParseIP("1.1.1.1"), net.ParseIP("2.2.2.2"))
}

func TestMessageDump6(t *testing.T) {
	skipUnless(t, "netlink_dump")

	testutil.SetupDNAT6(t)

	f, err := os.CreateTemp("", "message_dump6")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	defer f.Close()

	testMessageDump(t, f, net.ParseIP("fd00::1"), net.ParseIP("fd00::2"))
}

func testMessageDump(t *testing.T, f *os.File, serverIP, clientIP net.IP) {
	consumer, err := NewConsumer(
		&config.Config{
			Config: ebpf.Config{
				ProcRoot: "/proc",
			},
			ConntrackRateLimit:           500,
			ConntrackRateLimitInterval:   time.Second,
			EnableRootNetNs:              true,
			EnableConntrackAllNamespaces: false,
		}, nil)
	require.NoError(t, err)
	events, err := consumer.Events()
	require.NoError(t, err)

	writeDone := make(chan struct{})
	go func() {
		for e := range events {
			for _, m := range e.Messages() {
				writeMsgToFile(f, m)
			}
			e.Done()
		}
		close(writeDone)
	}()

	tcpServer := nettestutil.StartServerTCP(t, serverIP, natPort)
	defer tcpServer.Close()

	udpServer := nettestutil.StartServerUDP(t, serverIP, nonNatPort)
	defer udpServer.Close()

	for i := 0; i < 100; i++ {
		tc := nettestutil.MustPingTCP(t, clientIP, natPort)
		tc.Close()
		uc := nettestutil.MustPingUDP(t, clientIP, nonNatPort)
		uc.Close()
	}

	time.Sleep(time.Second)
	consumer.Stop()
	<-writeDone
}

func skipUnless(t *testing.T, requiredArg string) {
	for _, arg := range os.Args[1:] {
		if arg == requiredArg {
			return
		}
	}

	//nolint:gosimple // TODO(NET) Fix gosimple linter
	t.Skip(
		fmt.Sprintf(
			"skipped %s. you can enable it by using running tests with `-args %s`",
			t.Name(),
			requiredArg,
		),
	)
}

func writeMsgToFile(f *os.File, m netlink.Message) {
	length := make([]byte, 4)
	binary.LittleEndian.PutUint32(length, uint32(len(m.Data)))
	payload := append(length, m.Data...)
	f.Write(payload)
}
