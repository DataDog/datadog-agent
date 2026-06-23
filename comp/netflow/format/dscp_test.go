// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDSCP(t *testing.T) {
	tests := []struct {
		name         string
		dscp         uint32
		expectedName string
	}{
		{name: "CS0", dscp: 0, expectedName: "CS0"},
		{name: "LE", dscp: 1, expectedName: "LE"},
		{name: "CS1", dscp: 8, expectedName: "CS1"},
		{name: "AF11", dscp: 10, expectedName: "AF11"},
		{name: "AF12", dscp: 12, expectedName: "AF12"},
		{name: "AF13", dscp: 14, expectedName: "AF13"},
		{name: "CS2", dscp: 16, expectedName: "CS2"},
		{name: "AF21", dscp: 18, expectedName: "AF21"},
		{name: "AF22", dscp: 20, expectedName: "AF22"},
		{name: "AF23", dscp: 22, expectedName: "AF23"},
		{name: "CS3", dscp: 24, expectedName: "CS3"},
		{name: "AF31", dscp: 26, expectedName: "AF31"},
		{name: "AF32", dscp: 28, expectedName: "AF32"},
		{name: "AF33", dscp: 30, expectedName: "AF33"},
		{name: "CS4", dscp: 32, expectedName: "CS4"},
		{name: "AF41", dscp: 34, expectedName: "AF41"},
		{name: "AF42", dscp: 36, expectedName: "AF42"},
		{name: "AF43", dscp: 38, expectedName: "AF43"},
		{name: "CS5", dscp: 40, expectedName: "CS5"},
		{name: "VOICE-ADMIT", dscp: 44, expectedName: "VOICE-ADMIT"},
		{name: "NQB", dscp: 45, expectedName: "NQB"},
		{name: "EF", dscp: 46, expectedName: "EF"},
		{name: "CS6", dscp: 48, expectedName: "CS6"},
		{name: "CS7", dscp: 56, expectedName: "CS7"},
		{name: "unknown value falls back", dscp: 23, expectedName: "DSCP-23"},
		{name: "unknown max value falls back", dscp: 63, expectedName: "DSCP-63"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedName, DSCP(tt.dscp))
		})
	}
}
