// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

//nolint:revive // TODO(NET) Fix revive linter
package testutil

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
)

var curDir string

func init() {
	curDir, _ = _curDir()
}

// SetupDNAT sets up a NAT translation from:
// * 2.2.2.2 to 1.1.1.1 (OUTPUT Chain)
// * 3.3.3.3 to 1.1.1.1 (PREROUTING Chain)
func SetupDNAT(t *testing.T) {
	linkName := "dummy" + strconv.Itoa(rand.Intn(9999)+1)
	nettestutil.IptablesSave(t)
	t.Cleanup(func() {
		teardownDNAT(t, linkName)
	})

	cmds := []string{
		fmt.Sprintf("ip link add %s type dummy", linkName),
		fmt.Sprintf("ip address add 1.1.1.1 broadcast + dev %s", linkName),
		fmt.Sprintf("ip link set %s up", linkName),
		"iptables -t nat -A OUTPUT --dest 2.2.2.2 -j DNAT --to-destination 1.1.1.1",
		"iptables -t nat -A PREROUTING --dest 3.3.3.3 -j DNAT --to-destination 1.1.1.1",
	}
	nettestutil.RunCommands(t, cmds, false)
}

// SetupSNAT sets up a NAT translation from:
// * 6.6.6.6 to 7.7.7.7 (POSTROUTING Chain)
func SetupSNAT(t *testing.T) string {
	linkName := "dummy-2-" + strconv.Itoa(rand.Intn(9999)+1)
	nettestutil.IptablesSave(t)
	t.Cleanup(func() {
		teardownDNAT(t, linkName)
	})

	cmds := []string{
		fmt.Sprintf("ip link add %s type dummy", linkName),
		fmt.Sprintf("ip address add 7.7.7.7 broadcast + dev %s", linkName),
		fmt.Sprintf("ip address add 6.6.6.6 broadcast + dev %s", linkName),
		fmt.Sprintf("ip link set %s up", linkName),
		"iptables -t nat -A POSTROUTING -s 6.6.6.6/32 -j SNAT --to-source 7.7.7.7",
	}
	nettestutil.RunCommands(t, cmds, false)

	return linkName
}

// teardownDNAT cleans up the resources created by SetupDNAT
func teardownDNAT(t *testing.T, linkName string) {
	cmds := []string{
		// tear down the testing interface, and iptables rule
		fmt.Sprintf("ip link del %s", linkName),
		// clear out the conntrack table
		"conntrack -F",
	}
	nettestutil.RunCommands(t, cmds, true)
}

func getDefaultInterfaceName(t *testing.T) string {
	out := nettestutil.RunCommands(t, []string{"ip route get 8.8.8.8"}, false)
	if len(out) > 0 {
		parts := strings.Split(out[0], " ")
		if len(parts) > 5 {
			return parts[4]
		}
	}
	return ""
}

// SetupDNAT6 sets up a NAT translation from fd00::2 to fd00::1
func SetupDNAT6(t *testing.T) {
	linkName := "dummy" + strconv.Itoa(rand.Intn(9999)+1)
	ifName := getDefaultInterfaceName(t)
	t.Cleanup(func() {
		teardownDNAT6(t, ifName, linkName)
	})

	nettestutil.Ip6tablesSave(t)
	cmds := []string{
		fmt.Sprintf("ip link add %s type dummy", linkName),
		fmt.Sprintf("ip address add fd00::1 dev %s", linkName),
		fmt.Sprintf("ip link set %s up", linkName),
		fmt.Sprintf("%s/testdata/wait_if.sh %s", curDir, linkName),
		"ip -6 route add fd00::2 dev " + ifName,
		"ip6tables -t nat -A OUTPUT --dest fd00::2 -j DNAT --to-destination fd00::1",
	}
	nettestutil.RunCommands(t, cmds, false)
}

