// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package validate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidHostname(t *testing.T) {
	tests := []struct {
		hostname      string
		shouldBeValid bool
	}{
		{
			hostname:      "datadoghq.com",
			shouldBeValid: true,
		},
		{
			hostname:      "",
			shouldBeValid: false,
		},
		{
			hostname:      "localhost",
			shouldBeValid: false,
		},
		{
			hostname:      "data🐕hq.com",
			shouldBeValid: false,
		},
		{
			hostname:      strings.Repeat("a", 256),
			shouldBeValid: false,
		},
		{
			hostname:      "LOCALHOST", // Localhost identifiers are not valid
			shouldBeValid: false,
		},
		{
			hostname:      "localhost.localdomain", // Localhost identifiers are not valid
			shouldBeValid: false,
		},
		{
			hostname:      "localhost6.localdomain6", // Localhost identifiers are not valid
			shouldBeValid: false,
		},
		{
			hostname:      "ip6-localhost", // Localhost identifiers are not valid
			shouldBeValid: false,
		},
	}

	for _, tt := range tests {
		err := ValidHostname(tt.hostname)
		if tt.shouldBeValid {
			assert.Nil(t, err)
		} else {
			assert.NotNil(t, err)
		}
	}
}
