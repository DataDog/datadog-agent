// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package registration

import (
	json "github.com/json-iterator/go"
)

type destination struct {
	URI      string `json:"URI"`
	Protocol string `json:"protocol"`
}
type buffering struct {
	MaxBytes  int `json:"maxBytes"`
	MaxItems  int `json:"maxItems"`
	TimeoutMs int `json:"timeoutMs"`
}

// TelemetrySubscriptionPayload is the payload we send while subscribing to the
// AWS Telemetry API
type TelemetrySubscriptionPayload struct {
	Buffering     buffering   `json:"buffering"`
	Destination   destination `json:"destination"`
	Types         []string    `json:"types"`
	SchemaVersion string      `json:"schemaVersion"`
}

// MarshalJSON marshals the given TelemetrySubscriptionPayload object
func (p *TelemetrySubscriptionPayload) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinite recursion while serializing
	type TelemetrySubscriptionPayloadAlias TelemetrySubscriptionPayload
	return json.Marshal((*TelemetrySubscriptionPayloadAlias)(p))
}
