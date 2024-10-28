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

func TestNormalizeHost(t *testing.T) {
	assert := assert.New(t)
	long := strings.Repeat("a", 256)
	hostname, err := NormalizeHost(long)
	assert.NotNil(err, "long host should return error")
	assert.Len(hostname, 0, "long host should be dropped")

	hostname, err = NormalizeHost("a<b")
	assert.Nil(err, "< should not return error")
	assert.Equal("a-b", hostname, "< defanged")

	hostname, err = NormalizeHost("a>b")
	assert.Nil(err, "> should not return error")
	assert.Equal("a-b", hostname, "> defanged")

	hostname, err = NormalizeHost("example.com\r\n")
	assert.Nil(err, "NL/CR should not return error")
	assert.Equal("example.com", hostname, "NL/CR dropped")

	// Invalid host name as bytes that would look like this: 9cbef2d1-8c20-4bf2-97a5-7d70��
	b := []byte{
		57, 99, 98, 101, 102, 50, 100, 49, 45, 56, 99, 50, 48, 45,
		52, 98, 102, 50, 45, 57, 55, 97, 53, 45, 55, 100, 55, 48,
		0, 0, 0, 0, 239, 191, 189, 239, 191, 189, 1, // these are bad bytes
	}
	hostname, err = NormalizeHost(string(b))
	assert.NotNil(err, "null rune should return error")
	assert.Equal("", hostname, "host with null rune should be dropped")
}