// teardownDNAT6 cleans up the resources created by SetupDNAT6
func teardownDNAT6(t *testing.T, ifName string, linkName string) {
	cmds := []string{
		// tear down the testing interface, and iptables rule
		fmt.Sprintf("ip link del %s", linkName),
		"ip -6 r del fd00::2 dev " + ifName,

		// clear out the conntrack table
		"conntrack -F",
	}
	nettestutil.RunCommands(t, cmds, true)
}

// SetupVethPair sets up a network namespace, along with two IP addresses
// 2.2.2.3 and 2.2.2.4 to be used for namespace aware tests.
// 2.2.2.4 is within the specified namespace, while 2.2.2.3 is a peer in the root namespace.
func SetupVethPair(tb testing.TB) (ns string) {
	ns = AddNS(tb)
	tb.Cleanup(func() {
		teardownVethPair(tb)
	})

	cmds := []string{
		"ip link add veth1 type veth peer name veth2",
		fmt.Sprintf("ip link set veth2 netns %s", ns),
		"ip address add 2.2.2.3/24 dev veth1",
		fmt.Sprintf("ip -n %s address add 2.2.2.4/24 dev veth2", ns),
		"ip link set veth1 up",
		fmt.Sprintf("ip -n %s link set veth2 up", ns),
		fmt.Sprintf("ip netns exec %s ip route add default via 2.2.2.3", ns),
	}
	nettestutil.RunCommands(tb, cmds, false)
	return
}

// teardownVethPair cleans up the resources created by SetupVethPair
func teardownVethPair(tb testing.TB) {
	cmds := []string{
		"ip link del veth1",
	}
	nettestutil.RunCommands(tb, cmds, true)
}

// SetupVeth6Pair sets up a network namespace, along with two IPv6 addresses
// fd00::1 and fd00::2 to be used for namespace aware tests.
// fd00::2 is within the specified namespace, while fd00::1 is a peer in the root namespace.
func SetupVeth6Pair(t *testing.T) (ns string) {
	ns = AddNS(t)
	t.Cleanup(func() {
		teardownVeth6Pair(t, ns)
	})

	cmds := []string{
		"ip link add veth1 type veth peer name veth2",
		fmt.Sprintf("ip link set veth2 netns %s", ns),
		"ip address add fd00::1/64 dev veth1",
		fmt.Sprintf("ip -n %s address add fd00::2/64 dev veth2", ns),
		"ip link set veth1 up",
		fmt.Sprintf("ip -n %s link set veth2 up", ns),
		fmt.Sprintf("%s/testdata/wait_if.sh veth1 %s", curDir, ns),
		fmt.Sprintf("%s/testdata/wait_if.sh veth2 %s", curDir, ns),
		fmt.Sprintf("ip netns exec %s ip -6 route add default dev veth2", ns),
	}
	nettestutil.RunCommands(t, cmds, false)
	return
}

//nolint:revive // TODO(NET) Fix revive linter
func teardownVeth6Pair(t *testing.T, ns string) {
	cmds := []string{
		"ip link del veth1",
	}
	nettestutil.RunCommands(t, cmds, true)
}

// SetupCrossNsDNAT sets up a network namespace, a veth pair, and a NAT
// rule in the specified network namespace. Redirecting port 80 to 8080
// within the namespace.
func SetupCrossNsDNAT(tb testing.TB) (ns string) {
	return setupCrossNsDNAT(tb, 80, 8080)
}

// SetupCrossNsDNATWithPorts sets up a network namespace, a veth pair, and a NAT
// rule in the specified network namespace. Redirecting `dport` to `redirPort`
// within the namespace.
func SetupCrossNsDNATWithPorts(tb testing.TB, dport int, redirPort int) (ns string) {
	return setupCrossNsDNAT(tb.(*testing.T), dport, redirPort)
}

