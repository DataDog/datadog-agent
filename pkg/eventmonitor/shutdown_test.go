// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package eventmonitor

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/logging"
)

type shutdownPlatform struct {
	probe.PlatformProbe
	calls        *[]string
	statsStopped *bool
	t            *testing.T
}

func (p *shutdownPlatform) Stop() { *p.calls = append(*p.calls, "probe.Stop") }
func (p *shutdownPlatform) Close() error {
	require.True(p.t, *p.statsStopped, "stats must stop before probe resources are released")
	*p.calls = append(*p.calls, "probe.Close")
	return nil
}

type tracedShutdownPlatform struct {
	*shutdownPlatform
	enabled bool
}

func (p *tracedShutdownPlatform) StopWithTrace(phase *logging.StartupPhase) {
	require.Equal(p.t, p.enabled, phase != nil)
	p.Stop()
}

func (p *tracedShutdownPlatform) CloseWithTrace(phase *logging.StartupPhase) error {
	require.Equal(p.t, p.enabled, phase != nil)
	return p.Close()
}

type shutdownConsumer struct {
	calls *[]string
	id    string
}

func (c *shutdownConsumer) ID() string { return c.id }
func (*shutdownConsumer) Start() error { return nil }
func (c *shutdownConsumer) Stop()      { *c.calls = append(*c.calls, c.id+".Stop") }

type tracedShutdownConsumer struct {
	*shutdownConsumer
	t       *testing.T
	enabled bool
}

func (c *tracedShutdownConsumer) StopWithTrace(phase *logging.StartupPhase) {
	require.Equal(c.t, c.enabled, phase != nil)
	c.Stop()
}

func TestShutdownPreservesProducerConsumerAndCleanupOrder(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, tracedPlatform := range []bool{false, true} {
			t.Run(fmt.Sprintf("tracing=%t/platform=%t", enabled, tracedPlatform), func(t *testing.T) {
				var calls []string
				statsStopped := false
				platform := &shutdownPlatform{calls: &calls, statsStopped: &statsStopped, t: t}
				var pp probe.PlatformProbe = platform
				if tracedPlatform {
					pp = &tracedShutdownPlatform{shutdownPlatform: platform, enabled: enabled}
				}
				ctx, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)
				m := &EventMonitor{
					Probe: &probe.Probe{PlatformProbe: pp},
					cancelFnc: func() {
						calls = append(calls, "stats.cancel")
						cancel()
					},
					eventConsumers: []EventConsumer{
						&shutdownConsumer{calls: &calls, id: "plain"},
						&tracedShutdownConsumer{shutdownConsumer: &shutdownConsumer{calls: &calls, id: "traced"}, t: t, enabled: enabled},
					},
				}
				m.wg.Add(1)
				go func() {
					defer m.wg.Done()
					<-ctx.Done()
					statsStopped = true
				}()
				if enabled {
					// Allocate an in-memory trace without exporting it or contacting an intake.
					logger := &logging.FxTracingLogger{}
					logger.EnableSpansSending("8126")
					trace := logging.StartStartupTrace(logger, "stopSystemProbe", "test")
					m.CloseWithTrace(trace.StartPhase("module.event_monitor.close"))
				} else {
					m.Close()
				}
				require.Equal(t, []string{"probe.Stop", "plain.Stop", "traced.Stop", "stats.cancel", "probe.Close"}, calls)
			})
		}
	}
}
