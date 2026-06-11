// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package probe

import (
	"context"
	"encoding/binary"
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
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usergroup"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	securityprofile "github.com/DataDog/datadog-agent/pkg/security/security_profile"
	"github.com/DataDog/datadog-agent/pkg/util/ktime"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

// makeEventSeed creates a seed event buffer with proper structure:
// - Event header (16 bytes): timestamp(8) + type(4) + flags(4)
// - PIDContext (40 bytes)
// - SpanContext (24 bytes)
// - CGroupContext (16 bytes)
// Total minimum: 96 bytes
func makeEventSeed(eventType model.EventType, extraData []byte) []byte {
	// Minimum size: header(16) + PIDContext(40) + SpanContext(24) + CGroupContext(16) = 96
	buf := make([]byte, 96+len(extraData))

	// Event header
	binary.NativeEndian.PutUint64(buf[0:8], 1000000000)         // timestamp
	binary.NativeEndian.PutUint32(buf[8:12], uint32(eventType)) // type
	binary.NativeEndian.PutUint32(buf[12:16], 0)                // flags

	// PIDContext (40 bytes starting at offset 16)
	binary.NativeEndian.PutUint32(buf[16:20], 1234) // pid
	binary.NativeEndian.PutUint32(buf[20:24], 1234) // tid
	binary.NativeEndian.PutUint32(buf[24:28], 1)    // netns
	binary.NativeEndian.PutUint32(buf[28:32], 1)    // mntns
	// rest is padding/zero

	// SpanContext (24 bytes starting at offset 56)
	// All zeros is fine

	// CGroupContext (16 bytes starting at offset 80)
	// All zeros is fine

	// Extra data for specific event types
	copy(buf[96:], extraData)

	return buf
}

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

	// Create shared resolvers that will be used by multiple components
	timeResolver, err := ktime.NewResolver()
	if err != nil {
		tb.Fatalf("failed to create time resolver: %v", err)
	}

	pathResolver := &path.NoOpResolver{}
	mountResolver := &mount.NoOpResolver{}

	userGroupResolver, err := usergroup.NewResolver(nil)
	if err != nil {
		tb.Fatalf("failed to create usergroup resolver: %v", err)
	}

	// Create process resolver with shared dependencies
	processResolver, err := process.NewTestEBPFResolver(timeResolver, pathResolver, mountResolver, userGroupResolver)
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

	cgroupResolver, err := cgroup.NewResolver(noopSD, nil, dentryResolver)
	if err != nil {
		tb.Fatalf("failed to create cgroup resolver: %v", err)
	}

	ebpfResolvers := &resolvers.EBPFResolvers{
		MountResolver:     mountResolver,
		PathResolver:      pathResolver,
		ProcessResolver:   processResolver,
		DentryResolver:    dentryResolver,
		NamespaceResolver: nsResolver,
		CGroupResolver:    cgroupResolver,
		TagsResolver:      tagsResolver,
		TimeResolver:      timeResolver,
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
	// Seed corpus with properly structured events for various event types
	// This helps the fuzzer start with valid structures and mutate from there

	// Minimal header only (will fail early but tests header parsing)
	f.Add(0, make([]byte, 16))

	// Full context structure for common event types
	f.Add(0, makeEventSeed(model.FileOpenEventType, make([]byte, 64)))    // open
	f.Add(0, makeEventSeed(model.FileMkdirEventType, make([]byte, 64)))   // mkdir
	f.Add(0, makeEventSeed(model.FileUnlinkEventType, make([]byte, 64)))  // unlink
	f.Add(0, makeEventSeed(model.FileRenameEventType, make([]byte, 128))) // rename
	f.Add(0, makeEventSeed(model.FileChmodEventType, make([]byte, 64)))   // chmod
	f.Add(0, makeEventSeed(model.FileChownEventType, make([]byte, 64)))   // chown
	f.Add(0, makeEventSeed(model.ForkEventType, make([]byte, 256)))       // fork
	f.Add(0, makeEventSeed(model.ExecEventType, make([]byte, 256)))       // exec
	f.Add(0, makeEventSeed(model.ExitEventType, make([]byte, 64)))        // exit
	f.Add(0, makeEventSeed(model.FileMountEventType, make([]byte, 128)))  // mount
	f.Add(0, makeEventSeed(model.FileUmountEventType, make([]byte, 64)))  // umount
	f.Add(0, makeEventSeed(model.SetuidEventType, make([]byte, 64)))      // setuid
	f.Add(0, makeEventSeed(model.SetgidEventType, make([]byte, 64)))      // setgid
	f.Add(0, makeEventSeed(model.CapsetEventType, make([]byte, 64)))      // capset
	f.Add(0, makeEventSeed(model.MMapEventType, make([]byte, 64)))        // mmap
	f.Add(0, makeEventSeed(model.MProtectEventType, make([]byte, 64)))    // mprotect
	f.Add(0, makeEventSeed(model.LoadModuleEventType, make([]byte, 128))) // load module
	f.Add(0, makeEventSeed(model.SignalEventType, make([]byte, 64)))      // signal
	f.Add(0, makeEventSeed(model.SpliceEventType, make([]byte, 64)))      // splice
	f.Add(0, makeEventSeed(model.DNSEventType, make([]byte, 128)))        // dns
	f.Add(0, makeEventSeed(model.ConnectEventType, make([]byte, 128)))    // connect
	f.Add(0, makeEventSeed(model.BindEventType, make([]byte, 128)))       // bind

	// Invalid/boundary event types
	f.Add(0, makeEventSeed(model.UnknownEventType, make([]byte, 64)))     // unknown (0)
	f.Add(0, makeEventSeed(model.MaxKernelEventType-1, make([]byte, 64))) // last valid
	f.Add(0, makeEventSeed(model.MaxKernelEventType, make([]byte, 64)))   // boundary
	f.Add(0, makeEventSeed(model.MaxKernelEventType+1, make([]byte, 64))) // invalid

	p := newFuzzEBPFProbe(f)

	f.Fuzz(func(_ *testing.T, cpu int, data []byte) {
		p.handleEvent(cpu%256, data)
	})
}
