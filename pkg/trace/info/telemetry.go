// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package info

import "github.com/DataDog/datadog-agent/pkg/telemetry"

const (
	traceTelemetrySubsystem = "trace_agent"
)

var (
	receiverTelemetryLabelKeys    = []string{"lang", "lang_version", "lang_vendor", "interpreter", "tracer_version", "endpoint_version", "service"}
	receiverTelemetryPriorityKeys = append(append([]string{}, receiverTelemetryLabelKeys...), "priority")
	receiverTelemetryReasonKeys   = append(append([]string{}, receiverTelemetryLabelKeys...), "reason")

	receiverTelemetryOpts = telemetry.Options{NoDoubleUnderscoreSep: true}
)

// ReceiverTelemetry holds telemetry metrics for the trace receiver
type ReceiverTelemetry struct {
	tracesReceived        telemetry.Counter
	tracesFiltered        telemetry.Counter
	tracesBytes           telemetry.Counter
	spansReceived         telemetry.Counter
	spansDropped          telemetry.Counter
	spansFiltered         telemetry.Counter
	eventsExtracted       telemetry.Counter
	eventsSampled         telemetry.Counter
	payloadAccepted       telemetry.Counter
	payloadRefused        telemetry.Counter
	clientDroppedP0Spans  telemetry.Counter
	clientDroppedP0Traces telemetry.Counter
	tracesPriority        telemetry.Counter
	tracesDropped         telemetry.Counter
	spansMalformed        telemetry.Counter
}

// NewReceiverTelemetry creates a new ReceiverTelemetry instance
func NewReceiverTelemetry() *ReceiverTelemetry {
	return &ReceiverTelemetry{
		tracesReceived:        telemetry.NewCounterWithOpts(traceTelemetrySubsystem, "receiver_traces_received", receiverTelemetryLabelKeys, "Number of traces received by the trace-agent receiver", receiverTelemetryOpts),
		tracesFiltered:        telemetry.NewCounterWithOpts(traceTelemetrySubsystem, "receiver_traces_filtered", receiverTelemetryLabelKeys, "Number of traces filtered by the trace-agent receiver", receiverTelemetryOpts),
		tracesBytes:           telemetry.NewCounterWithOpts(traceTelemetrySubsystem, "receiver_traces_bytes", receiverTelemetryLabelKeys, "Volume of trace bytes received", receiverTelemetryOpts),
		spansReceived:         telemetry.NewCounterWithOpts(traceTelemetrySubsystem, "receiver_spans_received", receiverTelemetryLabelKeys, "Number of spans received", receiverTelemetryOpts),
		spansDropped:          telemetry.NewCounterWithOpts(traceTelemetrySubsystem, "receiver_spans_dropped", receiverTelemetryLabelKeys, "Number of spans dropped", receiverTelemetryOpts),
		spansFiltered:         telemetry.NewCounterWithOpts(traceTelemetrySubsystem, "receiver_spans_filtered", receiverTelemetryLabelKeys, "Number of spans filtered", receiverTelemetryOpts),
		eventsExtracted:       telemetry.NewCounterWithOpts(traceTelemetrySubsystem, "receiver_events_extracted", receiverTelemetryLabelKeys, "Number of events extracted", receiverTelemetryOpts),
		eventsSampled:         telemetry.NewCounterWithOpts(traceTelemetrySubsystem, "receiver_events_sampled", receiverTelemetryLabelKeys, "Number of events sampled", receiverTelemetryOpts),
		payloadAccepted:       telemetry.NewCounterWithOpts(traceTelemetrySubsystem, "receiver_payload_accepted", receiverTelemetryLabelKeys, "Number of payloads accepted", receiverTelemetryOpts),
		payloadRefused:        telemetry.NewCounterWithOpts(traceTelemetrySubsystem, "receiver_payload_refused", receiverTelemetryLabelKeys, "Number of payloads refused", receiverTelemetryOpts),
		clientDroppedP0Spans:  telemetry.NewCounterWithOpts(traceTelemetrySubsystem, "receiver_client_dropped_p0_spans", receiverTelemetryLabelKeys, "Number of client-reported priority 0 spans dropped", receiverTelemetryOpts),
		clientDroppedP0Traces: telemetry.NewCounterWithOpts(traceTelemetrySubsystem, "receiver_client_dropped_p0_traces", receiverTelemetryLabelKeys, "Number of client-reported priority 0 traces dropped", receiverTelemetryOpts),
		tracesPriority:        telemetry.NewCounterWithOpts(traceTelemetrySubsystem, "receiver_traces_priority", receiverTelemetryPriorityKeys, "Number of traces grouped by sampling priority", receiverTelemetryOpts),
		tracesDropped:         telemetry.NewCounterWithOpts(traceTelemetrySubsystem, "normalizer_traces_dropped", receiverTelemetryReasonKeys, "Number of traces dropped by the normalizer grouped by reason", receiverTelemetryOpts),
		spansMalformed:        telemetry.NewCounterWithOpts(traceTelemetrySubsystem, "normalizer_spans_malformed", receiverTelemetryReasonKeys, "Number of malformed spans grouped by reason", receiverTelemetryOpts),
	}
}

func receiverLabelValues(tags Tags) []string {
	return []string{
		tags.Lang,
		tags.LangVersion,
		tags.LangVendor,
		tags.Interpreter,
		tags.TracerVersion,
		tags.EndpointVersion,
		tags.Service,
	}
}
