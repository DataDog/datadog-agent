// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package registration

import (
	"encoding/json"
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

// LogSubscriptionPayload is the payload we send while subscribing to the AWS Logs API
type LogSubscriptionPayload struct {
	Buffering   buffering   `json:"buffering"`
	Destination destination `json:"destination"`
	Types       []string    `json:"types"`
}

// MarshalJSON marshals the given LogSubscriptionPayload object
func (p *LogSubscriptionPayload) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinite recursion while serializing
	type LogSubscriptionPayloadAlias LogSubscriptionPayload
	return json.Marshal((*LogSubscriptionPayloadAlias)(p))
}
