// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package serializerexporter

import "go.opentelemetry.io/collector/featuregate"

// useSyncForwarderGate, when enabled, swaps the asynchronous DefaultForwarder
// behind the metric serializer for a synchronous, error-propagating forwarder
// (OTelSyncForwarder). With it on, send failures surface back through
// ConsumeMetrics so OTel's exporterhelper queue/retry layer can observe and
// retry — fixing the silent-drop behavior described in OTAGENT-1024.
//
// Enabled by default (Beta) on this branch for SMP performance comparison.
// Disable via --feature-gates=-datadog.serializerexporter.UseSyncForwarder
var useSyncForwarderGate = featuregate.GlobalRegistry().MustRegister(
	"datadog.serializerexporter.UseSyncForwarder",
	featuregate.StageBeta,
	featuregate.WithRegisterDescription("Send metrics synchronously inside the serializer exporter so failures propagate to OTel exporterhelper."),
	featuregate.WithRegisterReferenceURL("https://datadoghq.atlassian.net/browse/OTAGENT-1024"),
)

// IsSyncForwarderEnabled reports whether the UseSyncForwarder feature gate is
// currently enabled. Used by the DDOT otel-agent command to decide whether to
// inject an OTelSyncForwarder (sync, error-propagating) or the default async
// DefaultForwarder into the shared serializer (OTAGENT-1024).
func IsSyncForwarderEnabled() bool {
	return useSyncForwarderGate.IsEnabled()
}
