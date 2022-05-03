// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewStringPair(t *testing.T) {
	tests := []struct {
		input, left, right string
		isPattern          bool
	}{
		{"test", "test", "", false},
		{"test*", "test", "", true},
		{"a*b", "a", "b", true},
		{"*", "", "", true},
		{"", "", "", false},
	}

	for _, entry := range tests {
		sp := NewStringPair(entry.input)
		assert.Equal(t, entry.left, sp.left)
		assert.Equal(t, entry.right, sp.right)
		assert.Equal(t, entry.isPattern, sp.isPattern)
	}
}

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

func TestBuildGlob(t *testing.T) {
	tests := []struct {
		a, b, glob string
		merge      bool
	}{
		{"prefixaasuffix", "prefixbbsuffix", "prefix*suffix", true},
		{"test", "hello", "", false},
	}

	minLenMatch := 3

	for _, entry := range tests {
		a := NewStringPair(entry.a)
		b := NewStringPair(entry.b)
		sp, merge := BuildGlob(a, b, minLenMatch)
		assert.Equal(t, entry.merge, merge)
		assert.Equal(t, entry.glob, sp.ToGlob())
	}
}
