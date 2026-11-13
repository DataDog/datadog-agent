// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
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
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	secnetns "github.com/DataDog/datadog-agent/pkg/security/resolvers/netns"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// The subtests share one module: the config is the union of the configs of the scenarios they reuse, since any
// testOpts change rebuilds the module.
var _ = declare(TestTCX, testOpts{
	networkTCXEnabled:         true,
	networkIngressEnabled:     true,
	networkRawPacketEnabled:   true,
	networkFlowMonitorEnabled: true,
	dnsPort:                   DNSPort,
})

// TestTCX runs the network scenarios with TCX requested. On kernels without TCX, they check the fallback to legacy tc
// filters instead.
func TestTCX(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel()
	})

	if testEnvironment == DockerEnvironment {
		t.Skip("skipping TCX tests in docker")
	}

	kv, err := kernel.NewKernelVersion()
	if err != nil {
		t.Fatal(err)
	}
	tcxSupported := kv.HasTCX()
	rawPacketSupported := !sprobe.IsRawPacketNotSupported(kv)

	// must happen before the module of the group attaches to lo, as when an agent dies and another one starts
	staleFilters := addStaleTCFilters(t, "lo", probes.GetTCProbes(true, rawPacketSupported))

	t.Run("upgrade", func(t *testing.T) {
		test, err := newTestModule(t, nil, []*rules.RuleDefinition{tcxTestRule})
		if err != nil {
			t.Fatal(err)
		}
		defer test.CloseTest()

		err = retry(t, func() error {
			names, err := tcFilterNames("lo")
			if err != nil {
				return err
			}
			if left := slices.DeleteFunc(names, func(name string) bool { return !slices.Contains(staleFilters, name) }); len(left) > 0 {
				return fmt.Errorf("stale filters left on lo: %v", left)
			}
			return nil
		}, backoff.WithBackOff(backoff.NewConstantBackOff(200*time.Millisecond)), backoff.WithMaxTries(50))
		assert.NoError(t, err)
	})

	t.Run("mode", func(t *testing.T) {
		test, err := newTestModule(t, nil, []*rules.RuleDefinition{tcxTestRule})
		if err != nil {
			t.Fatal(err)
		}
		defer test.CloseTest()

		p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
		if !ok {
			t.Fatal("not supported")
		}

		expectedMode := manager.TCAttachModeLegacy
		if tcxSupported {
			expectedMode = manager.TCAttachModeTCX
		}
		assert.Equal(t, tcxSupported, p.GetUseTCX())
		assert.Equal(t, expectedMode, p.Resolvers.TCResolver.AttachMode())

		status, err := test.cws.GetStatus(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, tcxSupported, status.GetEnvironment().GetUseTCX())

		// TCX is requested explicitly by the config of the group
		hasWarning := slices.ContainsFunc(status.GetEnvironment().GetWarnings(), func(warning string) bool {
			return strings.Contains(warning, "network.tcx.enabled")
		})
		assert.Equal(t, !tcxSupported, hasWarning, "TCX warning in %v", status.GetEnvironment().GetWarnings())
	})

	t.Run("dns", func(t *testing.T) {
		testDNS(t, lookupIPAtPort(DNSPort))
	})

	t.Run("dns_response", testDNSResponse)

	t.Run("imds", testAWSIMDSv1Request)

	t.Run("raw_packet", testRawPacket)

	t.Run("raw_packet_drop", testRawPacketAction)

	t.Run("flow_monitor", testNetworkFlowSendUDP4)

	t.Run("net_device", testNetDevice)

	// last: it stops the module
	t.Run("tc_filters", func(t *testing.T) {
		testTCXFilters(t, tcxSupported, rawPacketSupported)
	})
}

// tcxTestRule forces the activation of the network probes
var tcxTestRule = &rules.RuleDefinition{
	ID:         "test_rule_tcx",
	Expression: `dns.question.type == A`,
}

