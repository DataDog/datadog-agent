// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/libnetwork/resolvconf"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/tests/testutils"
)

func TestNetworkCIDR(t *testing.T) {
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

	// write the rules using the local resolv.conf file
	resolvFile, err := resolvconf.GetSpecific("/etc/resolv.conf")
	if err != nil {
		t.Fatal(err)
	}
	nameserversCIDR := resolvconf.GetNameserversAsPrefix(resolvFile.Content)

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`dns.question.type == A && dns.question.name == "google.com" && process.file.name == "testsuite" && network.destination.ip in [%s]`, strings.Join(lo.Map(nameserversCIDR, func(p netip.Prefix, _ int) string { return p.String() }), ", ")),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("dns", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			_, err = net.LookupIP("google.com")
			if err != nil {
				return err
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "dns", event.GetType(), "wrong event type")
			assert.Equal(t, "google.com", event.DNS.Name, "wrong domain name")

			test.validateDNSSchema(t, event)
		})
	})
}

func TestRawPacket(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "RHEL, SLES, SUSE and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		// OpenSUSE distributions are missing the dummy kernel module
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel() || kv.IsOpenSUSELeapKernel()
	})

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s, %v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	testDestIP := "192.168.172.171"
	testUDPDestPort := uint16(12345)

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testDestIP+"/32")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	rule := &rules.RuleDefinition{
		ID:         "test_rule_raw_packet_udp4",
		Expression: fmt.Sprintf(`packet.filter == "ip dst %s and udp dst port %d" && process.file.name == "%s"`, testDestIP, testUDPDestPort, filepath.Base(executable)),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, withStaticOpts(testOpts{networkRawPacketEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("udp4", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			conn, err := net.Dial("udp4", fmt.Sprintf("%s:%d", testDestIP, testUDPDestPort))
			if err != nil {
				return err
			}
			defer conn.Close()

			_, err = conn.Write([]byte("hello"))
			if err != nil {
				return err
			}

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "packet", event.GetType(), "wrong event type")
			assertTriggeredRule(t, rule, "test_rule_raw_packet_udp4")
			assertFieldEqual(t, event, "packet.l3_protocol", int(model.EthPIP))
			_, expectedIPNet, err := net.ParseCIDR(testDestIP + "/32")
			if err != nil {
				t.Fatal(err)
			}
			assertFieldEqual(t, event, "packet.destination.ip", *expectedIPNet)
			assertFieldEqual(t, event, "packet.l4_protocol", int(model.IPProtoUDP))
			assertFieldEqual(t, event, "packet.destination.port", int(testUDPDestPort))
		})
	})
}
