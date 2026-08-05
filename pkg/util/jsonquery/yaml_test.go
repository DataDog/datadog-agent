// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package jsonquery

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Data contains YAML code
type data []byte

func TestYAMLExistQuery(t *testing.T) {
	exist, err := YAMLCheckExist(data("{\"ip_address\": \"127.0.0.50\"}"), ".ip_address == \"127.0.0.50\"")
	assert.NoError(t, err)
	assert.True(t, exist)

	exist, err = YAMLCheckExist(data("{\"ip_address\": \"127.0.0.50\"}"), ".ip_address == \"127.0.0.99\"")
	assert.NoError(t, err)
	assert.False(t, exist)

	exist, err = YAMLCheckExist(data("{\"ip_address\": \"127.0.0.50\"}"), ".ip_address")
	assert.EqualError(t, err, "filter query must return a boolean: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `127.0.0.50` into bool")
	assert.False(t, exist)

	exist, err = YAMLCheckExist(data("{}"), ".ip_address == \"127.0.0.99\"")
	assert.NoError(t, err)
	assert.False(t, exist)
}

func TestYAMLExistQueryBlockYAML(t *testing.T) {
	instance := data(`
host: localhost
port: 8080
tags:
  - env:prod
  - team:fleet
tls:
  verify: true
`)

	tests := []struct {
		name   string
		query  string
		expect bool
	}{
		{name: "string equality", query: `.host == "localhost"`, expect: true},
		{name: "numeric comparison", query: ".port > 1024", expect: true},
		{name: "numeric comparison false", query: ".port < 1024", expect: false},
		{name: "nested boolean", query: ".tls.verify", expect: true},
		{name: "has on present key", query: `has("tags")`, expect: true},
		{name: "has on absent key", query: `has("password")`, expect: false},
		{name: "array membership", query: `.tags | index("env:prod") != null`, expect: true},
		{name: "array membership absent", query: `.tags | index("env:dev") != null`, expect: false},
		{name: "any over array", query: `any(.tags[]; startswith("team:"))`, expect: true},
		{name: "missing key defaults", query: `(.password // "") == ""`, expect: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exist, err := YAMLCheckExist(instance, tt.query)
			assert.NoError(t, err)
			assert.Equal(t, tt.expect, exist)
		})
	}
}

func TestYAMLExistQueryErrors(t *testing.T) {
	t.Run("invalid YAML", func(t *testing.T) {
		exist, err := YAMLCheckExist(data("key: [unterminated"), ".key")
		assert.Error(t, err)
		assert.False(t, exist)
	})

	t.Run("invalid query", func(t *testing.T) {
		exist, err := YAMLCheckExist(data("key: value"), ".%bad")
		assert.Error(t, err)
		assert.False(t, exist)
	})

	t.Run("runtime error", func(t *testing.T) {
		// Indexing a string raises a jq runtime error.
		exist, err := YAMLCheckExist(data("key: value"), ".key.nested")
		assert.Error(t, err)
		assert.False(t, exist)
	})

	t.Run("non-boolean output", func(t *testing.T) {
		exist, err := YAMLCheckExist(data("key: 42"), ".key")
		assert.ErrorContains(t, err, "filter query must return a boolean")
		assert.False(t, exist)
	})

	t.Run("no output matches nothing", func(t *testing.T) {
		exist, err := YAMLCheckExist(data("key: value"), "empty")
		assert.NoError(t, err)
		assert.False(t, exist)
	})

	t.Run("null output matches nothing", func(t *testing.T) {
		exist, err := YAMLCheckExist(data("key: value"), ".absent")
		assert.NoError(t, err)
		assert.False(t, exist)
	})

	t.Run("YAML-style boolean strings are still accepted", func(t *testing.T) {
		// Historical leniency: the filter output is parsed the way YAML would parse it,
		// so a string that YAML reads as a boolean matches.
		for _, value := range []string{"true", "True", "TRUE", "false", "False"} {
			exist, err := YAMLCheckExist(data("enabled: \""+value+"\""), ".enabled")
			assert.NoError(t, err, "value %q", value)
			assert.Equal(t, strings.EqualFold(value, "true"), exist, "value %q", value)
		}
	})

	t.Run("empty document", func(t *testing.T) {
		// An empty instance normalizes to `null`, which jq indexes without erroring.
		exist, err := YAMLCheckExist(data(""), `.host == "localhost"`)
		assert.NoError(t, err)
		assert.False(t, exist)
	})
}

func BenchmarkYAMLCheckExist(b *testing.B) {
	instance := data(`
host: localhost
port: 8080
tags:
  - env:prod
  - team:fleet
tls:
  verify: true
`)

	for b.Loop() {
		if _, err := YAMLCheckExist(instance, `.host == "localhost"`); err != nil {
			b.Fatal(err)
		}
	}
}
