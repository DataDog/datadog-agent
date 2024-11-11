// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// TODO investigate flaky unit tests on windows
//go:build !windows

package server

import (
	_ "embed"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactHeader(t *testing.T) {
	header := http.Header{
		"Foo":       []string{"bar"},
		"Key":       []string{"AAAAAAA"},
		"appkEy":    []string{"BBBBBBB"},
		"other-KeY": []string{"CCCCCCC", "DDDDDDD"},
	}
	redactedHeader := redactHeader(header)
	expectedHeader := http.Header{
		"Foo":       []string{"bar"},
		"Key":       []string{"<redacted>"},
		"Appkey":    []string{"<redacted>"},
		"Other-Key": []string{"<redacted>"},
	}
	expectedKeys := []string{}
	for key, expectedValues := range expectedHeader {
		redactedValues := redactedHeader[key]
		assert.Equal(t, expectedValues, redactedValues, "unexpected value at key %s", key)
		expectedKeys = append(expectedKeys, key)
	}
	redactedKeys := []string{}
	for key, redactedValues := range redactedHeader {
		expectedValues := expectedHeader[key]
		assert.Equal(t, expectedValues, redactedValues, "unexpected value at key %s", key)
		redactedKeys = append(redactedKeys, key)
	}
	assert.Equal(t, len(expectedKeys), len(redactedKeys), "unexpected length in keys")
}