// testTCXFilters checks the classifiers of lo in the host namespace and in a new one, while the module runs and
// once it stops
func testTCXFilters(t *testing.T, tcxSupported bool, rawPacketSupported bool) {
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{tcxTestRule})
	if err != nil {
		t.Fatal(err)
	}

	var testModuleCleanedUp bool
	defer func() {
		if !testModuleCleanedUp {
			test.CloseTest()
		}
	}()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Fatal("not supported")
	}

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	newNetNSSleep := exec.Command(syscallTester, "new_netns_exec", which(t, "sleep"), "600")
	if err := newNetNSSleep.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = newNetNSSleep.Process.Kill()
		_ = newNetNSSleep.Wait()
	})

	namespaces := map[string]*utils.NSPath{
		"host": utils.NewNSPathFromPid(utils.Getpid(), utils.NetNsType),
		"new":  utils.NewNSPathFromPid(uint32(newNetNSSleep.Process.Pid), utils.NetNsType),
	}

	// the classifiers of lo in each namespace, as the dump reports them
	var dumped map[string][]tc.ProgramDump
	err = retry(t, func() error {
		var err error
		dumped, err = dumpLoopbackPrograms(p, namespaces)
		return err
	}, backoff.WithBackOff(backoff.NewConstantBackOff(200*time.Millisecond)), backoff.WithMaxTries(50))
	if err != nil {
		t.Fatal(err)
	}

	var programIDs []uint32
	for name, nsPath := range namespaces {
		for _, direction := range []manager.TrafficType{manager.Egress, manager.Ingress} {
			var expected []string
			// the raw packet classifier runs first in both modes
			if rawPacketSupported {
				expected = append(expected, "classifier_raw_packet_"+direction.String())
			}
			expected = append(expected, "classifier_"+direction.String())

			var programs []tc.ProgramDump
			for _, funcName := range expected {
				i := slices.IndexFunc(dumped[name], func(program tc.ProgramDump) bool { return program.FuncName == funcName })
				if !assert.GreaterOrEqual(t, i, 0, "%s isn't attached to lo in the %s namespace: %+v", funcName, name, dumped[name]) {
					continue
				}
				programs = append(programs, dumped[name][i])
				programIDs = append(programIDs, dumped[name][i].ProgramID)
			}
			if len(programs) != len(expected) {
				continue
			}

			var attached []link.AttachedProgram
			if tcxSupported {
				attached = tcxHookPrograms(t, nsPath, "lo", direction)
			}

			for position, program := range programs {
				assert.Equal(t, direction.String(), program.Direction)
				if !assert.NotNil(t, program.Position, "%s in the %s namespace has no position", program.FuncName, name) {
					continue
				}
				assert.Equal(t, position, *program.Position, "%s in the %s namespace", program.FuncName, name)

				if !tcxSupported {
					assert.Equal(t, manager.TCAttachModeLegacy.String(), program.AttachMode)
					assert.Zero(t, program.LinkID)
					continue
				}

				assert.Equal(t, manager.TCAttachModeTCX.String(), program.AttachMode)
				if assert.Greater(t, len(attached), position, "%s hook of lo in the %s namespace: %+v", direction, name, attached) {
					assert.Equal(t, ebpf.ProgramID(program.ProgramID), attached[position].ID, "%s in the %s namespace", program.FuncName, name)
					linkID, ok := attached[position].LinkID()
					assert.True(t, ok, "%s in the %s namespace isn't attached with a link", program.FuncName, name)
					assert.Equal(t, uint32(linkID), program.LinkID, "%s in the %s namespace", program.FuncName, name)
				}
			}
		}

		ingressExists, egressExists, err := tcFiltersExist(nsPath, "lo", "classifier_ingress", "classifier_egress")
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, !tcxSupported, ingressExists, "ingress tc filter of lo in the %s namespace", name)
		assert.Equal(t, !tcxSupported, egressExists, "egress tc filter of lo in the %s namespace", name)
	}

	nsID, err := namespaces["new"].GetNSID()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Manager.Get().CleanupNetworkNamespace(nsID); err != nil {
		t.Fatal(err)
	}

	// no zombie check here
	test.ValidateEndOfTest(false)
	test.CloseTestAndMonitor()
	testMod = nil // force a full testModule reinitialization
	testModuleCleanedUp = true

	for name, nsPath := range namespaces {
		if tcxSupported {
			for _, direction := range []manager.TrafficType{manager.Egress, manager.Ingress} {
				for _, program := range tcxHookPrograms(t, nsPath, "lo", direction) {
					assert.NotContains(t, programIDs, uint32(program.ID), "a classifier is left on the %s hook of lo in the %s namespace", direction, name)
				}
			}
		}

		ingressExists, egressExists, err := tcFiltersExist(nsPath, "lo", "classifier_ingress", "classifier_egress")
		if err != nil {
			t.Fatal(err)
		}
		assert.False(t, ingressExists, "ingress tc filter of lo left in the %s namespace", name)
		assert.False(t, egressExists, "egress tc filter of lo left in the %s namespace", name)
	}

	if err := test.CheckZombieProcesses(); err != nil {
		t.Errorf("failed checking for zombie processes: %v", err)
	}
}

