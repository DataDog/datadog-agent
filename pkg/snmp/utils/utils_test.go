// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateNamespace(t *testing.T) {
	assert := assert.New(t)
	long := strings.Repeat("a", 105)
	_, err := NormalizeNamespace(long)
	assert.NotNil(err, "namespace should not be too long")

	namespace, err := NormalizeNamespace("a<b")
	assert.Nil(err, "namespace with symbols should be normalized")
	assert.Equal("a-b", namespace, "namespace should not contain symbols")

	namespace, err = NormalizeNamespace("a\nb")
	assert.Nil(err, "namespace with symbols should be normalized")
	assert.Equal("ab", namespace, "namespace should not contain symbols")

	// Invalid namespace as bytes that would look like this: 9cbef2d1-8c20-4bf2-97a5-7d70��
	b := []byte{
		57, 99, 98, 101, 102, 50, 100, 49, 45, 56, 99, 50, 48, 45,
		52, 98, 102, 50, 45, 57, 55, 97, 53, 45, 55, 100, 55, 48,
		0, 0, 0, 0, 239, 191, 189, 239, 191, 189, 1, // these are bad bytes
	}
	_, err = NormalizeNamespace(string(b))
	assert.NotNil(err, "namespace should not contain bad bytes")
}
