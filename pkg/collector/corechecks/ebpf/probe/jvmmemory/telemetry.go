// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package jvmmemory

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

const telemetrySubsystem = "jvm_memory"

// Reasons of the libjvm_errors telemetry counter.
const (
	reasonOpen     = "open"
	reasonParse    = "parse"
	reasonArch     = "arch"
	reasonAttach   = "attach"
	reasonTooMany  = "too_many_files"
	reasonTooSites = "too_many_sites"
)

// Telemetry of the probe, registered once per process so that the module can
// be restarted.
var probeTelemetry = struct {
	filesAttached      telemetry.SimpleCounter
	filesWithoutProbes telemetry.SimpleCounter
	fileErrors         telemetry.Counter
	siteAttachErrors   telemetry.Counter
	attachedFiles      telemetry.SimpleGauge
	specs              telemetry.SimpleGauge
	jvms               telemetry.SimpleGauge
}{
	filesAttached: telemetryimpl.GetCompatComponent().NewSimpleCounter(telemetrySubsystem, "libjvm_attached",
		"Number of libjvm.so files whose HotSpot probes were attached."),
	filesWithoutProbes: telemetryimpl.GetCompatComponent().NewSimpleCounter(telemetrySubsystem, "libjvm_without_probes",
		"Number of libjvm.so files skipped because they have no HotSpot USDT probes (e.g. musl, Amazon Corretto or Oracle JDK builds)."),
	fileErrors: telemetryimpl.GetCompatComponent().NewCounter(telemetrySubsystem, "libjvm_errors", []string{"reason"},
		"Number of failed attempts to attach the HotSpot probes of a libjvm.so file, by reason. A file may be attempted again, e.g. for another JVM or after a transient error."),
	siteAttachErrors: telemetryimpl.GetCompatComponent().NewCounter(telemetrySubsystem, "probe_attach_errors", []string{"probe"},
		"Number of HotSpot probe sites that could not be attached, by probe."),
	attachedFiles: telemetryimpl.GetCompatComponent().NewSimpleGauge(telemetrySubsystem, "libjvm_files",
		"Number of libjvm.so files whose HotSpot probes are attached."),
	specs: telemetryimpl.GetCompatComponent().NewSimpleGauge(telemetrySubsystem, "usdt_specs",
		"Number of distinct USDT argument specs in use."),
	jvms: telemetryimpl.GetCompatComponent().NewSimpleGauge(telemetrySubsystem, "jvms",
		"Number of monitored JVM processes."),
}
