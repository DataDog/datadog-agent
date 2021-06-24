// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package registration

import (
	"encoding/json"
)

type Destination struct {
	URI      string `json:"URI"`
	Protocol string `json:"protocol"`
}
type Buffering struct {
	MaxBytes  int `json:"maxBytes"`
	MaxItems  int `json:"maxItems"`
	TimeoutMs int `json:"timeoutMs"`
}
type LogSubscriptionPayload struct {
	Buffering   Buffering   `json:"buffering"`
	Destination Destination `json:"destination"`
	Types       []string    `json:"types"`
}

func (p *LogSubscriptionPayload) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinite recursion while serializing
	type LogSubscriptionPayloadAlias LogSubscriptionPayload
	return json.Marshal((*LogSubscriptionPayloadAlias)(p))
}
