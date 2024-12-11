// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSkipOIDRowsNaive(t *testing.T) {
	for _, tc := range []struct{ oid, expected string }{
		{"1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.1.0"},
		{".1.3.6.1.2.1.1.1.0.", "1.3.6.1.2.1.1.1.0"},
		{"1.3.6.1.2.1.1.9.1.2.1", "1.3.6.1.2.1.1.9.1.3"},
		// breakdown example: column ID 1, key 127.0.0.1 is interpreted as column ID 127
		{"1.3.6.1.2.1.4.20.1.1.127.0.0.1", "1.3.6.1.2.1.4.20.1.1.128"},
		// breakdown example: column ID 1, key ending in 0 is interpreted as scalar
		{"1.3.6.1.2.1.4.24.4.1.1.195.200.251.0.0.255.255.255.0.0.0.0.0", "1.3.6.1.2.1.4.24.4.1.1.195.200.251.0.0.255.255.255.0.0.0.0.0"},
		// breakdown example: key containing '.1.':
		{"1.3.6.1.2.1.4.22.1.1.2.192.168.1.1", "1.3.6.1.2.1.4.22.1.1.2.192.168.1.2"},
	} {
		assert.Equal(t, tc.expected, SkipOIDRowsNaive(tc.oid))
	}
}
