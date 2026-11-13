// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cenkalti/backoff/v7"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/link"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	secnetns "github.com/DataDog/datadog-agent/pkg/security/resolvers/netns"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const tcxMovesDomain = "tcx-moves.test"

var tcxMovesRule = &rules.RuleDefinition{
	ID:         "test_rule_tcx_moves",
	Expression: fmt.Sprintf(`dns.question.name == "%s"`, tcxMovesDomain),
}

// tcxMoves holds what the device move scenarios share
type tcxMoves struct {
	test        *testModule
	probe       *sprobe.EBPFProbe
	ip          string
	hostNetNS   uint32
	settleDelay time.Duration
}

// testTCXDeviceMoves checks that TCX links never run with the namespace of a device that moved away
func testTCXDeviceMoves(t *testing.T) {
	for _, module := range []string{"dummy", "macvlan", "veth", "vxlan", "bridge"} {
		if out, err := loadModule(module); err != nil {
			t.Fatalf("couldn't load the %s module: %s, %v", module, string(out), err)
		}
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{tcxTestRule, tcxMovesRule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Fatal("not supported")
	}
	hostNetNS, err := getCurrentNetns()
	if err != nil {
		t.Fatal(err)
	}

	m := &tcxMoves{
		test:        test,
		probe:       p,
		ip:          which(t, "ip"),
		hostNetNS:   hostNetNS,
		settleDelay: test.probe.Config.Probe.NetworkTCXSettleDelay,
	}

	m.run(t, "wptcxdum0", "link", "add", "wptcxdum0", "type", "dummy")
	m.run(t, "", "link", "set", "wptcxdum0", "up")

	t.Run("macvlan_moved_before_settling", m.testMovedBeforeSettling)
	t.Run("tunnel_and_bridge_not_delayed", m.testNotDelayed)
	t.Run("veth_peer_moved", m.testVethPeerMoved)
	t.Run("host_veth_pair", m.testHostVethPair)
	t.Run("docker_veth_pair", m.testDockerVethPair)
	t.Run("failing_device", m.testFailingDevice)
	t.Run("netns_destruction", m.testNetNSDestruction)
}

// run runs ip with args, and deletes the device named by cleanup at the end of the test
func (m *tcxMoves) run(tb testing.TB, cleanup string, args ...string) {
	tb.Helper()
	if out, err := exec.Command(m.ip, args...).CombinedOutput(); err != nil {
		tb.Fatalf("ip %s: %s, %v", strings.Join(args, " "), string(out), err)
	}
	if cleanup != "" {
		tb.Cleanup(func() {
			_ = exec.Command(m.ip, "link", "delete", cleanup).Run()
		})
	}
}

// newNetNS creates a named network namespace, and registers it with the resolver before any device moves into it
func (m *tcxMoves) newNetNS(tb testing.TB, name string) (uint32, *utils.NSPath) {
	tb.Helper()
	m.run(tb, "", "netns", "add", name)
	tb.Cleanup(func() {
		_ = exec.Command(m.ip, "netns", "delete", name).Run()
	})

	nsPath := utils.NewNSPathFromPath(filepath.Join("/var/run/netns", name), utils.NetNsType)
	nsID, err := nsPath.GetNSID()
	if err != nil {
		tb.Fatal(err)
	}
	if entry, _ := m.probe.Resolvers.NamespaceResolver.SaveNetworkNamespaceHandle(nsID, nsPath); entry == nil {
		tb.Fatalf("the resolver didn't take namespace %d", nsID)
	}
	return nsID, nsPath
}

func (m *tcxMoves) flush() {
	m.probe.Resolvers.NamespaceResolver.FlushNamespaces()
}

// ifIndex returns the ifindex of a device in a namespace, the host one when nsPath is nil
func (m *tcxMoves) ifIndex(tb testing.TB, nsPath *utils.NSPath, name string) uint32 {
	tb.Helper()
	handle, err := netlinkHandle(nsPath)
	if err != nil {
		tb.Fatal(err)
	}
	defer handle.Close()

	device, err := handle.LinkByName(name)
	if err != nil {
		tb.Fatalf("couldn't find %s: %v", name, err)
	}
	return uint32(device.Attrs().Index)
}

func netlinkHandle(nsPath *utils.NSPath) (*netlink.Handle, error) {
	if nsPath == nil {
		return netlink.NewHandle(unix.NETLINK_ROUTE)
	}
	ns, err := netns.GetFromPath(nsPath.GetPath())
	if err != nil {
		return nil, err
	}
	defer ns.Close()
	return netlink.NewHandleAt(ns, unix.NETLINK_ROUTE)
}

// programs returns the IDs of the classifiers the resolver holds for a device
func (m *tcxMoves) programs(nsID uint32, ifIndex uint32) []uint32 {
	var ids []uint32
	for _, program := range m.probe.Resolvers.TCResolver.DumpNetworkNamespacePrograms(nsID, nil, nil)[ifIndex] {
		ids = append(ids, program.ProgramID)
	}
	return ids
}

func (m *tcxMoves) isAttached(nsID uint32, ifIndex uint32) bool {
	return len(m.programs(nsID, ifIndex)) > 0
}

func (m *tcxMoves) waitAttached(tb testing.TB, nsID uint32, ifIndex uint32, timeout time.Duration) {
	tb.Helper()
	err := retry(tb, func() error {
		if !m.isAttached(nsID, ifIndex) {
			return fmt.Errorf("device %d of namespace %d isn't attached", ifIndex, nsID)
		}
		return nil
	}, backoff.WithBackOff(backoff.NewConstantBackOff(100*time.Millisecond)), backoff.WithMaxElapsedTime(timeout))
	if err != nil {
		tb.Fatalf("%v\n%s", err, m.describe(nsID, ifIndex))
	}
}

// describe reports what the resolvers hold for a device, for failure messages
func (m *tcxMoves) describe(nsID uint32, ifIndex uint32) string {
	dump, err := dumpNetworkNamespaces(m.probe, false)
	if err != nil {
		return fmt.Sprintf("couldn't dump the network namespaces: %v", err)
	}

	var lines []string
	switch i := slices.IndexFunc(dump, func(ns secnetns.NetworkNamespaceDump) bool { return ns.NsID == nsID }); {
	case i < 0:
		lines = append(lines, fmt.Sprintf("namespace %d isn't tracked", nsID))
	case dump[i].HandleFD < 0:
		lines = append(lines, fmt.Sprintf("namespace %d has no handle, queued devices %+v", nsID, dump[i].DevicesInQueue))
	default:
		lines = append(lines, fmt.Sprintf("namespace %d has handle %s, queued devices %+v", nsID, dump[i].HandlePath, dump[i].DevicesInQueue))
	}

	namespaces := []uint32{m.hostNetNS, nsID}
	for _, ns := range dump {
		namespaces = append(namespaces, ns.NsID)
	}
	slices.Sort(namespaces)
	for _, ns := range slices.Compact(namespaces) {
		for _, program := range m.probe.Resolvers.TCResolver.DumpNetworkNamespacePrograms(ns, nil, nil)[ifIndex] {
			lines = append(lines, fmt.Sprintf("ifindex %d of namespace %d holds %s (%s, %s, program %d)", ifIndex, ns, program.FuncName, program.Direction, program.AttachMode, program.ProgramID))
		}
	}

	_ = m.probe.Resolvers.NamespaceResolver.SendStats()
	lines = append(lines, fmt.Sprintf("namespace resolver errors: %v", m.test.statsdClient.GetByPrefix(metrics.MetricNamespaceResolverError+":error_type:")))
	return strings.Join(lines, "\n")
}

// hookPrograms returns the IDs of the TCX programs of both hooks of a device
func hookPrograms(tb testing.TB, nsPath *utils.NSPath, ifName string) []uint32 {
	tb.Helper()
	var ids []uint32
	for _, direction := range []manager.TrafficType{manager.Egress, manager.Ingress} {
		for _, program := range tcxHookPrograms(tb, nsPath, ifName, direction) {
			ids = append(ids, uint32(program.ID))
		}
	}
	return ids
}

// assertOnlyLocalPrograms checks that the programs of a moved device are exactly the ones attached in its namespace
func (m *tcxMoves) assertOnlyLocalPrograms(tb testing.TB, nsID uint32, nsPath *utils.NSPath, ifName string) {
	tb.Helper()
	local := m.programs(nsID, m.ifIndex(tb, nsPath, ifName))
	assert.NotEmpty(tb, local, "%s isn't attached in namespace %d", ifName, nsID)
	assert.ElementsMatch(tb, local, hookPrograms(tb, nsPath, ifName), "programs of %s in namespace %d", ifName, nsID)
}

func (m *tcxMoves) staleLinks() int64 {
	m.probe.Resolvers.TCResolver.SendTCProgramsStats(m.test.statsdClient)
	return m.test.statsdClient.Get(metrics.MetricTCStaleLinks)
}

func (m *tcxMoves) errors(class string) int64 {
	_ = m.probe.Resolvers.NamespaceResolver.SendStats()
	return m.test.statsdClient.Get(metrics.MetricNamespaceResolverError + ":error_type:" + class)
}

// testMovedBeforeSettling moves a macvlan, standing for a VF or a Docker macvlan, right after creating it
func (m *tcxMoves) testMovedBeforeSettling(t *testing.T) {
	_, nsPath := m.newNetNS(t, "wptcx_early")

	m.run(t, "", "link", "add", "wptcxmv0", "link", "wptcxdum0", "type", "macvlan", "mode", "bridge")
	hostIfIndex := m.ifIndex(t, nil, "wptcxmv0")
	m.run(t, "", "link", "set", "wptcxmv0", "netns", "wptcx_early")

	time.Sleep(m.settleDelay + 2*time.Second)

	assert.False(t, m.isAttached(m.hostNetNS, hostIfIndex), "wptcxmv0 was attached in the host namespace")
	// a link attached in the host namespace would have followed the device
	assert.Empty(t, hookPrograms(t, nsPath, "wptcxmv0"), "programs of wptcxmv0 in its new namespace")
}

// testNotDelayed checks that tunnels and bridges, which stay where they're created, are attached right away
func (m *tcxMoves) testNotDelayed(t *testing.T) {
	m.run(t, "wptcxvx0", "link", "add", "wptcxvx0", "type", "vxlan", "id", "42", "dstport", "4789", "local", "127.0.0.1")
	m.run(t, "wptcxbr0", "link", "add", "wptcxbr0", "type", "bridge")

	for _, name := range []string{"wptcxvx0", "wptcxbr0"} {
		m.waitAttached(t, m.hostNetNS, m.ifIndex(t, nil, name), m.settleDelay/2)
	}
}

// testVethPeerMoved checks that the move event of a veth peer attaches it once, in its new namespace
func (m *tcxMoves) testVethPeerMoved(t *testing.T) {
	nsID, nsPath := m.newNetNS(t, "wptcx_veth")

	m.run(t, "wptcxve0", "link", "add", "wptcxve0", "type", "veth", "peer", "name", "wptcxve1")
	m.run(t, "", "link", "set", "wptcxve1", "netns", "wptcx_veth")

	m.waitAttached(t, nsID, m.ifIndex(t, nsPath, "wptcxve1"), 5*time.Second)
	m.assertOnlyLocalPrograms(t, nsID, nsPath, "wptcxve1")
	assert.False(t, m.isAttached(m.hostNetNS, m.ifIndex(t, nil, "wptcxve0")), "the host end was attached")
}

// testHostVethPair checks that a veth pair kept in the host namespace, like cilium_host and cilium_net, is attached
// by the host snapshot of a flush, although no event reports it
func (m *tcxMoves) testHostVethPair(t *testing.T) {
	m.run(t, "wptcxch0", "link", "add", "wptcxch0", "type", "veth", "peer", "name", "wptcxch1")
	ifIndexes := []uint32{m.ifIndex(t, nil, "wptcxch0"), m.ifIndex(t, nil, "wptcxch1")}

	m.flush()
	time.Sleep(m.settleDelay / 2)
	for _, ifIndex := range ifIndexes {
		assert.False(t, m.isAttached(m.hostNetNS, ifIndex), "device %d was attached before settling", ifIndex)
	}
	for _, ifIndex := range ifIndexes {
		m.waitAttached(t, m.hostNetNS, ifIndex, m.settleDelay+5*time.Second)
	}
}

// testDockerVethPair checks that the host end of a veth pair whose other end moved stays unattached, as in legacy
// mode, even when a host snapshot catches the pair before the move
func (m *tcxMoves) testDockerVethPair(t *testing.T) {
	m.newNetNS(t, "wptcx_docker")

	m.run(t, "wptcxdk0", "link", "add", "wptcxdk0", "type", "veth", "peer", "name", "wptcxdk1")
	hostEnd := m.ifIndex(t, nil, "wptcxdk0")
	m.flush()
	m.run(t, "", "link", "set", "wptcxdk1", "netns", "wptcx_docker")

	for range 3 {
		time.Sleep(m.settleDelay + time.Second)
		m.flush()
	}
	time.Sleep(m.settleDelay + time.Second)
	assert.False(t, m.isAttached(m.hostNetNS, hostEnd), "the host end was attached")
}

// testFailingDevice fills the TCX hooks of a host device, so that attaching it fails, and checks that the host
// snapshots don't try it again
func (m *tcxMoves) testFailingDevice(t *testing.T) {
	m.run(t, "wptcxfl0", "link", "add", "wptcxfl0", "type", "veth", "peer", "name", "wptcxfl1")
	failing := m.ifIndex(t, nil, "wptcxfl0")

	// a hook holds a program only once, and reports ERANGE when full, below 64 programs
	var progs []*ebpf.Program
	for _, attach := range []ebpf.AttachType{ebpf.AttachTCXIngress, ebpf.AttachTCXEgress} {
		for i := 0; ; i++ {
			if i == 64 {
				t.Fatalf("the %s hook of wptcxfl0 never filled up", attach)
			}
			if i == len(progs) {
				prog, err := ebpf.NewProgram(&ebpf.ProgramSpec{
					Name:         fmt.Sprintf("foreign_%d", i),
					Type:         ebpf.SchedCLS,
					License:      "Apache-2.0",
					Instructions: asm.Instructions{asm.Mov.Imm(asm.R0, probes.TCActUnspec), asm.Return()},
				})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = prog.Close() })
				progs = append(progs, prog)
			}

			l, err := link.AttachTCX(link.TCXOptions{Interface: int(failing), Program: progs[i], Attach: attach})
			if errors.Is(err, unix.ERANGE) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = l.Close() })
		}
	}

	failures := m.errors("tcx_attach")
	m.flush()
	// wptcxfl1 is attached first in the same settled batch, so its attach doesn't show that wptcxfl0 was tried
	err := retry(t, func() error {
		if m.errors("tcx_attach") == failures {
			return errors.New("no failed attach of wptcxfl0 was counted")
		}
		return nil
	}, backoff.WithBackOff(backoff.NewConstantBackOff(100*time.Millisecond)), backoff.WithMaxElapsedTime(m.settleDelay+5*time.Second))
	if err != nil {
		t.Fatalf("%v\n%s", err, m.describe(m.hostNetNS, failing))
	}
	m.waitAttached(t, m.hostNetNS, m.ifIndex(t, nil, "wptcxfl1"), 5*time.Second)
	assert.Equal(t, int64(1), m.errors("tcx_attach")-failures, "failed attaches of wptcxfl0")

	for range 3 {
		m.flush()
		time.Sleep(time.Second)
	}
	time.Sleep(m.settleDelay + time.Second)
	assert.Equal(t, int64(1), m.errors("tcx_attach")-failures, "failed attaches of wptcxfl0")
	assert.False(t, m.isAttached(m.hostNetNS, failing))
}

