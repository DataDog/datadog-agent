// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestTCXQuerier(t *testing.T) {
	for _, test := range []struct {
		name        string
		err         error
		expectedErr bool
		unsupported bool
	}{
		{name: "kernel without tcx", err: fmt.Errorf("query programs: %w", unix.EINVAL), unsupported: true},
		{name: "kernel without prog query", err: fmt.Errorf("query programs: %w", ebpf.ErrNotSupported), unsupported: true},
		{name: "interface gone", err: fmt.Errorf("query programs: %w", unix.ENODEV)},
		{name: "other error", err: fmt.Errorf("query programs: %w", unix.EPERM), expectedErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var queries int
			q := tcxQuerier{query: func(int, manager.TrafficType) ([]tc.TCXProgram, error) {
				queries++
				return nil, test.err
			}}

			for range 3 {
				programs, err := q.programs(1, manager.Ingress)
				assert.Empty(t, programs)
				if test.expectedErr {
					assert.ErrorIs(t, err, test.err)
				} else {
					assert.NoError(t, err)
				}
			}

			assert.Equal(t, test.unsupported, q.unsupported)
			if test.unsupported {
				assert.Equal(t, 1, queries, "no query should follow the one that showed tcx isn't supported")
			} else {
				assert.Equal(t, 3, queries)
			}
		})
	}

	t.Run("programs", func(t *testing.T) {
		expected := []tc.TCXProgram{{ID: 3, LinkID: 13}, {ID: 1, LinkID: 11}}
		q := tcxQuerier{query: func(int, manager.TrafficType) ([]tc.TCXProgram, error) {
			return expected, nil
		}}
		programs, err := q.programs(1, manager.Egress)
		require.NoError(t, err)
		assert.Equal(t, expected, programs)
		assert.False(t, q.unsupported)
	})
}

func TestHasPeerInOtherNetNS(t *testing.T) {
	attrs := func(netnsID int) netlink.LinkAttrs {
		attrs := netlink.NewLinkAttrs()
		attrs.NetNsID = netnsID
		return attrs
	}

	// the pairs the blind hooks scan leaves out
	pairs := []string{"veth", "netkit"}
	assert.True(t, hasPeerInOtherNetNS(&netlink.Veth{LinkAttrs: attrs(2)}, pairs...))
	assert.True(t, hasPeerInOtherNetNS(&netlink.Veth{LinkAttrs: attrs(0)}, pairs...))
	assert.False(t, hasPeerInOtherNetNS(&netlink.Veth{LinkAttrs: attrs(-1)}, pairs...), "both ends of the pair are in the host")
	assert.True(t, hasPeerInOtherNetNS(&netlink.Netkit{LinkAttrs: attrs(2)}, pairs...))
	assert.False(t, hasPeerInOtherNetNS(&netlink.Netkit{LinkAttrs: attrs(-1)}, pairs...), "both ends of the pair are in the host")
	assert.False(t, hasPeerInOtherNetNS(&netlink.Dummy{LinkAttrs: attrs(-1)}, pairs...))
	assert.False(t, hasPeerInOtherNetNS(&netlink.Macvlan{LinkAttrs: attrs(2)}, pairs...))
}

func TestBPFFilters(t *testing.T) {
	chain := uint32(3)
	filters := []netlink.Filter{
		&netlink.U32{FilterAttrs: netlink.FilterAttrs{Priority: 1}},
		&netlink.BpfFilter{FilterAttrs: netlink.FilterAttrs{Priority: 1}, Name: "cil_from_netdev-eth0", Id: 7, DirectAction: true},
		&netlink.BpfFilter{FilterAttrs: netlink.FilterAttrs{Priority: 2, Chain: &chain}, Name: "other", Id: 8},
	}

	assert.Equal(t, []tc.Filter{
		{Name: "cil_from_netdev-eth0", ProgramID: 7, Priority: 1, DirectAction: true},
		{Name: "other", ProgramID: 8, Priority: 2, Chain: 3},
	}, bpfFilters(filters))
}

// gaugeRecorder records the gauges it receives, keyed by name and tags
type gaugeRecorder struct {
	statsd.NoOpClient
	gauges map[string]float64
}

func (r *gaugeRecorder) Gauge(name string, value float64, tags []string, _ float64) error {
	r.gauges[name+"|"+strings.Join(tags, ",")] = value
	return nil
}

