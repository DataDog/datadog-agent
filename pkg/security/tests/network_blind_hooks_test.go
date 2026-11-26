// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"testing"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/netns"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/tests/testutils"
)

var _ = declare(TestTCBlindHooks, testOpts{networkIngressEnabled: true})

func TestTCBlindHooks(t *testing.T) {
	SkipIfNotAvailable(t)

	checkNetworkCompatibility(t)

	// the test changes the hooks of the host interfaces
	if testEnvironment == DockerEnvironment {
		t.Skip("skipping blind hooks test in docker")
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	// enables the IMDS events that the blind spot hides
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_blind_hooks_imds_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "aws" && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := testutils.RemoveDummyInterface(dummy); err != nil {
			t.Error(err)
		}
	}()

	imdsServerAddr := testutils.IMDSTestServerIP + ":" + strconv.Itoa(testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err := testutils.StopIMDSserver(imdsServer); err != nil {
			t.Error(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	if _, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe); !ok {
		t.Skip("not supported")
	}

	t.Run("cilium_tc_filter", func(t *testing.T) {
		device := createTestDevice(t, "csm_blind0")
		addForeignTCFilter(t, device, manager.Ingress, 1, "cil_from_netdev-csm_blind0")

		hook := requireBlindHook(t, test, device, manager.Ingress)
		assert.Equal(t, tc.ProgramAhead{Name: "cil_from_netdev-csm_blind0", Reason: tc.BlindReasonCiliumTCFilter}, hook.Program)

		deleteTCFilters(t, device, manager.Ingress)

		_, found := findBlindHook(detectBlindHooks(t, test), device, manager.Ingress)
		assert.False(t, found, "nothing runs ahead of the classifiers once the filters are gone")
	})

	t.Run("cilium_tcx", func(t *testing.T) {
		device := createTestDevice(t, "csm_blind1")
		attachForeignTCXProgram(t, device, manager.Egress, "cil_to_netdev")
		deleteTCFilters(t, device, manager.Egress)

		hook := requireBlindHook(t, test, device, manager.Egress)
		assert.Equal(t, tc.ProgramAhead{Name: "cil_to_netdev", Reason: tc.BlindReasonCiliumTCX}, hook.Program)
	})

	t.Run("unknown_tc_filter", func(t *testing.T) {
		device := createTestDevice(t, "csm_blind2")
		addForeignTCFilter(t, device, manager.Ingress, 1, "foreign_filter")

		hook := requireBlindHook(t, test, device, manager.Ingress)
		assert.Equal(t, tc.ProgramAhead{Name: "foreign_filter", Reason: tc.BlindReasonUnknownTCFilter}, hook.Program)
	})

	t.Run("unknown_tcx", func(t *testing.T) {
		device := createTestDevice(t, "csm_blind3")
		attachForeignTCXProgram(t, device, manager.Egress, "foreign_prog")

		hook := requireBlindHook(t, test, device, manager.Egress)
		assert.Equal(t, tc.ProgramAhead{Name: "foreign_prog", Reason: tc.BlindReasonUnknownTCX}, hook.Program)
	})

	// records the blind spot of legacy mode: the IMDS requests sent over lo go unseen
	t.Run("legacy_blind_spot", func(t *testing.T) {
		lo, err := netlink.LinkByName("lo")
		if err != nil {
			t.Fatal(err)
		}

		sendIMDSRequest := func() error {
			response, err := http.Get(fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL))
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %w", err)
			}
			return response.Body.Close()
		}
		isTestRequest := func(event *model.Event) bool {
			return event.IMDS.Type == "request" && event.IMDS.URL == testutils.IMDSSecurityCredentialsURL
		}

		if err := test.GetProbeEvent(sendIMDSRequest, isTestRequest, 5*time.Second, model.IMDSEventType); err != nil {
			t.Fatalf("the IMDS request wasn't captured before adding the foreign filters: %v", err)
		}

		addForeignTCFilter(t, lo, manager.Ingress, 1, "cil_from_netdev-lo")
		addForeignTCFilter(t, lo, manager.Egress, 1, "cil_to_netdev-lo")

		hooks := detectBlindHooks(t, test)
		for _, direction := range []manager.TrafficType{manager.Ingress, manager.Egress} {
			hook, found := findBlindHook(hooks, lo, direction)
			if assert.True(t, found, "the %s hook of lo isn't reported as blind", direction) {
				assert.Equal(t, tc.BlindReasonCiliumTCFilter, hook.Program.Reason)
			}
		}

		err = test.GetProbeEvent(sendIMDSRequest, isTestRequest, 3*time.Second, model.IMDSEventType)
		if _, ok := err.(ErrTimeout); !ok {
			t.Fatalf("expected the foreign filters to hide the IMDS request from the classifiers, got: %v", err)
		}
	})
}

// detectBlindHooks scans the hooks of the host interfaces, and checks that the scan counted no error, which also holds
// on kernels without TCX
func detectBlindHooks(tb testing.TB, test *testModule) []netns.BlindHook {
	tb.Helper()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		tb.Fatal("not supported")
	}

	test.statsdClient.Flush()
	hooks := p.Resolvers.NamespaceResolver.DetectBlindHooks()
	test.eventMonitor.SendStats()

	for _, errorType := range []string{"filter_list", "tcx_query"} {
		assert.Zero(tb, test.statsdClient.Get(metrics.MetricNamespaceResolverError+":error_type:"+errorType), "%s errors", errorType)
	}

	return hooks
}

func findBlindHook(hooks []netns.BlindHook, device netlink.Link, direction manager.TrafficType) (netns.BlindHook, bool) {
	for _, hook := range hooks {
		if hook.Device == device.Attrs().Name && hook.Direction == direction {
			return hook, true
		}
	}
	return netns.BlindHook{}, false
}

func requireBlindHook(tb testing.TB, test *testModule, device netlink.Link, direction manager.TrafficType) netns.BlindHook {
	tb.Helper()

	hooks := detectBlindHooks(tb, test)
	hook, found := findBlindHook(hooks, device, direction)
	if !found {
		tb.Fatalf("the %s hook of %s isn't reported as blind: %+v", direction, device.Attrs().Name, hooks)
	}
	return hook
}
