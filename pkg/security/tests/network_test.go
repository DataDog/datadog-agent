// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/cilium/ebpf"
	"github.com/docker/docker/libnetwork/resolvconf"
	"github.com/oliveagle/jsonpath"
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
		test.WaitSignalFromRule(t, func() error {
			_, err = net.LookupIP("google.com")
			if err != nil {
				return err
			}
			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "dns", event.GetType(), "wrong event type")
			assert.Equal(t, "google.com", event.DNS.Question.Name, "wrong domain name")

			test.validateDNSSchema(t, event)
		}, "test_rule")
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

	_, expectedIPNet, err := net.ParseCIDR(testDestIP + "/32")
	if err != nil {
		t.Fatal(err)
	}

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

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_raw_packet_udp4",
			Expression: fmt.Sprintf(`packet.filter == "ip dst %s and udp dst port %d" && process.file.name == "%s"`, testDestIP, testUDPDestPort, filepath.Base(executable)),
		},
		{
			ID:         "test_rule_raw_packet_icmp",
			Expression: `packet.filter == "icmp and icmp[icmptype] == icmp-echo and ip dst 8.8.8.8"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{networkRawPacketEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("udp4", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			conn, err := net.Dial("udp4", net.JoinHostPort(testDestIP, strconv.Itoa(int(testUDPDestPort))))
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
			assertFieldEqual(t, event, "packet.destination.ip", *expectedIPNet)
			assertFieldEqual(t, event, "packet.l4_protocol", int(model.IPProtoUDP))
			assertFieldEqual(t, event, "packet.destination.port", int(testUDPDestPort))
		}, "test_rule_raw_packet_udp4")
	})

	t.Run("icmp", func(t *testing.T) {
		if _, err := whichNonFatal("docker"); err != nil {
			t.Skip("Skip test where docker is unavailable")
		}

		wrapper, err := newDockerCmdWrapper(test.Root(), test.Root(), "busybox", "")
		if err != nil {
			t.Fatalf("failed to start docker wrapper: %v", err)
		}

		waitSignal := test.WaitSignalWithoutProcessContext

		kv, err := kernel.NewKernelVersion()
		if err != nil {
			t.Errorf("failed to get kernel version: %s", err)
			return
		}

		if !kv.HasBpfGetSocketCookieForCgroupSocket() || kv.Code < kernel.Kernel5_15 {
			waitSignal = test.WaitSignalWithoutProcessContext
		}

		wrapper.Run(t, "ping", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			waitSignal(t, func() error {
				cmd := cmdFunc("/bin/ping", []string{"-c", "1", "8.8.8.8"}, nil)
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assert.Equal(t, "test_rule_raw_packet_icmp", rule.ID, "wrong rule triggered")
				assert.Equal(t, "8.8.8.8/32", event.NetworkContext.Destination.IPNet.String(), "wrong destination IP")
				assert.Equal(t, uint16(model.IPProtoICMP), event.RawPacket.L4Protocol)
				assert.Equal(t, uint32(model.ICMPTypeEchoRequest), event.RawPacket.NetworkContext.Type)
			})
		})
	})
}

func TestRawPacketAction(t *testing.T) {
	if testEnvironment == DockerEnvironment {
		t.Skip("skipping cgroup ID test in docker")
	}

	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "network feature", isRawPacketNotSupported)

	rule := &rules.RuleDefinition{
		ID:         "test_rule_raw_packet_drop",
		Expression: `exec.file.name == "free"`,
		Actions: []*rules.ActionDefinition{
			{
				NetworkFilter: &rules.NetworkFilterDefinition{
					BPFFilter: "port 53",
					Scope:     "cgroup",
					Policy:    "drop",
				},
			},
		},
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, withStaticOpts(testOpts{networkRawPacketEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	cmdWrapper, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer cmdWrapper.stop()

	t.Run("drop", func(t *testing.T) {
		cmd := cmdWrapper.Command("nslookup", []string{"google.com"}, []string{})
		if err := cmd.Run(); err != nil {
			t.Error(err)
		}

		test.WaitSignalFromRule(t, func() error {
			cmd := cmdWrapper.Command("free", []string{}, []string{})
			return cmd.Run()
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_raw_packet_drop")
		}, "test_rule_raw_packet_drop")

		err = retry.Do(func() error {
			msg := test.msgSender.getMsg("test_rule_raw_packet_drop")
			if msg == nil {
				return errors.New("not found")
			}
			validateMessageSchema(t, string(msg.Data))

			jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.policy == 'drop')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.filter == 'port 53')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.status == 'performed')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}

			})

			return nil
		}, retry.Delay(200*time.Millisecond), retry.Attempts(60), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)

		// wait for the action to be performed
		time.Sleep(5 * time.Second)

		cmd = cmdWrapper.Command("nslookup", []string{"microsoft.com"}, []string{})
		if err = cmd.Run(); err == nil {
			t.Error("should return an error")
		}

		err = retry.Do(func() error {
			msg := test.msgSender.getMsg("rawpacket_action")
			if msg == nil {
				return errors.New("not found")
			}
			validateRawPacketActionSchema(t, string(msg.Data))

			return nil
		}, retry.Delay(500*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	})
}

func TestRawPacketActionWithSignature(t *testing.T) {
	if testEnvironment == DockerEnvironment {
		t.Skip("skipping cgroup ID test in docker")
	}

	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "network feature", isRawPacketNotSupported)

	// Initial rule to capture the signature - no action yet
	// Include a DNS rule to ensure network probes (including cgroup socket hooks) are attached from the start.
	// This is necessary because without a network event type in the initial ruleset, the cgroup socket
	// hooks (hook_sock_create, hook_sock_release) are not attached. These hooks populate the sock_cookie_pid
	// map which is needed for PID resolution in the TC classifier on kernels < 6.1.
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_capture_signature",
			Expression: `exec.file.name == "free"`,
		},
		{
			ID:         "test_dns_to_activate_network_probes",
			Expression: `dns.question.name == "never.match.example.com"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{networkRawPacketEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	cmdWrapper, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer cmdWrapper.stop()

	var capturedSignature string

	// First, run "free" to capture the signature
	test.WaitSignalFromRule(t, func() error {
		cmd := cmdWrapper.Command("free", []string{}, []string{})
		return cmd.Run()
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_rule_capture_signature")
		capturedSignature = event.FieldHandlers.ResolveSignature(event)
	}, "test_rule_capture_signature")

	if capturedSignature == "" {
		t.Fatal("captured signature is empty")
	}

	// Verify DNS works before applying the filter
	cmd := cmdWrapper.Command("nslookup", []string{"google.com"}, []string{})
	if err := cmd.Run(); err != nil {
		t.Errorf("nslookup should work before filter: %v", err)
	}

	// Now create a new rule with the network filter action using the captured signature
	newRuleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_capture_signature",
			Expression: `exec.file.name == "free" && event.signature != "` + capturedSignature + `"`,
		},
		{
			ID:         "test_rule_raw_packet_drop_with_signature",
			Expression: `exec.file.name == "free" && event.signature == "` + capturedSignature + `"`,
			Actions: []*rules.ActionDefinition{
				{
					NetworkFilter: &rules.NetworkFilterDefinition{
						BPFFilter: "port 53",
						Scope:     "cgroup",
						Policy:    "drop",
					},
				},
			},
		},
	}

	// Set the new policy and reload
	if err := setTestPolicy(commonCfgDir, nil, newRuleDefs); err != nil {
		t.Fatalf("failed to set new policy: %v", err)
	}
	if err := test.reloadPolicies(); err != nil {
		t.Fatalf("failed to reload policies: %v", err)
	}
	// Trigger a small event to force the replay of cached events.
	// The replay only happens in handleEvent when a new eBPF event arrives.
	exec.Command("true").Run()

	// Run free again to trigger the rule with signature (free is not long-running, so we need to run it again)
	err = test.GetEventSent(t, func() error {
		cmd := cmdWrapper.Command("free", []string{}, []string{})
		return cmd.Run()
	}, func(rule *rules.Rule, event *model.Event) bool {
		assertTriggeredRule(t, rule, "test_rule_raw_packet_drop_with_signature")

		// Verify the network filter action was performed using the event's action reports
		assert.Equal(t, 1, len(event.ActionReports), "expected one action report")
		if len(event.ActionReports) == 1 {
			report := event.ActionReports[0]
			if rawPacketReport, ok := report.(*probe.RawPacketActionReport); ok {
				assert.Equal(t, "port 53", rawPacketReport.Filter, "unexpected filter")
				assert.Equal(t, "drop", rawPacketReport.Policy, "unexpected policy")
			}
		}
		return true
	}, 10*time.Second, "test_rule_raw_packet_drop_with_signature")
	if err != nil {
		t.Fatal(err)
	}

	// Wait for the action to be performed
	time.Sleep(5 * time.Second)

	// DNS should now be blocked for this container
	cmd = cmdWrapper.Command("nslookup", []string{"microsoft.com"}, []string{})
	_, nslookupErr := cmd.CombinedOutput()
	if nslookupErr == nil {
		t.Error("nslookup should return an error after filter is applied")
	}

	// Verify the raw packet action event was sent
	err = retry.Do(func() error {
		msg := test.msgSender.getMsg("rawpacket_action")
		if msg == nil {
			return errors.New("not found")
		}
		validateRawPacketActionSchema(t, string(msg.Data))
		return nil
	}, retry.Delay(500*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
	assert.NoError(t, err)

	// Now remove the network isolation rule and verify DNS works again
	// Create a new policy without the network filter action
	ruleDefsWithoutFilter := []*rules.RuleDefinition{
		{
			ID:         "test_rule_capture_signature",
			Expression: `exec.file.name == "free"`,
		},
	}

	// Set the new policy without network filter and reload
	if err := setTestPolicy(commonCfgDir, nil, ruleDefsWithoutFilter); err != nil {
		t.Fatalf("failed to set policy without filter: %v", err)
	}
	if err := test.reloadPolicies(); err != nil {
		t.Fatalf("failed to reload policies: %v", err)
	}

	// Wait for the filter to be removed
	time.Sleep(2 * time.Second)

	// DNS should now work again in the container
	cmd = cmdWrapper.Command("nslookup", []string{"example.com"}, []string{})
	if err := cmd.Run(); err != nil {
		t.Errorf("nslookup should work after removing network filter: %v", err)
	}
}

func TestRawPacketActionProcessScopeWithSignature(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "network feature", isRawPacketNotSupported)

	// Use a local UDP port for testing - no external network dependency
	const udpTestPort = "5555"

	// Initial rule to capture signature when syscall_tester starts
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_capture_signature",
			Expression: `exec.file.name == "syscall_tester"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{networkRawPacketEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	// Channel for reading lines from stdout - single goroutine reads, avoids race
	linesCh := make(chan string, 100)
	linesErrCh := make(chan error, 1)

	// Start a single reader goroutine (will be started after we have the stdout pipe)
	startLineReader := func(reader *bufio.Reader) {
		go func() {
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					linesErrCh <- err
					return
				}
				linesCh <- line
			}
		}()
	}

	// Helper function to drain all buffered lines and return the last UDP status
	drainAndReadUDPStatus := func(timeout time.Duration) (udpOK bool, err error) {
		deadline := time.Now().Add(timeout)
		foundAny := false

		for time.Now().Before(deadline) {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				break
			}
			select {
			case line := <-linesCh:
				if strings.Contains(line, "UDP_OK") {
					udpOK = true
					foundAny = true
				} else if strings.Contains(line, "UDP_FAIL") {
					udpOK = false
					foundAny = true
				}
			case <-linesErrCh:
				if foundAny {
					return udpOK, nil
				}
				return false, errors.New("reader error")
			case <-time.After(remaining):
				if foundAny {
					return udpOK, nil
				}
				return false, errors.New("timeout reading UDP status")
			}
		}
		if foundAny {
			return udpOK, nil
		}
		return false, errors.New("timeout reading UDP status")
	}

	// Start udploop in background and capture the signature
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var udploopCmd *exec.Cmd
	var stdout io.ReadCloser
	var capturedSignature string

	test.WaitSignalFromRule(t, func() error {
		udploopCmd = exec.CommandContext(ctx, syscallTester, "udploop", udpTestPort)
		var err error
		stdout, err = udploopCmd.StdoutPipe()
		if err != nil {
			return err
		}
		return udploopCmd.Start()
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_rule_capture_signature")
		capturedSignature = event.FieldHandlers.ResolveSignature(event)
	}, "test_rule_capture_signature")

	defer func() {
		cancel()
		if udploopCmd != nil {
			udploopCmd.Wait()
		}
	}()

	if capturedSignature == "" {
		t.Fatal("captured signature is empty")
	}

	reader := bufio.NewReader(stdout)
	startLineReader(reader)

	// Step 1: Verify UDP works before isolation
	udpOK, err := drainAndReadUDPStatus(10 * time.Second)
	if err != nil || !udpOK {
		t.Fatalf("UDP should work before isolation: udpOK=%v, err=%v", udpOK, err)
	}

	// Step 2: Apply network isolation rule with signature matching
	newRuleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_capture_signature",
			Expression: `exec.file.name == "syscall_tester" && event.signature != "` + capturedSignature + `"`,
		},
		{
			ID:         "test_rule_syscall_tester_isolate",
			Expression: `exec.file.name == "syscall_tester" && event.signature == "` + capturedSignature + `"`,
			Actions: []*rules.ActionDefinition{
				{
					NetworkFilter: &rules.NetworkFilterDefinition{
						BPFFilter: "port " + udpTestPort,
						Scope:     "process",
						Policy:    "drop",
					},
				},
			},
		},
	}

	if err := setTestPolicy(commonCfgDir, nil, newRuleDefs); err != nil {
		t.Fatalf("failed to set policy with network filter: %v", err)
	}
	if err := test.reloadPolicies(); err != nil {
		t.Fatalf("failed to reload policies: %v", err)
	}
	// Trigger a small event to force the replay of cached events.
	// The replay only happens in handleEvent when a new eBPF event arrives.
	exec.Command("true").Run()

	// Wait for the filter to be applied
	time.Sleep(3 * time.Second)

	// Step 3: Verify UDP fails for the now-isolated process
	udpOK, err = drainAndReadUDPStatus(10 * time.Second)
	if err == nil && udpOK {
		t.Error("Step 3: UDP should fail for isolated process")
	}

	// Step 4: Remove the network isolation rule
	ruleDefsWithoutFilter := []*rules.RuleDefinition{
		{
			ID:         "test_rule_capture_signature",
			Expression: `exec.file.name == "syscall_tester"`,
		},
	}

	if err := setTestPolicy(commonCfgDir, nil, ruleDefsWithoutFilter); err != nil {
		t.Fatalf("failed to set policy without filter: %v", err)
	}
	if err := test.reloadPolicies(); err != nil {
		t.Fatalf("failed to reload policies: %v", err)
	}

	// Wait for the filter to be removed
	time.Sleep(2 * time.Second)

	// Step 5: Verify UDP works again on the same process
	udpOK, err = drainAndReadUDPStatus(10 * time.Second)
	if err != nil || !udpOK {
		t.Errorf("Step 5: UDP should work after removing network filter: udpOK=%v, err=%v", udpOK, err)
	}
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
		assert.NoError(t, err)
		assert.NotEmpty(t, progSpecs)

		colSpec := ebpf.CollectionSpec{
			Programs: make(map[string]*ebpf.ProgramSpec),
		}
		for _, progSpec := range progSpecs {
			colSpec.Programs[progSpec.Name] = progSpec
		}

		progsCol, err := ebpf.NewCollection(&colSpec)
		assert.NoError(t, err)
		if err == nil {
			progsCol.Close()
		}
	}

	for _, filter := range filters {
		t.Run(filter.BPFFilter, func(t *testing.T) {
			runTest(t, []rawpacket.Filter{filter}, rawpacket.DefaultProgOpts())
		})
	}

	t.Run("all-without-limit", func(t *testing.T) {
		runTest(t, filters, rawpacket.DefaultProgOpts())
	})

	t.Run("all-with-limit", func(t *testing.T) {
		// kernels < 5.2 have a limit of 4k instructions for the eBPF program size
		checkKernelCompatibility(t, "Old debian kernels", func(kv *kernel.Version) bool {
			return kv.IsDebianKernel() && kv.Code < kernel.Kernel5_2
		})

		opts := rawpacket.DefaultProgOpts()
		opts.MaxProgSize = 4000
		opts.NopInstLen = 3500
		runTest(t, filters, opts)
	})
}

func TestNetworkFlowSendUDP4(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "RHEL, SLES, SUSE and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		// OpenSUSE distributions are missing the dummy kernel module
		return kv.IsSLESKernel() || kv.IsOpenSUSELeapKernel() || probe.IsNetworkFlowMonitorNotSupported(kv)
	})

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s, %v", string(out), err)
		}
	}

	testDestIP := "127.0.0.1"
	testUDPDestPort := 12345

	rule := &rules.RuleDefinition{
		ID:         "test_rule_network_flow",
		Expression: `network_flow_monitor.flows.length > 0 && process.file.name == "syscall_tester"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, withStaticOpts(
		testOpts{
			networkFlowMonitorEnabled: true,
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("test_network_flow_send_udp4", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "network_flow_send_udp4", testDestIP, strconv.Itoa(testUDPDestPort))
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "network_flow_monitor", event.GetType(), "wrong event type")
			assert.Equal(t, uint64(1), event.NetworkFlowMonitor.FlowsCount, "wrong FlowsCount")
			assert.Equal(t, 1, len(event.NetworkFlowMonitor.Flows), "wrong flows count")
			if len(event.NetworkFlowMonitor.Flows) > 0 {
				assert.Equal(t, testDestIP, event.NetworkFlowMonitor.Flows[0].Destination.IPNet.IP.To4().String(), "wrong destination IP")
				assert.Equal(t, uint16(testUDPDestPort), event.NetworkFlowMonitor.Flows[0].Destination.Port, "wrong destination Port")
				assert.Equal(t, uint16(model.IPProtoUDP), event.NetworkFlowMonitor.Flows[0].L4Protocol, "wrong L4 protocol")
				assert.Equal(t, uint16(model.EthPIP), event.NetworkFlowMonitor.Flows[0].L3Protocol, "wrong L3 protocol")
				assert.Equal(t, uint64(1), event.NetworkFlowMonitor.Flows[0].Egress.PacketCount, "wrong egress packet count")
				assert.Equal(t, uint64(46), event.NetworkFlowMonitor.Flows[0].Egress.DataSize, "wrong egress data size") // full packet size including l2 header
				assert.Equal(t, uint64(0), event.NetworkFlowMonitor.Flows[0].Ingress.PacketCount, "wrong ingress packet count")
				assert.Equal(t, uint64(0), event.NetworkFlowMonitor.Flows[0].Ingress.DataSize, "wrong ingress data size")
			}
		}, "test_rule_network_flow")
	})
}