func TestBlindHooksReport(t *testing.T) {
	client := &gaugeRecorder{gauges: make(map[string]float64)}
	nr := &Resolver{client: client}

	hooks := []BlindHook{
		{Device: "eth0", Direction: manager.Ingress, Program: tc.ProgramAhead{Name: "cil_from_netdev", Reason: tc.BlindReasonCiliumTCX}},
		{Device: "eth1", Direction: manager.Ingress, Program: tc.ProgramAhead{Name: "cil_from_netdev", Reason: tc.BlindReasonCiliumTCX}},
		{Device: "eth0", Direction: manager.Egress, Program: tc.ProgramAhead{Name: "foreign_filter", Reason: tc.BlindReasonUnknownTCFilter}},
	}
	nr.publishBlindHooks(manager.TCAttachModeLegacy, hooks)
	assert.True(t, nr.blindHooks.warned)

	nr.sendBlindHookStats()
	assert.Equal(t, map[string]float64{
		metrics.MetricTCBlindHooks + "|direction:ingress,reason:cilium_tcx,attach_mode:legacy":       2,
		metrics.MetricTCBlindHooks + "|direction:egress,reason:unknown_tc_filter,attach_mode:legacy": 1,
	}, client.gauges)

	platform, _ := kernel.Platform()
	platformVersion, _ := kernel.PlatformVersion()
	release, _ := kernel.Release()
	arch, _ := kernel.Machine()
	telemetryValue := func(reason tc.BlindReason, direction manager.TrafficType) float64 {
		return blindHooksTelemetry.WithValues(manager.TCAttachModeLegacy.String(), string(reason), direction.String(), platform, platformVersion, release, arch).Get()
	}
	assert.Equal(t, 2.0, telemetryValue(tc.BlindReasonCiliumTCX, manager.Ingress))
	assert.Equal(t, 1.0, telemetryValue(tc.BlindReasonUnknownTCFilter, manager.Egress))
	assert.Zero(t, telemetryValue(tc.BlindReasonCiliumTCFilter, manager.Ingress))

	warning := nr.BlindHooksWarning()
	assert.Contains(t, warning, "2 host interface hooks")
	assert.Contains(t, warning, "eth0 ingress (cil_from_netdev), eth1 ingress (cil_from_netdev)")
	assert.NotContains(t, warning, "foreign_filter", "only the programs known to end the chain are worth a warning")

	// the hooks aren't blind anymore
	clear(client.gauges)
	nr.publishBlindHooks(manager.TCAttachModeLegacy, nil)
	nr.sendBlindHookStats()
	assert.Empty(t, client.gauges)
	assert.Zero(t, telemetryValue(tc.BlindReasonCiliumTCX, manager.Ingress))
	assert.Zero(t, telemetryValue(tc.BlindReasonUnknownTCFilter, manager.Egress))
	assert.Empty(t, nr.BlindHooksWarning())
}

func TestBlindHooksWarning(t *testing.T) {
	assert.Empty(t, blindHooksWarning([]BlindHook{
		{Device: "eth0", Direction: manager.Ingress, Program: tc.ProgramAhead{Name: "foreign_prog", Reason: tc.BlindReasonUnknownTCX}},
	}))

	assert.Equal(t, "network events are missed on 1 host interface hook, where Cilium programs end the tc chain before the Workload Protection classifiers: eth0 egress (cil_to_netdev)",
		blindHooksWarning([]BlindHook{
			{Device: "eth0", Direction: manager.Egress, Program: tc.ProgramAhead{Name: "cil_to_netdev", Reason: tc.BlindReasonCiliumTCX}},
		}))

	var hooks []BlindHook
	for i := range maxListedBlindHooks + 2 {
		hooks = append(hooks, BlindHook{Device: fmt.Sprintf("eth%d", i), Direction: manager.Ingress, Program: tc.ProgramAhead{Name: "cil_from_netdev", Reason: tc.BlindReasonCiliumTCX}})
	}
	warning := blindHooksWarning(hooks)
	assert.Contains(t, warning, fmt.Sprintf("%d host interface hooks", maxListedBlindHooks+2))
	assert.Contains(t, warning, fmt.Sprintf("eth%d ingress (cil_from_netdev), and 2 more", maxListedBlindHooks-1))
	assert.NotContains(t, warning, fmt.Sprintf("eth%d ", maxListedBlindHooks))
}