// dumpLoopbackPrograms returns the classifiers of lo in each of the provided namespaces, from a network namespace dump
func dumpLoopbackPrograms(p *sprobe.EBPFProbe, namespaces map[string]*utils.NSPath) (map[string][]tc.ProgramDump, error) {
	dump, err := dumpNetworkNamespaces(p, true)
	if err != nil {
		return nil, err
	}

	programs := make(map[string][]tc.ProgramDump)
	for name, nsPath := range namespaces {
		nsID, err := nsPath.GetNSID()
		if err != nil {
			return nil, err
		}

		i := slices.IndexFunc(dump, func(ns secnetns.NetworkNamespaceDump) bool { return ns.NsID == nsID })
		if i < 0 {
			return nil, fmt.Errorf("the %s namespace %d isn't in the dump", name, nsID)
		}
		j := slices.IndexFunc(dump[i].Devices, func(device secnetns.NetworkDeviceDump) bool { return device.IfName == "lo" })
		if j < 0 {
			return nil, fmt.Errorf("lo isn't in the dump of the %s namespace %d", name, nsID)
		}
		if len(dump[i].Devices[j].Programs) == 0 {
			return nil, fmt.Errorf("no classifier on lo in the %s namespace %d yet", name, nsID)
		}
		programs[name] = dump[i].Devices[j].Programs
	}
	return programs, nil
}

// dumpNetworkNamespaces returns the network namespaces held by the resolver, with their devices if snapshotInterfaces
func dumpNetworkNamespaces(p *sprobe.EBPFProbe, snapshotInterfaces bool) ([]secnetns.NetworkNamespaceDump, error) {
	resp := p.Resolvers.NamespaceResolver.DumpNetworkNamespaces(&api.DumpNetworkNamespaceParams{SnapshotInterfaces: snapshotInterfaces})
	defer os.Remove(resp.GetDumpFilename())
	defer os.Remove(resp.GetGraphFilename())
	if resp.GetError() != "" {
		return nil, errors.New(resp.GetError())
	}

	data, err := os.ReadFile(resp.GetDumpFilename())
	if err != nil {
		return nil, err
	}
	var dump []secnetns.NetworkNamespaceDump
	if err := json.Unmarshal(data, &dump); err != nil {
		return nil, err
	}
	return dump, nil
}

// tcxHookPrograms returns the programs of a TCX hook of an interface, in the order in which they run
func tcxHookPrograms(tb testing.TB, nsPath *utils.NSPath, ifName string, direction manager.TrafficType) []link.AttachedProgram {
	tb.Helper()

	attach := ebpf.AttachTCXEgress
	if direction == manager.Ingress {
		attach = ebpf.AttachTCXIngress
	}

	var programs []link.AttachedProgram
	// TCX queries resolve the ifindex in the network namespace of the caller
	err := runInTestNetns(nsPath, func() error {
		device, err := netlink.LinkByName(ifName)
		if err != nil {
			return err
		}
		result, err := link.QueryPrograms(link.QueryOptions{Target: device.Attrs().Index, Attach: attach})
		if err != nil {
			return err
		}
		programs = result.Programs
		return nil
	})
	if err != nil {
		tb.Fatalf("couldn't query the %s TCX hook of %s: %v", direction, ifName, err)
	}
	return programs
}

// runInTestNetns runs fn from the network namespace nsPath
func runInTestNetns(nsPath *utils.NSPath, fn func() error) error {
	ns, err := os.Open(nsPath.GetPath())
	if err != nil {
		return err
	}
	defer ns.Close()

	return tc.RunInNetNS(ns, fn)
}

// staleFilterPID is above PID_MAX_LIMIT, so no process can have it
const staleFilterPID = 1<<22 + 1

