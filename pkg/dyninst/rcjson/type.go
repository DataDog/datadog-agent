// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rcjson

import (
	"encoding/json"
	"fmt"
)

// These constants are defined in the dd-go library proto file:
// https://github.com/DataDog/dd-go/blob/87a0177d/pb/proto/remote-config/api/live-debugging/metadata.proto#L9-L18

// Type is the type of config.
// It is stored in the user metadata of the config.
// It is used to determine the type of config and how to handle it.
type Type int32

//go:generate go run golang.org/x/tools/cmd/stringer -tags linux_bpf -type=Type -linecomment

const (
	// TypeDefault is the default config type.
	TypeDefault Type = 0 // DEFAULT
	// TypeLogProbe is the log probe config type.
	TypeLogProbe Type = 1 // LOG_PROBE
	// TypeMetricProbe is the metric probe config type.
	TypeMetricProbe Type = 2 // METRIC_PROBE
	// TypeSpanProbe is the span probe config type.
	TypeSpanProbe Type = 3 // SPAN_PROBE
	// TypeSpanDecorationProbe is the span decoration probe config type.
	TypeSpanDecorationProbe Type = 4 // SPAN_DECORATION_PROBE
	// TypeTriggerProbe is the trigger probe config type.
	TypeTriggerProbe Type = 5 // TRIGGER_PROBE
	// TypeLegacyServiceConfig is the legacy service config type.
	TypeLegacyServiceConfig Type = 100 // LEGACY_SERVICE_CONFIG
	// TypeServiceConfig is the service config type.
	TypeServiceConfig Type = 101 // SERVICE_CONFIG
)

var validTypes = []Type{
	TypeLogProbe,
	TypeMetricProbe,
	TypeSpanProbe,
	TypeSpanDecorationProbe,
	TypeTriggerProbe,
	TypeLegacyServiceConfig,
	TypeServiceConfig,
}

// typesByName is the reverse of typeNames.
var typesByName = func() map[string]Type {
	m := make(map[string]Type)
	for _, v := range validTypes {
		m[v.String()] = v
	}
	return m
}()

// UnmarshalJSON unmarshals a Type from a JSON string.
func (x *Type) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if v, ok := typesByName[s]; ok {
		*x = v
	} else {
		return fmt.Errorf("invalid config type: %s", s)
	}
	return nil
}

// MarshalJSON marshals a Type to a JSON string.
func (x Type) MarshalJSON() ([]byte, error) {
	return json.Marshal(x.String())
}
