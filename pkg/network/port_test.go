// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package network

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"

	netlinktestutil "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestMain(m *testing.M) {
	logLevel := os.Getenv("DD_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "warn"
	}
	log.SetupLogger(seelog.Default, logLevel)

	os.Exit(m.Run())
}

var (
	ip4Re = regexp.MustCompile(`.+listening on.+0.0.0.0:([0-9]+)`)
	ip6Re = regexp.MustCompile(`.+listening on.+\[0000:0000:0000:0000:0000:0000:0000:0000\]:([0-9]+)`)
)

// runServerProcess runs a server using `socat` externally
//
// `proto` can be "tcp4", "tcp6", "udp4", or "udp6"
// `port` can be `0` in which case the os assigned port is returned
func runServerProcess(t *testing.T, proto string, port uint16, ns netns.NsHandle) (uint16, *os.Process) {
	var re *regexp.Regexp
	address := fmt.Sprintf("%s-listen:%d", proto, port)
	switch proto {
	case "tcp4", "udp4":
		re = ip4Re
	case "tcp6", "udp6":
		re = ip6Re
	default:
		require.FailNow(t, "unrecognized protocol")
	}

	var proc *os.Process
	kernel.WithNS(ns, func() error {
		cmd := exec.Command("socat", "-d", "-d", "STDIO", address)
		stderr, err := cmd.StderrPipe()
		require.NoError(t, err, "error getting stderr pipe for command %s", cmd)
		require.NoError(t, cmd.Start())
		proc = cmd.Process
		if port != 0 {
			return nil
		}

		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			matches := re.FindStringSubmatch(scanner.Text())
			if len(matches) == 0 {
				continue
			}

			require.Len(t, matches, 2)
			_port, err := strconv.ParseUint(matches[1], 10, 16)
			require.NoError(t, err)
			port = uint16(_port)
			break
		}

		return nil
	})

	return port, proc
}

func TestReadInitialState(t *testing.T) {
	t.Run("TCP", func(t *testing.T) {
		testReadInitialState(t, "tcp")
	})
	t.Run("UDP", func(t *testing.T) {
		testReadInitialState(t, "udp")
	})
}

func testReadInitialState(t *testing.T, proto string) {
	var ns, rootNs netns.NsHandle
	var err error
	nsName := netlinktestutil.AddNS(t)
	ns, err = netns.GetFromName(nsName)
	require.NoError(t, err)
	t.Cleanup(func() { ns.Close() })
	rootNs, err = kernel.GetRootNetNamespace("/proc")
	require.NoError(t, err)
	t.Cleanup(func() { rootNs.Close() })

	var protos []string
	switch proto {
	case "tcp":
		protos = []string{"tcp4", "tcp6", "udp4", "udp6"}
	case "udp":
		protos = []string{"udp4", "udp6", "tcp4", "tcp6"}
	}

	var ports []uint16
	for _, proto := range protos {
		i := 0
		for ; i < 5; i++ {
			port, proc := runServerProcess(t, proto, 0, rootNs)
			if !slices.Contains(ports, port) {
				t.Cleanup(func() { proc.Kill() })
				ports = append(ports, port)
				break
			}

			require.NoError(t, proc.Kill())
		}
		require.Less(t, i, 5, "failed to find unique port for proto %s", proto)
	}

	for _, proto := range protos {
		i := 0
		for ; i < 5; i++ {
			port, proc := runServerProcess(t, proto, 0, ns)
			if !slices.Contains(ports, port) {
				t.Cleanup(func() { proc.Kill() })
				ports = append(ports, port)
				break
			}

			require.NoError(t, proc.Kill())
		}
		require.Less(t, i, 5, "failed to find unique port for proto %s", proto)
	}

	rootNsIno, err := kernel.GetInoForNs(rootNs)
	require.NoError(t, err)
	nsIno, err := kernel.GetInoForNs(ns)
	require.NoError(t, err)

	connType := TCP
	otherConnType := UDP
	if proto == "udp" {
		connType, otherConnType = otherConnType, connType
	}

	initialPorts, err := ReadInitialState("/proc", connType, true)
	if !assert.NoError(t, err) {
		return
	}

	// check ports corresponding to proto in root ns
	for _, p := range ports[:2] {
		assert.Containsf(t, initialPorts, PortMapping{rootNsIno, p}, "PortMapping should exist for %s port %d in root ns", connType, p)
		assert.NotContainsf(t, initialPorts, PortMapping{nsIno, p}, "PortMapping should not exist for %s port %d in test ns", connType, p)
	}

	// check ports not corresponding to proto in root ns
	for _, p := range ports[2:4] {
		assert.NotContainsf(t, initialPorts, PortMapping{rootNsIno, p}, "PortMapping should not exist for %s port %d in root ns", otherConnType, p)
		assert.NotContainsf(t, initialPorts, PortMapping{nsIno, p}, "PortMapping should not exist for %s port %d in test ns", otherConnType, p)
	}

	// check ports corresponding to proto in test ns
	for _, p := range ports[4:6] {
		assert.Containsf(t, initialPorts, PortMapping{nsIno, p}, "PortMapping should exist for %s port %d in test ns", connType, p)
		assert.NotContainsf(t, initialPorts, PortMapping{rootNsIno, p}, "PortMapping should not exist for %s port %d in root ns", connType, p)
	}

	// check ports not corresponding to proto in test ns
	for _, p := range ports[6:8] {
		assert.NotContainsf(t, initialPorts, PortMapping{nsIno, p}, "PortMapping should not exist for %s port %d in test ns", otherConnType, p)
		assert.NotContainsf(t, initialPorts, PortMapping{rootNsIno, p}, "PortMapping should not exist for %s port %d in root ns", otherConnType, p)
	}
}
