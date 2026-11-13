// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// platformTags are the tags of the eBPF agent telemetry profile, which the network classifiers metrics share
var platformTags = []string{"platform", "platform_version", "kernel", "arch"}

// agent telemetry metrics, shipped by the runtime-security profile
var (
	blindHooksTelemetry = metrics.NewITGauge(metrics.MetricTCBlindHooksTelemetry,
		append([]string{"attach_mode", "reason", "direction"}, platformTags...),
		"Number of hooks of host interfaces where a foreign program runs before the TC classifiers")
	staleLinksTelemetry = metrics.NewITCounter(metrics.MetricTCStaleLinksTelemetry,
		platformTags,
		"Number of TCX links detached because their device left the network namespace in which they were attached")
)

// withPlatformTags appends the values of the platform tags to the provided tag values
func withPlatformTags(values ...string) []string {
	platform, _ := kernel.Platform()
	platformVersion, _ := kernel.PlatformVersion()
	release, _ := kernel.Release()
	arch, _ := kernel.Machine()
	return append(values, platform, platformVersion, release, arch)
}

func countStaleLinks(count int) {
	if count > 0 {
		staleLinksTelemetry.Add(float64(count), withPlatformTags()...)
	}
}