// testNetNSDestruction checks that the links of a destroyed namespace go away with their entries, without being
// counted as stale
func (m *tcxMoves) testNetNSDestruction(t *testing.T) {
	nsID, nsPath := m.newNetNS(t, "wptcx_gone")

	m.run(t, "", "link", "add", "wptcxde0", "type", "veth", "peer", "name", "wptcxde1")
	m.run(t, "", "link", "set", "wptcxde1", "netns", "wptcx_gone")
	ifIndex := m.ifIndex(t, nsPath, "wptcxde1")
	m.waitAttached(t, nsID, ifIndex, 5*time.Second)

	staleLinks := m.staleLinks()
	// a CNI plugin deletes the pair before the namespace
	m.run(t, "", "link", "delete", "wptcxde0")
	m.run(t, "", "netns", "delete", "wptcx_gone")

	err := retry(t, func() error {
		m.flush()
		if programs := m.probe.Resolvers.TCResolver.DumpNetworkNamespacePrograms(nsID, nil, nil); len(programs) > 0 {
			return fmt.Errorf("classifiers left in namespace %d: %+v", nsID, programs)
		}
		return nil
	}, backoff.WithBackOff(backoff.NewConstantBackOff(time.Second)), backoff.WithMaxTries(10))
	if err != nil {
		t.Errorf("%v\n%s", err, m.describe(nsID, ifIndex))
	}
	assert.Equal(t, staleLinks, m.staleLinks(), "stale links")

	_, err = os.Stat(nsPath.GetPath())
	assert.True(t, os.IsNotExist(err), "the namespace is still mounted: %v", err)
}
