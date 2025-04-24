// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0
// Original sources of this file:
//   - https://github.com/DataDog/datadog-agent/blob/dfab82/pkg/metadata/internal/gohai/payload.go
//   - https://github.com/DataDog/datadog-agent/blob/dfab82/pkg/metadata/v5/payload.go
//
// This file defines the 'Gohai' payload. gohai (https://github.com/DataDog/gohai) is a library for fetching system data.
// Its payload definition is used by the Datadog Agent and OpenTelemetry Collector Datadog exporter to export data about a given
// host, including information about its CPU, memory usage, network capabilities and platform.
//
// Most of the details on the payload definition are actually in DataDog/gohai, while here fields are just an empty interface.
// To make matters worse, this needs custom double marshaling because of legacy reasons.
//
// This payload definition is undergoing a refactor; the DataDog/gohai library is being deprecated in favor of a new module
// in DataDog/datadog-agent (see https://github.com/DataDog/gohai/pull/180).

// Package gohai implements the Gohai payload.
package gohai

import (
	"encoding/json"
)

// Gohai is the inner payload for Gohai.
// Its types must match those returned by Datadog/gohai.
// The `Collect` methods from DataDog/gohai return an interface{},
// so we need to carefully define the types of the fields here.
type Gohai struct {
	// CPU contains CPU information.
	// Type check:
	// - Collect calls getCPUInfo: https://github.com/DataDog/gohai/blob/4316413/cpu/cpu.go#L63
	//   - On macOS, getCPUInfo returns a map[string]string: https://github.com/DataDog/gohai/blob/4316413/cpu/cpu_darwin.go#L25
	//   - On Linux/arm64, getCPUInfo returns a map[string]string: https://github.com/DataDog/gohai/blob/4316413/cpu/cpu_linux_arm64.go#L26
	//   - On Linux/!arm64, getCPUInfo returns a map[string]string: https://github.com/DataDog/gohai/blob/4316413/cpu/cpu_linux_default.go#L36
	//   - On Windows, getCPUInfo returns a map[string]string: https://github.com/DataDog/gohai/blob/4316413/cpu/cpu_windows.go#L221
	//
	// All branches return a map[string]string, so we can safely define CPU as map[string]string.
	CPU map[string]string `json:"cpu"`

	// FileSystem contains filesystem information.
	// Type check:
	// - Collect calls getFileSystemInfo https://github.com/DataDog/gohai/blob/4316413/filesystem/filesystem_common.go#L23
	//   - On Linux, getFileSystemInfo calls parseDfOutput: https://github.com/DataDog/gohai/blob/4316413/filesystem/filesystem.go#L39
	//     - parseDfOutput returns a []any: https://github.com/DataDog/gohai/blob/4316413/filesystem/filesystem.go#L58
	//   - On Windows, getFileSystemInfo returns a []any: https://github.com/DataDog/gohai/blob/4316413/filesystem/filesystem_windows.go#L121
	//
	// All branches return a []any, so we can safely define FileSystem as []any.
	FileSystem []any `json:"filesystem"`

	// Memory contains memory information.
	// Type check:
	// - Collect calls getMemoryInfo: https://github.com/DataDog/gohai/blob/4316413/memory/memory.go#L28
	//   - On macOS, getMemoryInfo returns a map[string]string: https://github.com/DataDog/gohai/blob/4316413/memory/memory_darwin.go#L16
	//   - On Linux, getMemoryInfo returns a map[string]string: https://github.com/DataDog/gohai/blob/4316413/memory/memory_linux.go#L24
	//   - On Windows, getMemoryInfo returns a map[string]string: https://github.com/DataDog/gohai/blob/4316413/memory/memory_windows.go#L29
	//
	// All branches return a map[string]string, so we can safely define Memory as map[string]string.
	Memory map[string]string `json:"memory"`

	// Network contains network information.
	// Type check:
	// - Collect calls getNetworkInfo: https://github.com/DataDog/gohai/blob/4316413/network/network_common.go#L41
	//   - getNetworkInfo returns a map[string]any: https://github.com/DataDog/gohai/blob/4316413/network/network.go#L9
	//
	// All branches return a map[string]any, so we can safely define Network as map[string]any.
	Network map[string]any `json:"network"`

	// Platform contains platform information.
	// Type check:
	// - Collect calls getPlatformInfo: https://github.com/DataDog/gohai/blob/4316413/platform/platform.go#L22
	//   - getPlatformInfo returns a map[string]string: https://github.com/DataDog/gohai/blob/4316413/platform/platform.go#L52
	//
	// All branches return a map[string]string, so we can safely define Platform as map[string]string.
	Platform map[string]string `json:"platform"`
}

// Payload handles the JSON unmarshalling of the metadata payload
// As weird as it sounds, in the v5 payload the value of the "gohai" field
// is a JSON-formatted string. So this struct contains a MarshaledGohaiPayload
// which will be marshaled as a JSON-formatted string.
type Payload struct {
	Gohai gohaiMarshaler `json:"gohai"`
}

// Platform returns a reference to the Gohai payload 'platform' map.
// It initializes the field if nil.
func (p *Payload) Platform() map[string]string {
	if p.Gohai.Gohai.Platform == nil {
		p.Gohai.Gohai.Platform = map[string]string{}
	}
	return p.Gohai.Gohai.Platform
}

// CPU returns a reference to the Gohai payload 'cpu' map.
// It initializes the field if nil.
func (p *Payload) CPU() map[string]string {
	if p.Gohai.Gohai.CPU == nil {
		p.Gohai.Gohai.CPU = map[string]string{}
	}
	return p.Gohai.Gohai.CPU
}

// Network returns a reference to the Gohai payload 'network' map.
// It initializes the field if nil.
func (p *Payload) Network() map[string]any {
	if p.Gohai.Gohai.Network == nil {
		p.Gohai.Gohai.Network = map[string]any{}
	}
	return p.Gohai.Gohai.Network
}

// gohaiSerializer implements json.Marshaler and json.Unmarshaler on top of a gohai payload
type gohaiMarshaler struct {
	Gohai *Gohai
}

// MarshalJSON implements the json.Marshaler interface.
// It marshals the gohai struct twice (to a string) to comply with
// the v5 payload format
func (m gohaiMarshaler) MarshalJSON() ([]byte, error) {
	marshaledPayload, err := json.Marshal(m.Gohai)
	if err != nil {
		return []byte(""), err
	}
	doubleMarshaledPayload, err := json.Marshal(string(marshaledPayload))
	if err != nil {
		return []byte(""), err
	}
	return doubleMarshaledPayload, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
// Unmarshals the passed bytes twice (first to a string, then to gohai.Gohai)
func (m *gohaiMarshaler) UnmarshalJSON(bytes []byte) error {
	// First, unmarshal to a string
	var out string
	err := json.Unmarshal(bytes, &out)
	if err != nil {
		return err
	}

	// Then, unmarshal the JSON-formatted string into a gohai.Gohai struct.
	return json.Unmarshal([]byte(out), &(m.Gohai))
}

// NewEmpty creates a new empty Gohai payload.
func NewEmpty() Payload {
	return Payload{
		Gohai: gohaiMarshaler{
			Gohai: &Gohai{
				CPU:        map[string]string{},
				FileSystem: []any{},
				Memory:     map[string]string{},
				Network:    map[string]any{},
				Platform:   map[string]string{},
			},
		},
	}
}
