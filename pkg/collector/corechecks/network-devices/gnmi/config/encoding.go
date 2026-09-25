// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"fmt"
	"strings"
)

// Encoding identifies the value encoding requested from a gNMI target.
type Encoding int32

// Values match the Encoding enum in the gNMI wire protocol.
const (
	encodingJSON     Encoding = 0
	encodingProto    Encoding = 2
	encodingJSONIETF Encoding = 4

	// DefaultEncoding is the gNMI subscription encoding requested from devices.
	// JSON_IETF is the most widely supported encoding on vendor implementations.
	DefaultEncoding = encodingJSONIETF
)

// ParseEncoding converts a user-facing encoding string to a gNMI encoding enum.
func ParseEncoding(raw string) (Encoding, error) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return DefaultEncoding, nil
	}

	switch trimmed {
	case "proto", "protobuf":
		return encodingProto, nil
	case "json":
		return encodingJSON, nil
	case "json_ietf", "json-ietf", "jsonietf":
		return encodingJSONIETF, nil
	default:
		return 0, fmt.Errorf("unsupported encoding %q: use proto, json, or json_ietf", raw)
	}
}

// EncodingName returns a stable string representation of a gNMI encoding.
func EncodingName(encoding Encoding) string {
	switch encoding {
	case encodingProto:
		return "proto"
	case encodingJSON:
		return "json"
	case encodingJSONIETF:
		return "json_ietf"
	default:
		return "unknown"
	}
}
