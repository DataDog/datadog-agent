// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package uprobes

import (
	"sync"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	telemetryNoop "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
)

const uprobeAttacherTelemetrySubsystem = "uprobe_attacher"

var (
	telemetryDefs telemetryDefinitions
)

// telemetryDefinitions defines the telemetry counters and histograms for the uprobe attacher. These will
// be used to create the simple counters with pre-defined labels
type telemetryDefinitions struct {
	once                    sync.Once
	eventsHandled           telemetryComponent.Counter
	exitsHandled            telemetryComponent.Counter
	attachments             telemetryComponent.Counter
	binaryInspectionNs      telemetryComponent.Histogram
	probeAttachErrors       telemetryComponent.Counter
	createdProbes           telemetryComponent.Counter
	attachedProbes          telemetryComponent.Counter
	binaryInspectionBuckets []float64
}

func (td *telemetryDefinitions) init(tm telemetryComponent.Component) {
	td.once.Do(func() {
		// Duration in nanoseconds. Buckets
		td.binaryInspectionBuckets = []float64{
			10e3,  // 10μs
			100e3, // 100μs
			1e6,   // 1ms
			10e6,  // 10ms
			100e6, // 100ms
			1e9,   // 1s
		}

		td.eventsHandled = tm.NewCounter(
			uprobeAttacherTelemetrySubsystem,
			"events__handled",
			[]string{"attacher", "type"},
			"Number of uprobe attacher events handled, by event type (library/process).",
		)
		td.exitsHandled = tm.NewCounter(
			uprobeAttacherTelemetrySubsystem,
			"exits__handled",
			[]string{"attacher"},
			"Number of process exits handled by the uprobe attacher.",
		)
		td.attachments = tm.NewCounter(
			uprobeAttacherTelemetrySubsystem,
			"attachments",
			[]string{"attacher", "type", "result"},
			"Number of uprobe attachment attempts, by type (process/library) and result (success/failure).",
		)
		td.binaryInspectionNs = tm.NewHistogram(
			uprobeAttacherTelemetrySubsystem,
			"binary_inspection_duration_nanoseconds",
			[]string{"attacher"},
			"Distribution of uprobe binary inspection durations (in nanoseconds).",
			td.binaryInspectionBuckets,
		)
		td.probeAttachErrors = tm.NewCounter(
			uprobeAttacherTelemetrySubsystem,
			"probe_attachments__errors",
			[]string{"attacher", "stage"},
			"Number of probe attachment/validation errors.",
		)
		td.createdProbes = tm.NewCounter(
			uprobeAttacherTelemetrySubsystem,
			"probes__created",
			[]string{"attacher"},
			"Number of probes created by the uprobe attacher.",
		)
		td.attachedProbes = tm.NewCounter(
			uprobeAttacherTelemetrySubsystem,
			"probes__attached",
			[]string{"attacher"},
			"Number of probes attached by the uprobe attacher (includes re-attaching previously created probes).",
		)
	})
}

// uprobeAttacherTelemetry is per-attacher telemetry with the attacher name already bound in labels.
// Counters/histograms are stored as Simple* (no additional labels needed at callsites).
type uprobeAttacherTelemetry struct {
	eventsHandledProcess telemetryComponent.SimpleCounter
	eventsHandledLibrary telemetryComponent.SimpleCounter
	exitsHandled         telemetryComponent.SimpleCounter

	libraryAttachmentsSuccess telemetryComponent.SimpleCounter
	processAttachmentsSuccess telemetryComponent.SimpleCounter
	libraryAttachmentsFailure telemetryComponent.SimpleCounter
	processAttachmentsFailure telemetryComponent.SimpleCounter

	binaryInspectionNs telemetryComponent.SimpleHistogram

	probeAttachErrorsAttachExisting telemetryComponent.SimpleCounter
	probeAttachErrorsAddHook        telemetryComponent.SimpleCounter
	probeAttachErrorsValidate       telemetryComponent.SimpleCounter

	createdProbes  telemetryComponent.SimpleCounter
	attachedProbes telemetryComponent.SimpleCounter
}

// newUprobeAttacherTelemetry creates a new uprobeAttacherTelemetry instance. if tm is nil, will return a valid instance of the telemetry counters
// with no-op implementations so that they can be used without checking for nil constantly.
func newUprobeAttacherTelemetry(tm telemetryComponent.Component, attacherName string) *uprobeAttacherTelemetry {
	var definitions *telemetryDefinitions
	if tm == nil {
		// Create a no-op, one-use telemetryDefinitions instance for the case where telemetry is not enabled
		definitions = &telemetryDefinitions{
			once: sync.Once{},
		}
		definitions.init(telemetryNoop.GetCompatComponent())
	} else {
		definitions = &telemetryDefs
		definitions.init(tm)
	}

	return &uprobeAttacherTelemetry{
		eventsHandledProcess: definitions.eventsHandled.WithValues(attacherName, "process"),
		eventsHandledLibrary: definitions.eventsHandled.WithValues(attacherName, "library"),
		exitsHandled:         definitions.exitsHandled.WithValues(attacherName),

		libraryAttachmentsSuccess: definitions.attachments.WithValues(attacherName, "library", "success"),
		processAttachmentsSuccess: definitions.attachments.WithValues(attacherName, "process", "success"),
		libraryAttachmentsFailure: definitions.attachments.WithValues(attacherName, "library", "failure"),
		processAttachmentsFailure: definitions.attachments.WithValues(attacherName, "process", "failure"),

		binaryInspectionNs: definitions.binaryInspectionNs.WithValues(attacherName),

		probeAttachErrorsAttachExisting: definitions.probeAttachErrors.WithValues(attacherName, "attach_existing"),
		probeAttachErrorsAddHook:        definitions.probeAttachErrors.WithValues(attacherName, "add_hook"),
		probeAttachErrorsValidate:       definitions.probeAttachErrors.WithValues(attacherName, "validate"),

		createdProbes:  definitions.createdProbes.WithValues(attacherName),
		attachedProbes: definitions.attachedProbes.WithValues(attacherName),
	}
}
