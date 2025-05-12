// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package config

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

const (
	// ConfigTypeDefault is the default config type.
	ConfigTypeDefault Type = 0
	// ConfigTypeLogProbe is the log probe config type.
	ConfigTypeLogProbe Type = 1
	// ConfigTypeMetricProbe is the metric probe config type.
	ConfigTypeMetricProbe Type = 2
	// ConfigTypeSpanProbe is the span probe config type.
	ConfigTypeSpanProbe Type = 3
	// ConfigTypeSpanDecorationProbe is the span decoration probe config type.
	ConfigTypeSpanDecorationProbe Type = 4
	// ConfigTypeTriggerProbe is the trigger probe config type.
	ConfigTypeTriggerProbe Type = 5
	// ConfigTypeLegacyServiceConfig is the legacy service config type.
	ConfigTypeLegacyServiceConfig Type = 100
	// ConfigTypeServiceConfig is the service config type.
	ConfigTypeServiceConfig Type = 101
)

// Enum value maps for Type.
var typeNames = map[int32]string{
	0:   "DEFAULT",
	1:   "LOG_PROBE",
	2:   "METRIC_PROBE",
	3:   "SPAN_PROBE",
	4:   "SPAN_DECORATION_PROBE",
	5:   "TRIGGER_PROBE",
	100: "LEGACY_SERVICE_CONFIG",
	101: "SERVICE_CONFIG",
}

// typesByName is the reverse of typeNames.
var typesByName = func() map[string]int32 {
	m := make(map[string]int32)
	for k, v := range typeNames {
		m[v] = k
	}
	return m
}()

func (x Type) String() string {
	if s, ok := typeNames[int32(x)]; ok {
		return s
	}
	return ConfigTypeDefault.String()
}

// UnmarshalJSON unmarshals a Type from a JSON string.
func (x *Type) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if v, ok := typesByName[s]; ok {
		*x = Type(v)
	} else {
		return fmt.Errorf("invalid config type: %s", s)
	}
	return nil
}

// MarshalJSON marshals a Type to a JSON string.
func (x Type) MarshalJSON() ([]byte, error) {
	return json.Marshal(x.String())
}