func setupCrossNsDNAT(tb testing.TB, dport int, redirPort int) (ns string) {
	tb.Cleanup(func() {
		teardownCrossNsDNAT(tb)
	})
	ns = SetupVethPair(tb)

	nettestutil.IptablesSave(tb)
	cmds := []string{
		//this is required to enable conntrack in the root net namespace
		//conntrack won't be enabled unless there is at least one iptables
		//rule that uses connection tracking
		"iptables -I INPUT 1 -m conntrack --ctstate NEW,RELATED,ESTABLISHED -j ACCEPT",

		fmt.Sprintf("ip netns exec %s iptables -A PREROUTING -t nat -p tcp --dport %d -j REDIRECT --to-port %d", ns, dport, redirPort),
		fmt.Sprintf("ip netns exec %s iptables -A PREROUTING -t nat -p udp --dport %d -j REDIRECT --to-port %d", ns, dport, redirPort),
	}
	nettestutil.RunCommands(tb, cmds, false)
	return
}

// teardownCrossNsDNAT cleans up the resources created by SetupCrossNsDNAT
func teardownCrossNsDNAT(tb testing.TB) {
	cmds := []string{
		"conntrack -F",
	}
	nettestutil.RunCommands(tb, cmds, true)
}

// SetupCrossNsDNAT6 sets up a network namespace, along with two IPv6 addresses
// fd00::1 and fd00::2 to be used for namespace aware tests.
// fd00::2 is within the specified namespace, while fd00::1 is a peer in the root namespace.
func SetupCrossNsDNAT6(t *testing.T) (ns string) {
	t.Cleanup(func() {
		teardownCrossNsDNAT6(t)
	})
	ns = SetupVeth6Pair(t)

	nettestutil.Ip6tablesSave(t)
	cmds := []string{
		"ip6tables -I INPUT 1 -m conntrack --ctstate NEW,RELATED,ESTABLISHED -j ACCEPT",
		fmt.Sprintf("ip netns exec %s ip6tables -A PREROUTING -t nat -p tcp --dport 80 -j REDIRECT --to-port 8080", ns),
		fmt.Sprintf("ip netns exec %s ip6tables -A PREROUTING -t nat -p udp --dport 80 -j REDIRECT --to-port 8080", ns),
	}
	nettestutil.RunCommands(t, cmds, false)
	return
}

// TeardownCrossNsDNAT6 cleans up the resources created by SetupCrossNsDNAT6
func teardownCrossNsDNAT6(t *testing.T) {
	cmds := []string{
		"conntrack -F",
	}
	nettestutil.RunCommands(t, cmds, true)
}

// AddNS adds a randomly named network namespace
func AddNS(tb testing.TB) string {
	tb.Helper()
	var err error
	for i := 0; i < 10; i++ {
		ns := "test" + strconv.Itoa(rand.Intn(99))
		_, err = nettestutil.RunCommand("ip netns add " + ns)
		if err == nil {
			tb.Cleanup(func() {
				_, _ = nettestutil.RunCommand("ip netns del " + ns)
			})
			return ns
		}
	}
	tb.Fatalf("unable to create network namespace: %s", err)
	return ""
}

func _curDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("unable to get current file build path")
	}

	buildDir := filepath.Dir(file)

	// build relative path from base of repo
	buildRoot := rootDir(buildDir)
	relPath, err := filepath.Rel(buildRoot, buildDir)
	if err != nil {
		return "", err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	curRoot := rootDir(cwd)

	return filepath.Join(curRoot, relPath), nil
}

// rootDir returns the base repository directory, just before `pkg`.
// If `pkg` is not found, the dir provided is returned.
func rootDir(dir string) string {
	pkgIndex := -1
	parts := strings.Split(dir, string(filepath.Separator))
	for i, d := range parts {
		if d == "pkg" {
			pkgIndex = i
			break
		}
	}
	if pkgIndex == -1 {
		return dir
	}
	return strings.Join(parts[:pkgIndex], string(filepath.Separator))
}
