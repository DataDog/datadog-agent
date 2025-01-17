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

	"github.com/cilium/ebpf"
	"github.com/docker/docker/libnetwork/resolvconf"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes/rawpacket"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/tests/testutils"
)

func TestNetworkCIDR(t *testing.T) {
	SkipIfNotAvailable(t)

	checkNetworkCompatibility(t)

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
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "dns", event.GetType(), "wrong event type")
			assert.Equal(t, "google.com", event.DNS.Name, "wrong domain name")

			test.validateDNSSchema(t, event)
		})
	})
}

func isRawPacketNotSupported(kv *kernel.Version) bool {
	// OpenSUSE distributions are missing the dummy kernel module
	return probe.IsRawPacketNotSupported(kv) || kv.IsSLESKernel() || kv.IsOpenSUSELeapKernel()
}

func TestRawPacket(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "network feature", isRawPacketNotSupported)

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

func TestRawPacketFilter(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "RHEL, SLES, SUSE, AWS, Ubuntu and Oracle kernels", isRawPacketNotSupported)

	colSpecForMaps := ebpf.CollectionSpec{
		Maps: map[string]*ebpf.MapSpec{
			"raw_packet_event": {
				Name:       "raw_packet_event",
				Type:       ebpf.Array,
				KeySize:    4,
				ValueSize:  4096, // to be adapted with the raw_packet_event
				MaxEntries: 1,
			},
			"classifier_router": {
				Name:       "classifier_router",
				Type:       ebpf.ProgramArray,
				KeySize:    4,
				ValueSize:  4,
				MaxEntries: 1,
			},
		},
	}

	mapsCol, err := ebpf.NewCollection(&colSpecForMaps)
	assert.Nil(t, err)
	defer mapsCol.Close()

	rawPacketEventMap := mapsCol.Maps["raw_packet_event"]
	assert.NotNil(t, rawPacketEventMap)
	assert.Greater(t, rawPacketEventMap.FD(), 0)

	clsRouterMapFd := mapsCol.Maps["classifier_router"]
	assert.NotNil(t, clsRouterMapFd)
	assert.Greater(t, clsRouterMapFd.FD(), 0)

	filters := []rawpacket.Filter{
		{
			BPFFilter: "port 5555",
		},
		{
			BPFFilter: "tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x47455420",
		},
		{
			BPFFilter: "icmp[icmptype] != icmp-echo and icmp[icmptype] != icmp-echoreply",
		},
		{
			BPFFilter: "port ftp or ftp-data",
		},
		{
			BPFFilter: "tcp[tcpflags] & (tcp-syn|tcp-fin) != 0 and not src and dst net 192.168.1.0/24",
		},
		{
			BPFFilter: "tcp port 80 and (((ip[2:2] - ((ip[0]&0xf)<<2)) - ((tcp[12]&0xf0)>>2)) != 0)",
		},
		{
			BPFFilter: "ether[0] & 1 = 0 and ip[16] >= 224",
		},
		{
			BPFFilter: "udp port 67 and port 68",
		},
		{
			BPFFilter: "((port 67 or port 68) and (udp[38:4] = 0x3e0ccf08))",
		},
		{
			BPFFilter: "portrange 21-23",
		},
		{
			BPFFilter: "tcp[13] & 8!=0",
		},
	}

	runTest := func(t *testing.T, filters []rawpacket.Filter, opts rawpacket.ProgOpts) {
		progSpecs, err := rawpacket.FiltersToProgramSpecs(rawPacketEventMap.FD(), clsRouterMapFd.FD(), filters, opts)
		assert.Nil(t, err)
		assert.NotEmpty(t, progSpecs)

		colSpec := ebpf.CollectionSpec{
			Programs: make(map[string]*ebpf.ProgramSpec),
		}
		for _, progSpec := range progSpecs {
			colSpec.Programs[progSpec.Name] = progSpec
		}

		progsCol, err := ebpf.NewCollection(&colSpec)
		assert.Nil(t, err)
		if err == nil {
			progsCol.Close()
		}
	}

	for _, filter := range filters {
		t.Run(filter.BPFFilter, func(t *testing.T) {
			runTest(t, []rawpacket.Filter{filter}, rawpacket.DefaultProgOpts)
		})
	}

	t.Run("all-without-limit", func(t *testing.T) {
		runTest(t, filters, rawpacket.DefaultProgOpts)
	})

	t.Run("all-with-limit", func(t *testing.T) {
		opts := rawpacket.DefaultProgOpts
		opts.MaxProgSize = 4000
		opts.NopInstLen = 3500
		runTest(t, filters, opts)
	})
}
