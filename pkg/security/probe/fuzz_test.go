// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package probe

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/google/gopacket/layers"
	"go.uber.org/atomic"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	pconfig "github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/eventstream"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/mount"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/netns"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/path"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	securityprofile "github.com/DataDog/datadog-agent/pkg/security/security_profile"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

// newFuzzEBPFProbe assembles a minimal EBPFProbe suitable for fuzz testing.
// All optional features (activity dumps, security profiles, SSH sessions, network,
// internal monitoring) are disabled via zero-value RuntimeSecurityConfig so that
// feature-gated code paths return early.
func newFuzzEBPFProbe(tb testing.TB) *EBPFProbe {
	tb.Helper()

	probeConfig := &pconfig.Config{DentryCacheSize: 256}
	cfg := &config.Config{
		Probe:           probeConfig,
		RuntimeSecurity: &config.RuntimeSecurityConfig{},
	}

	noopSD := &statsd.NoOpClient{}

	processResolver, err := process.NewTestEBPFResolver()
	if err != nil {
		tb.Fatalf("failed to create test process resolver: %v", err)
	}

	dentryResolver, err := dentry.NewResolver(probeConfig, noopSD, nil)
	if err != nil {
		tb.Fatalf("failed to create dentry resolver: %v", err)
	}

	nsResolver, err := netns.NewResolver(probeConfig, nil, noopSD, nil)
	if err != nil {
		tb.Fatalf("failed to create namespace resolver: %v", err)
	}

	tagsResolver := tags.NewResolver(nil, nil, nil)

	ebpfResolvers := &resolvers.EBPFResolvers{
		MountResolver:     &mount.NoOpResolver{},
		PathResolver:      &path.NoOpResolver{},
		ProcessResolver:   processResolver,
		DentryResolver:    dentryResolver,
		NamespaceResolver: nsResolver,
		CGroupResolver:    &cgroup.Resolver{},
		TagsResolver:      tagsResolver,
	}

	fieldHandlers := &EBPFFieldHandlers{
		BaseFieldHandlers: &BaseFieldHandlers{config: cfg},
		resolvers:         ebpfResolvers,
	}

	ep := &EBPFProbe{
		Resolvers:     ebpfResolvers,
		config:        cfg,
		fieldHandlers: fieldHandlers,
		event:         &model.Event{},
		eventPool: ddsync.NewTypedPool(func() *model.Event {
			return &model.Event{}
		}),
		profileManager:      securityprofile.NewTestManager(cfg),
		processKiller:       &ProcessKiller{},
		fileHasher:          &FileHasher{},
		replayEventsState:   atomic.NewBool(false),
		ctx:                 context.Background(),
		tcRequests:          make(chan tcClassifierRequest, 10),
		dnsLayer:            &layers.DNS{},
		numCPU:              1,
		BPFFilterTruncated:  atomic.NewUint64(0),
		MetricNameTruncated: atomic.NewUint64(0),
		onDemandManager:     &OnDemandProbesManager{},
		onDemandRateLimiter: rate.NewLimiter(rate.Inf, 1),
	}

	// Set up monitors with back-reference to EBPFProbe
	ep.monitors = &EBPFMonitors{
		ebpfProbe:          ep,
		eventStreamMonitor: eventstream.NewTestMonitor(),
	}

	ep.probe = &Probe{
		Config:        cfg,
		PlatformProbe: ep,
	}

	return ep
}

// FuzzHandleEvent feeds arbitrary byte slices into EBPFProbe.handleEvent
// to find panics, nil-pointer dereferences, and other memory-safety issues
// caused by malformed binary event data from the kernel.
func FuzzHandleEvent(f *testing.F) {
	// Seed corpus: a minimal 16-byte event header (type + flags + timestamp)
	f.Add(0, make([]byte, 16))

	p := newFuzzEBPFProbe(f)

	f.Fuzz(func(t *testing.T, cpu int, data []byte) {
		p.handleEvent(cpu%256, data)
	})
}
