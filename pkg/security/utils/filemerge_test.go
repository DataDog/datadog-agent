// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommonPrefix(t *testing.T) {
	tests := []struct {
		a, b, prefix string
	}{
		{"test", "tesh", "tes"},
		{"test", "test", "test"},
		{"a", "b", ""},
		{"", "test", ""},
		{"test", "", ""},
		{"", "", ""},
	}

	for _, entry := range tests {
		a := NewStringPair(entry.a)
		b := NewStringPair(entry.b)
		detected := commonPrefix(a, b)
		assert.Equal(t, entry.prefix, detected)
	}
}

func TestCommonSuffix(t *testing.T) {
	tests := []struct {
		a, b, suffix string
	}{
		{"test", "hest", "est"},
		{"test", "test", "test"},
		{"a", "b", ""},
		{"", "test", ""},
		{"test", "", ""},
		{"", "", ""},
	}

	for _, entry := range tests {
		a := NewStringPair(entry.a)
		b := NewStringPair(entry.b)
		detected := commonSuffix(a, b)
		assert.Equal(t, entry.suffix, detected)
	}
}
