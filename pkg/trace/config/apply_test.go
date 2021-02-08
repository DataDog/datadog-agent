// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestParseReplaceRules tests the compileReplaceRules helper function.
func TestParseRepaceRules(t *testing.T) {
	assert := assert.New(t)
	rules := []*ReplaceRule{
		{Name: "http.url", Pattern: "(token/)([^/]*)", Repl: "${1}?"},
		{Name: "http.url", Pattern: "guid", Repl: "[REDACTED]"},
		{Name: "custom.tag", Pattern: "(/foo/bar/).*", Repl: "${1}extra"},
	}
	err := compileReplaceRules(rules)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		assert.Equal(r.Pattern, r.Re.String())
	}
}

// TestSplitTag tests the splitTag helper function.
func TestSplitTag(t *testing.T) {
	for _, tt := range []struct {
		tag string
		kv  *Tag
	}{
		{
			tag: "",
			kv:  &Tag{K: ""},
		},
		{
			tag: "key:value",
			kv:  &Tag{K: "key", V: "value"},
		},
		{
			tag: "env:prod",
			kv:  &Tag{K: "env", V: "prod"},
		},
		{
			tag: "env:staging:east",
			kv:  &Tag{K: "env", V: "staging:east"},
		},
		{
			tag: "key",
			kv:  &Tag{K: "key"},
		},
	} {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, splitTag(tt.tag), tt.kv)
		})
	}
}
