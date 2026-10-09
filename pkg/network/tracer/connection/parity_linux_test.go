// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package connection

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/parity"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/parity/workload"
)

// parityTracer is a connection tracer plus the closed connections it delivered
type parityTracer struct {
	tr     Tracer
	mu     sync.Mutex
	closed []network.ConnectionStats
}

// newParityConfig returns a CO-RE config selecting the fentry or the kprobe
// tracer. Each tracer gets its own config, since newEbpfTracer mutates the one
// it is given.
func newParityConfig(fentry bool) *config.Config {
	cfg := config.New()
	cfg.EnableEbpfless = false
	cfg.EnableSKTracer = false
	cfg.EnableFentry = fentry
	cfg.EnableCORE = true
	cfg.EnableCORETracer = true
	cfg.EnableRuntimeCompiler = false
	cfg.AllowRuntimeCompiledFallback = false
	cfg.AllowPrebuiltFallback = false
	// keep the TLS-cert uprobe attacher (shared process monitor) out of the
	// first coexistence run
	cfg.EnableCertCollection = false
	cfg.AttachKprobesWithKprobeEventsABI = false
	return cfg
}

func startParityTracer(t *testing.T, fentry bool, want TracerType) *parityTracer {
	tr, err := NewTracer(newParityConfig(fentry), nil)
	require.NoError(t, err)
	t.Cleanup(tr.Stop)
	// a misconfigured arm would silently load a different tracer
	require.Equal(t, want, tr.Type(), "unexpected tracer type")

	p := &parityTracer{tr: tr}
	require.NoError(t, tr.Start(func(c *network.ConnectionStats) {
		if c == nil {
			return
		}
		// the struct is pooled; IsClosed is normally set by the outer tracer
		cs := *c
		cs.IsClosed = true
		p.mu.Lock()
		p.closed = append(p.closed, cs)
		p.mu.Unlock()
	}))
	return p
}

// snapshot returns every connection the tracer has reported that passes filter:
// closed connections delivered so far plus currently active ones
func (p *parityTracer) snapshot(t *testing.T, filter func(*network.ConnectionStats) bool) []network.ConnectionStats {
	p.tr.FlushPending()
	buf := network.NewConnectionBuffer(512, 512)
	require.NoError(t, p.tr.GetConnections(buf, filter))

	p.mu.Lock()
	defer p.mu.Unlock()
	conns := slices.DeleteFunc(slices.Clone(p.closed), func(c network.ConnectionStats) bool { return !filter(&c) })
	return append(conns, buf.Connections()...)
}

// hasAll reports whether every expected connection side appears in conns
func hasAll(conns []network.ConnectionStats, rec *workload.Recorder) bool {
	oracle := rec.Oracle()
	seen := 0
	for i := range conns {
		if _, ok := oracle(&conns[i]); ok {
			seen++
		}
	}
	return seen >= len(rec.Expectations())
}

// TestKprobeFentryParity loads the kprobe and fentry connection tracers side by
// side, runs the same workload under both, and compares what they report.
func TestKprobeFentryParity(t *testing.T) {
	if !slices.Contains(ebpftest.SupportedBuildModes(), ebpftest.Fentry) {
		t.Skip("fentry not supported on this kernel (set TEST_FENTRY_OVERRIDE=true to force)")
	}

	kprobe := startParityTracer(t, false, TracerTypeKProbeCORE)
	fentry := startParityTracer(t, true, TracerTypeFentry)

	rec := workload.Run(t, workload.Default())
	require.False(t, t.Failed(), "workload failed")
	filter := rec.Filter()

	// closed connections arrive asynchronously; wait until both tracers have
	// reported every connection side the workload knows about
	var a, b []network.ConnectionStats
	deadline := time.Now().Add(10 * time.Second)
	for {
		a, b = kprobe.snapshot(t, filter), fentry.snapshot(t, filter)
		if (hasAll(a, rec) && hasAll(b, rec)) || time.Now().After(deadline) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// No PacketSlack: packet counts of closed connections must match exactly.
	// That relies on the workload closing sockets with nothing in flight. The
	// kprobe handler and the fentry trampoline read the socket's segment
	// counters one after the other at tcp_close, so a close that races in-flight
	// segments makes fentry report a few more packets (seen in 3/8 runs before
	// the workload ordered its closes, never when pinned to one CPU).
	report := parity.Compare(a, b, parity.Options{
		LabelA:           "kprobe",
		LabelB:           "fentry",
		CounterTolerance: 0.01,
		RTTTolerance:     0.5,
		Filter:           filter,
		Oracle:           rec.Oracle(),
	})
	t.Log(report.String())
	require.True(t, report.OK(), "kprobe and fentry tracers diverged:\n%s", report)
}
