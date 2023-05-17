// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

package tests

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func TestNetDevice(t *testing.T) {
	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel()
	})

	if testEnvironment != DockerEnvironment && !config.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s, %v", string(out), err)
		}
	}

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `dns.question.type == A && dns.question.name == "google.com" && process.file.name == "testsuite"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	currentNetns, err := utils.NetNSPathFromPid(uint32(utils.Getpid())).GetProcessNetworkNamespace()
	if err != nil {
		t.Errorf("couldn't retrieve current network namespace ID: %v", err)
	}
	var testNetns uint32
	executable := which(t, "ip")
	defer func() {
		_ = exec.Command(executable, "netns", "delete", "test_netns").Run()
	}()

	t.Run("register_netdevice", func(t *testing.T) {
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
				return fmt.Errorf("couldn't parse test_netns inum")
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
	})

	t.Run("veth_newlink", func(t *testing.T) {
		err = test.GetProbeEvent(func() error {
			cmd := exec.Command(executable, "link", "add", "host-eth0", "type", "veth", "peer", "name", "ns-eth0", "netns", "test_netns")
			return cmd.Run()
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
	})

	t.Run("veth_newlink_dev_change_netns", func(t *testing.T) {
		err = test.GetProbeEvent(func() error {
			cmd := exec.Command(executable, "link", "add", "host-eth1", "type", "veth", "peer", "name", "ns-eth1")
			if err = cmd.Run(); err != nil {
				return fmt.Errorf("couldn't create veth pair: %v", err)
			}

			cmd = exec.Command(executable, "link", "set", "ns-eth1", "netns", "test_netns")
			return cmd.Run()
		}, func(event *model.Event) bool {
			if !assert.Equal(t, "veth_pair", event.GetType(), "wrong event type") {
				return true
			}

			return assert.Equal(t, "host-eth1", event.VethPair.HostDevice.Name, "wrong interface name") &&
				assert.Equal(t, currentNetns, event.VethPair.HostDevice.NetNS, "wrong network namespace ID") &&
				assert.Equal(t, "ns-eth1", event.VethPair.PeerDevice.Name, "wrong peer interface name") &&
				assert.Equal(t, testNetns, event.VethPair.PeerDevice.NetNS, "wrong peer network namespace ID")
		}, 10*time.Second, model.VethPairEventType)
		if err != nil {
			t.Error(err)
		}
	})
}
