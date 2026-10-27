// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package logsagentexporter

import "go.opentelemetry.io/collector/featuregate"

// SyncSenderGateID is the ID of the feature gate that enables the sync sender.
const SyncSenderGateID = "datadog.logsagentexporter.UseSyncSender"

// useSyncSenderGate, when enabled, makes exporters that have a SyncSender send logs in ConsumeLogs
// instead of handing them to the asynchronous logs agent pipeline. Delivery failures then reach
// exporterhelper, which counts, retries or drops them (OTAGENT-1075).
//
// Disabled by default (Alpha). Enable via
//
//	--feature-gates=+datadog.logsagentexporter.UseSyncSender
var useSyncSenderGate = featuregate.GlobalRegistry().MustRegister(
	SyncSenderGateID,
	featuregate.StageAlpha,
	featuregate.WithRegisterDescription("Send logs synchronously in the logs agent exporter so that delivery failures propagate to OTel exporterhelper."),
	featuregate.WithRegisterReferenceURL("https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/47386"),
)

// IsSyncSenderEnabled reports whether the UseSyncSender feature gate is enabled.
func IsSyncSenderEnabled() bool {
	return useSyncSenderGate.IsEnabled()
}
