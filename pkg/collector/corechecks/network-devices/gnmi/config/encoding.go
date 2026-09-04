// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"fmt"
	"strings"

	gnmi "github.com/openconfig/gnmi/proto/gnmi"
)

// DefaultEncoding is the gNMI subscription encoding requested from devices.
// JSON_IETF is the most widely supported encoding on vendor implementations.
const DefaultEncoding = gnmi.Encoding_JSON_IETF

// ParseEncoding converts a user-facing encoding string to a gNMI encoding enum.
func ParseEncoding(raw string) (gnmi.Encoding, error) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return DefaultEncoding, nil
	}

	switch trimmed {
	case "proto", "protobuf":
		return gnmi.Encoding_PROTO, nil
	case "json":
		return gnmi.Encoding_JSON, nil
	case "json_ietf", "json-ietf", "jsonietf":
		return gnmi.Encoding_JSON_IETF, nil
	default:
		return 0, fmt.Errorf("unsupported encoding %q: use proto, json, or json_ietf", raw)
	}
}

// EncodingName returns a stable string representation of a gNMI encoding.
func EncodingName(encoding gnmi.Encoding) string {
	switch encoding {
	case gnmi.Encoding_PROTO:
		return "proto"
	case gnmi.Encoding_JSON:
		return "json"
	case gnmi.Encoding_JSON_IETF:
		return "json_ietf"
	default:
		return "unknown"
	}
}
