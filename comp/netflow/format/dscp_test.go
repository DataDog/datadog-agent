// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDSCPFromTOS(t *testing.T) {
	tests := []struct {
		name     string
		tos      uint32
		expected uint32
	}{
		{name: "zero", tos: 0, expected: 0},
		{name: "EF with ECN bits set", tos: 0b1011_1000, expected: 46}, // 184 / 0xb8
		{name: "ECN bits are masked off", tos: 0b1011_1011, expected: 46},
		{name: "max", tos: 0b1111_1111, expected: 63},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, DSCPFromTOS(tt.tos))
		})
	}
}

func TestDSCPNameFromTOS(t *testing.T) {
	// Inputs are raw ToS bytes (dscp << 2); the 2 low ECN bits must not affect the result.
	tests := []struct {
		name         string
		tos          uint32
		expectedName string
	}{
		{name: "CS0", tos: 0 << 2, expectedName: "CS0"},
		{name: "LE", tos: 1 << 2, expectedName: "LE"},
		{name: "CS1", tos: 8 << 2, expectedName: "CS1"},
		{name: "AF11", tos: 10 << 2, expectedName: "AF11"},
		{name: "AF12", tos: 12 << 2, expectedName: "AF12"},
		{name: "AF13", tos: 14 << 2, expectedName: "AF13"},
		{name: "CS2", tos: 16 << 2, expectedName: "CS2"},
		{name: "AF21", tos: 18 << 2, expectedName: "AF21"},
		{name: "AF22", tos: 20 << 2, expectedName: "AF22"},
		{name: "AF23", tos: 22 << 2, expectedName: "AF23"},
		{name: "CS3", tos: 24 << 2, expectedName: "CS3"},
		{name: "AF31", tos: 26 << 2, expectedName: "AF31"},
		{name: "AF32", tos: 28 << 2, expectedName: "AF32"},
		{name: "AF33", tos: 30 << 2, expectedName: "AF33"},
		{name: "CS4", tos: 32 << 2, expectedName: "CS4"},
		{name: "AF41", tos: 34 << 2, expectedName: "AF41"},
		{name: "AF42", tos: 36 << 2, expectedName: "AF42"},
		{name: "AF43", tos: 38 << 2, expectedName: "AF43"},
		{name: "CS5", tos: 40 << 2, expectedName: "CS5"},
		{name: "VOICE-ADMIT", tos: 44 << 2, expectedName: "VOICE-ADMIT"},
		{name: "NQB", tos: 45 << 2, expectedName: "NQB"},
		{name: "EF", tos: 46 << 2, expectedName: "EF"},
		{name: "EF with ECN bits set", tos: 0b1011_1011, expectedName: "EF"},
		{name: "CS6", tos: 48 << 2, expectedName: "CS6"},
		{name: "CS7", tos: 56 << 2, expectedName: "CS7"},
		{name: "unknown value falls back", tos: 23 << 2, expectedName: "DSCP-23"},
		{name: "unknown max value falls back", tos: 63 << 2, expectedName: "DSCP-63"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedName, DSCPNameFromTOS(tt.tos))
		})
	}
}