var nonAlphanumeric = regexp.MustCompile("[^a-zA-Z0-9]")

// addStaleTCFilters adds to a host interface the filters of tcProbes that an agent that died would have left there, and
// returns their names. Their program returns TC_ACT_UNSPEC, so they don't change the traffic if they aren't cleaned up.
func addStaleTCFilters(tb testing.TB, ifName string, tcProbes []*manager.Probe) []string {
	tb.Helper()

	device, err := netlink.LinkByName(ifName)
	if err != nil {
		tb.Fatal(err)
	}
	nsID, err := getCurrentNetns()
	if err != nil {
		tb.Fatal(err)
	}

	qdisc := &netlink.GenericQdisc{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: device.Attrs().Index,
			Handle:    netlink.MakeHandle(0xffff, 0),
			Parent:    netlink.HANDLE_CLSACT,
		},
		QdiscType: "clsact",
	}
	if err := netlink.QdiscAdd(qdisc); err != nil && !errors.Is(err, unix.EEXIST) {
		tb.Fatalf("couldn't add a clsact qdisc to %s: %v", ifName, err)
	}

	prog, err := ebpf.NewProgram(&ebpf.ProgramSpec{
		Name:    "stale_classifier",
		Type:    ebpf.SchedCLS,
		License: "Apache-2.0",
		Instructions: asm.Instructions{
			asm.Mov.Imm(asm.R0, probes.TCActUnspec),
			asm.Return(),
		},
	})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		_ = prog.Close()
	})

	var names []string
	for _, tcProbe := range tcProbes {
		parent, section := uint32(netlink.HANDLE_MIN_EGRESS), "classifier/egress"
		if tcProbe.NetworkDirection == manager.Ingress {
			parent, section = netlink.HANDLE_MIN_INGRESS, "classifier/ingress"
		}

		key := tc.ProgramKey{
			UID:              tcProbe.UID,
			FuncName:         tcProbe.EBPFFuncName,
			IfIndex:          uint32(device.Attrs().Index),
			NetNS:            nsID,
			NetworkDirection: tcProbe.NetworkDirection,
		}
		// the name the eBPF manager gives the filter of a clone
		name := nonAlphanumeric.ReplaceAllString(fmt.Sprintf("%s_%s_%d", section, key.Key(), staleFilterPID), "_")

		filter := &netlink.BpfFilter{
			FilterAttrs: netlink.FilterAttrs{
				LinkIndex: device.Attrs().Index,
				Parent:    parent,
				Priority:  10,
				Protocol:  unix.ETH_P_ALL,
			},
			Fd:           prog.FD(),
			Name:         name,
			DirectAction: true,
		}
		if err := netlink.FilterAdd(filter); err != nil {
			tb.Fatalf("couldn't add filter %s to %s: %v", name, ifName, err)
		}
		names = append(names, name)
	}

	tb.Cleanup(func() {
		for _, parent := range []uint32{netlink.HANDLE_MIN_INGRESS, netlink.HANDLE_MIN_EGRESS} {
			filters, _ := netlink.FilterList(device, parent)
			for _, filter := range filters {
				if bpfFilter, ok := filter.(*netlink.BpfFilter); ok && slices.Contains(names, bpfFilter.Name) {
					_ = netlink.FilterDel(filter)
				}
			}
		}
	})

	return names
}

// tcFilterNames returns the names of the bpf filters of a host interface
func tcFilterNames(ifName string) ([]string, error) {
	device, err := netlink.LinkByName(ifName)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, parent := range []uint32{netlink.HANDLE_MIN_INGRESS, netlink.HANDLE_MIN_EGRESS} {
		filters, err := netlink.FilterList(device, parent)
		if err != nil {
			return nil, err
		}
		for _, filter := range filters {
			if bpfFilter, ok := filter.(*netlink.BpfFilter); ok {
				names = append(names, bpfFilter.Name)
			}
		}
	}
	return names, nil
}

// lookupIPAtPort returns a lookup that sends its A queries to a local port, whether or not a server answers them
func lookupIPAtPort(port uint16) func(name string) error {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))))
		},
	}

	return func(name string) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = resolver.LookupIP(ctx, "ip4", name)
		return nil
	}
}
