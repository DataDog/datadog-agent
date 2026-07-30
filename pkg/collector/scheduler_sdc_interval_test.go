// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package collector

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// intervalStub wraps stub.StubCheck to return an arbitrary Interval(),
// including 0 (the long-running/one-shot check sentinel), for tests.
type intervalStub struct {
	*stub.StubCheck
	interval time.Duration
}

func (c *intervalStub) Interval() time.Duration { return c.interval }

func TestWrapWithSDCIntervalOverride_NoOverrideConfigured(t *testing.T) {
	cfg := pkgconfigsetup.Datadog()
	cfg.SetInTest("checks.sdc_compression_interval_override", 0)
	cfg.SetInTest("checks.sdc_compression_all", true)

	ch := &intervalStub{StubCheck: &stub.StubCheck{}, interval: 15 * time.Second}
	got := wrapWithSDCIntervalOverride(ch, "my_check")

	require.Same(t, check.Check(ch), got, "override <= 0 must leave the check unwrapped")
}

func TestWrapWithSDCIntervalOverride_NotCompressed(t *testing.T) {
	cfg := pkgconfigsetup.Datadog()
	cfg.SetInTest("checks.sdc_compression_interval_override", 1)
	cfg.SetInTest("checks.sdc_compression_all", false)
	cfg.SetInTest("checks.sdc_compression_checks", []string{"other_check"})

	ch := &intervalStub{StubCheck: &stub.StubCheck{}, interval: 15 * time.Second}
	got := wrapWithSDCIntervalOverride(ch, "my_check")

	require.Same(t, check.Check(ch), got, "a check not eligible for SDC compression must not have its interval overridden")
}

func TestWrapWithSDCIntervalOverride_LongRunningCheckIsNotOverridden(t *testing.T) {
	cfg := pkgconfigsetup.Datadog()
	cfg.SetInTest("checks.sdc_compression_interval_override", 1)
	cfg.SetInTest("checks.sdc_compression_all", true)

	// Interval() == 0 is a distinct scheduling mode (enqueueOnce, see
	// pkg/collector/scheduler/scheduler.go), used by long-running checks
	// (e.g. container_image, sbom) that manage their own lifecycle -
	// forcing a positive interval onto one would incorrectly convert it
	// into a normal ticked check.
	ch := &intervalStub{StubCheck: &stub.StubCheck{}, interval: 0}
	got := wrapWithSDCIntervalOverride(ch, "my_check")

	require.Same(t, check.Check(ch), got, "a long-running check (Interval()==0) must never be wrapped, even when otherwise eligible")
	require.Zero(t, got.Interval(), "the long-running check's interval must remain 0")
}

func TestWrapWithSDCIntervalOverride_OverridesEligibleCheck(t *testing.T) {
	cfg := pkgconfigsetup.Datadog()
	cfg.SetInTest("checks.sdc_compression_interval_override", 1)
	cfg.SetInTest("checks.sdc_compression_all", true)

	ch := &intervalStub{StubCheck: &stub.StubCheck{}, interval: 15 * time.Second}
	got := wrapWithSDCIntervalOverride(ch, "my_check")

	require.NotSame(t, check.Check(ch), got, "an eligible, normally-scheduled check must be wrapped")
	require.Equal(t, 1*time.Second, got.Interval())
}
