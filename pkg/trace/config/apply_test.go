// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"encoding/json"
	"fmt"
	"github.com/mailru/easyjson/jlexer"
	"github.com/stretchr/testify/require"
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

// TestSQLObfuscationConfigDeserializationMethod checks if the use of easyjson results in the same deserialization
// output as encoding/json.
func TestSQLObfuscationConfigDeserializationMethod(t *testing.T) {
	mockSQLConfig := SQLObfuscationConfig{QuantizeSQLTables: true}
	mockSQLConfigBytes, err := json.Marshal(mockSQLConfig)
	require.NoError(t, err)

	var regularJSONDeserialized, easyJSONDeserialized SQLObfuscationConfig

	err = json.Unmarshal(mockSQLConfigBytes, &regularJSONDeserialized)
	require.NoError(t, err)

	jl := &jlexer.Lexer{Data: mockSQLConfigBytes}
	easyJSONDeserialized.UnmarshalEasyJSON(jl)
	require.NoError(t, jl.Error())

	assert.Equal(t, regularJSONDeserialized, easyJSONDeserialized)
}

func BenchmarkSQLObfuscationConfigEasyJSONDeserialization(b *testing.B) {
	for i := 1; i <= 100000; i *= 10 {
		b.Run(fmt.Sprintf("Range:%d", i), func(b *testing.B) {
			benchmarkSQLObfuscationConfigEasyJSONDeserialization(b)
		})
	}
}

func benchmarkSQLObfuscationConfigEasyJSONDeserialization(b *testing.B) {
	b.ReportAllocs()
	mockSQLConfig := SQLObfuscationConfig{QuantizeSQLTables: true}
	mockSQLConfigBytes, err := json.Marshal(mockSQLConfig)
	require.NoError(b, err)
	for i := 0; i < b.N; i++ {
		var sqlCfg SQLObfuscationConfig
		jl := &jlexer.Lexer{Data: mockSQLConfigBytes}
		sqlCfg.UnmarshalEasyJSON(jl)
		require.NoError(b, jl.Error())
	}
}
func BenchmarkSQLObfuscationConfigRegularJSONDeserialization(b *testing.B) {
	for i := 1; i <= 100000; i *= 10 {
		b.Run(fmt.Sprintf("Range:%d", i), func(b *testing.B) {
			benchmarkSQLObfuscationConfigRegularJSONDeserialization(b)
		})
	}
}

func benchmarkSQLObfuscationConfigRegularJSONDeserialization(b *testing.B) {
	b.ReportAllocs()
	mockSQLConfig := SQLObfuscationConfig{QuantizeSQLTables: true}
	mockSQLConfigBytes, err := json.Marshal(mockSQLConfig)
	require.NoError(b, err)
	for i := 0; i < b.N; i++ {
		var sqlCfg SQLObfuscationConfig
		err := json.Unmarshal(mockSQLConfigBytes, &sqlCfg)
		require.NoError(b, err)
	}
}
