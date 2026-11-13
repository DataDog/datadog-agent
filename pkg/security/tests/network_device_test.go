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
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v7"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var _ = declare(TestNetDevice, testOpts{networkIngressEnabled: true})

func TestNetDevice(t *testing.T) {
	SkipIfNotAvailable(t)

	testNetDevice(t)
}

func testNetDevice(t *testing.T) {
	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel()
	})

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s, %v", string(out), err)
		}
	}

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `dns.question.type == A && dns.question.name == "google.com" && process.file.name == "testsuite"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	currentNetns, err := utils.NewNSPathFromPid(utils.Getpid(), utils.NetNsType).GetNSID()
	if err != nil {
		t.Errorf("couldn't retrieve current network namespace ID: %v", err)
	}
	var testNetns uint32
	executable := which(t, "ip")
	defer func() {
		_ = exec.Command(executable, "netns", "delete", "test_netns").Run()
		_ = exec.Command(executable, "link", "delete", "host-eth1").Run()
		_ = exec.Command(executable, "link", "delete", "host-eth0").Run()
	}()

	// those tests are dependent on each other, they can't be run in isolation

	// register_netdevice
	err = test.GetProbeEvent(func() error {
		cmd := exec.Command(executable, "netns", "add", "test_netns")
		if err = cmd.Run(); err != nil {
			return err
		}

		// retrieve new netnsid
		fi, err := os.Stat("/var/run/netns/test_netns")
		if err != nil {
			return err
		}
		stat, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			return errors.New("couldn't parse test_netns inum")
		}
		testNetns = uint32(stat.Ino)

		return nil
	}, func(event *model.Event) bool {
		if !assert.Equal(t, "net_device", event.GetType(), "wrong event type") {
			return true
		}

		return assert.Equal(t, "lo", event.NetDevice.Device.Name, "wrong interface name") &&
			assert.Equal(t, testNetns, event.NetDevice.Device.NetNS, "wrong network namespace ID")
	}, 3*time.Second, model.NetDeviceEventType)
	if err != nil {
		t.Error(err)
	}

	// veth_newlink
	err = test.GetProbeEvent(func() error {
		cmd := exec.Command(executable, "link", "add", "host-eth0", "type", "veth", "peer", "name", "ns-eth0", "netns", "test_netns")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("ip output: %s", string(out))
		}
		return err
	}, func(event *model.Event) bool {
		if !assert.Equal(t, "veth_pair", event.GetType(), "wrong event type") {
			return true
		}

		return assert.Equal(t, "host-eth0", event.VethPair.HostDevice.Name, "wrong interface name") &&
			assert.Equal(t, currentNetns, event.VethPair.HostDevice.NetNS, "wrong network namespace ID") &&
			assert.Equal(t, "ns-eth0", event.VethPair.PeerDevice.Name, "wrong peer interface name") &&
			assert.Equal(t, testNetns, event.VethPair.PeerDevice.NetNS, "wrong peer network namespace ID")
	}, 10*time.Second, model.VethPairEventType)
	if err != nil {
		t.Error(err)
	}

	// veth_newlink_dev_change_netns
	err = test.GetProbeEvent(func() error {
		cmd := exec.Command(executable, "link", "add", "host-eth1", "type", "veth", "peer", "name", "ns-eth1")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("couldn't create veth pair (out=%s): %w", string(out), err)
		}

		cmd = exec.Command(executable, "link", "set", "ns-eth1", "netns", "test_netns")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("couldn't set netns (out=%s): %w", string(out), err)
		}

		return nil
	}, func(event *model.Event) bool {
		if !assert.Equal(t, "veth_pair_ns", event.GetType(), "wrong event type") {
			return true
		}

		return assert.Equal(t, "host-eth1", event.VethPair.HostDevice.Name, "wrong interface name") &&
			assert.Equal(t, currentNetns, event.VethPair.HostDevice.NetNS, "wrong network namespace ID") &&
			assert.Equal(t, "ns-eth1", event.VethPair.PeerDevice.Name, "wrong peer interface name") &&
			assert.Equal(t, testNetns, event.VethPair.PeerDevice.NetNS, "wrong peer network namespace ID")
	}, 10*time.Second, model.VethPairNsEventType)
	if err != nil {
		t.Error(err)
	}
}

var _ = declare(TestNetNSMountRegistration, testOpts{networkIngressEnabled: true})

func TestNetNSMountRegistration(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel()
	})

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s, %v", string(out), err)
		}
	}

	// dummy rule to force the activation of netdev-related probes
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `dns.question.type == A`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Fatal("not supported")
	}

	executable := which(t, "ip")
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command(executable, args...).CombinedOutput(); err != nil {
			t.Fatalf("ip %s: %s, %v", strings.Join(args, " "), string(out), err)
		}
	}
	defer func() {
		_ = exec.Command(executable, "link", "delete", "wpnsfs0").Run()
		_ = exec.Command(executable, "netns", "delete", "wpnsfs_netns").Run()
	}()

	// nothing runs in the namespace once ip exits, so only its nsfs mount can get it registered
	run("netns", "add", "wpnsfs_netns")
	nsPath := utils.NewNSPathFromPath("/var/run/netns/wpnsfs_netns", utils.NetNsType)
	nsID := nsInode(t, nsPath.GetPath())

	err = retry(t, func() error {
		ns := p.Resolvers.NamespaceResolver.ResolveNetworkNamespace(nsID)
		if ns == nil {
			return fmt.Errorf("namespace %d isn't tracked", nsID)
		}
		handle, err := ns.GetNamespaceHandleDup()
		if err != nil {
			return fmt.Errorf("namespace %d has no handle: %w", nsID, err)
		}
		return handle.Close()
	}, backoff.WithBackOff(backoff.NewConstantBackOff(100*time.Millisecond)), backoff.WithMaxElapsedTime(5*time.Second))
	if err != nil {
		// tells a mount event that never made it to the mount resolver from a mount path that couldn't be opened
		var mountID uint32
		p.Resolvers.MountResolver.Iterate(func(m *model.Mount) {
			if m.GetFSType() == "nsfs" && uint32(m.RootPathKey.Inode) == nsID {
				mountID = m.MountID
			}
		})
		if mountID == 0 {
			t.Fatalf("%v, and the mount resolver has no nsfs mount of it", err)
		}
		mountPath, _, _, pathErr := p.Resolvers.MountResolver.ResolveMountPath(mountID, 0)
		t.Fatalf("%v, and the mount resolver has its nsfs mount %d at %q (%v)", err, mountID, mountPath, pathErr)
	}

	run("link", "add", "wpnsfs0", "type", "veth", "peer", "name", "wpnsfs1")
	run("link", "set", "wpnsfs1", "netns", "wpnsfs_netns")

	err = retry(t, func() error {
		ingress, egress, err := tcFiltersExist(nsPath, "wpnsfs1", "classifier_ingress", "classifier_egress")
		if err != nil {
			return err
		}
		if !ingress || !egress {
			return fmt.Errorf("classifiers on the moved device: ingress %t, egress %t", ingress, egress)
		}
		return nil
	}, backoff.WithBackOff(backoff.NewConstantBackOff(100*time.Millisecond)), backoff.WithMaxElapsedTime(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	// the veth pair only goes away with the namespace, so it shows that the handle was released on umount
	run("netns", "delete", "wpnsfs_netns")
	err = retry(t, func() error {
		if err := exec.Command(executable, "link", "show", "wpnsfs0").Run(); err == nil {
			return fmt.Errorf("namespace %d is still alive", nsID)
		}
		return nil
	}, backoff.WithBackOff(backoff.NewConstantBackOff(100*time.Millisecond)), backoff.WithMaxElapsedTime(20*time.Second))
	if err != nil {
		t.Error(err)
	}
}

var _ = declare(TestTCFilters, testOpts{networkIngressEnabled: true})

func TestTCFilters(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel()
	})

	// skip the test to avoid nested namespaces issues
	if testEnvironment == DockerEnvironment {
		t.Skip("skipping tc filters test in docker")
	}

	// dummy rule to force the activation of netdev-related probes
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `dns.question.type == A`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}

	var testModuleCleanedUp bool
	defer func() {
		if !testModuleCleanedUp {
			test.CloseTest()
		}
	}()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	sleepExecutable := which(t, "sleep")

	var newNetNSSleep *exec.Cmd

	t.Run("attach_detach_filters", func(t *testing.T) {
		newNetNSSleep = exec.Command(syscallTester, "new_netns_exec", sleepExecutable, "600")
		err := newNetNSSleep.Start()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			// stop the sleep process
			newNetNSSleep.Process.Kill()
			_ = newNetNSSleep.Wait()
		})

		sleepProcPid := uint32(newNetNSSleep.Process.Pid)
		if sleepProcPid == 0 {
			t.Fatal("pid of the sleep command is zero")
		}

		netNs := utils.NewNSPathFromPid(sleepProcPid, utils.NetNsType)
		// wait for the new net namespace to be created
		// and for the tc probes to be attached to the new interface
		time.Sleep(1 * time.Second)
		nsid, err := netNs.GetNSID()
		if err != nil {
			t.Fatal(err)
		}

		ingressExists, egressExists, err := tcFiltersExist(netNs, "lo", "classifier_ingress", "classifier_egress")
		if err != nil {
			t.Fatal(err)
		}

		if !ingressExists {
			t.Error("Ingress tc classifier does not exist")
		}
		if !egressExists {
			t.Fatal("Egress tc classifier does not exist")
		}

		p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
		if !ok {
			t.Fatal("not supported")
		}

		if err := p.Manager.Get().CleanupNetworkNamespace(nsid); err != nil {
			t.Fatal(err)
		}

		// no zombie check here
		test.ValidateEndOfTest(false)
		test.CloseTestAndMonitor()
		testMod = nil // force a full testModule reinitialization
		testModuleCleanedUp = true

		ingressExists, egressExists, err = tcFiltersExist(netNs, "lo", "classifier_ingress", "classifier_egress")
		if err != nil {
			t.Fatal(err)
		}

		if ingressExists {
			t.Error("Ingress tc classifier wasn't properly detached")
		}

		if egressExists {
			t.Fatal("Egress tc classifier wasn't properly detached")
		}
	})

	if err := test.CheckZombieProcesses(); err != nil {
		t.Errorf("failed checking for zombie processes: %v", err)
	}
}

func tcFiltersExist(netNs *utils.NSPath, linkName string, ingressFilterNamePrefix, egressFilterNamePrefix string) (ingressExists bool, egressExists bool, err error) {
	netNsFile, err := os.Open(netNs.GetPath())
	if err != nil {
		return
	}
	defer netNsFile.Close()

	netlinkHandle, err := netlink.NewHandleAt(netns.NsHandle(int(netNsFile.Fd())), unix.NETLINK_ROUTE)
	if err != nil {
		return
	}
	defer netlinkHandle.Close()

	link, err := netlinkHandle.LinkByName(linkName)
	if err != nil {
		return
	}

	bpfFilterExists := func(parentHandle uint32, expectedFilterNamePrefix string) (bool, error) {
		filters, err := netlinkHandle.FilterList(link, parentHandle)
		if err != nil {
			return false, err
		}

		var found bool
		bpfType := (&netlink.BpfFilter{}).Type()
		for _, elem := range filters {
			if elem.Type() != bpfType {
				continue
			}

			bpfFilter, ok := elem.(*netlink.BpfFilter)
			if !ok {
				continue
			}

			if strings.HasPrefix(bpfFilter.Name, expectedFilterNamePrefix) {
				found = true
				break
			}
		}
		return found, nil
	}

	ingressExists, err = bpfFilterExists(netlink.HANDLE_MIN_INGRESS, ingressFilterNamePrefix)
	if err != nil {
		return
	}

	egressExists, err = bpfFilterExists(netlink.HANDLE_MIN_EGRESS, egressFilterNamePrefix)
	return
}
