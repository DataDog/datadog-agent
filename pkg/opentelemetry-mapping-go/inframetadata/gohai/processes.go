// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0
// Original sources of this file:
// - https://github.com/DataDog/datadog-agent/blob/dfab82/pkg/metadata/internal/resources/payload.go
//
// This file defines the Gohai metadata payload for resources. This includes information about processes running in the system.

package gohai

// ProcessesPayload handles the JSON unmarshalling
type ProcessesPayload struct {
	Processes map[string]interface{} `json:"processes"`
	Meta      map[string]string      `json:"meta"`
}
