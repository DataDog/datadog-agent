// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"encoding/json"
	"testing"
)

func TestGetVersion(t *testing.T) {
	testGetVersion(t)
}

func TestGetHostname(t *testing.T) {
	testGetHostname(t)
}

func TestGetClusterName(t *testing.T) {
	testGetClusterName(t)
}

func TestHeaders(t *testing.T) {
	testHeaders(t)
}

func TestGetConfig(t *testing.T) {
	testGetConfig(t)
}

func TestSetExternalTags(t *testing.T) {
	testSetExternalTags(t)
}

func TestEmitAgentTelemetry(t *testing.T) {
	testEmitAgentTelemetry(t)
}

func TestObfuscaterConfig(t *testing.T) {
	testObfuscaterConfig(t)
}

func TestLoadSQLConfig(t *testing.T) {
	testLoadSQLConfig(t)
}

func TestObfuscateSQL(t *testing.T) {
	testObfuscateSQL(t)
}

// BenchmarkLoadSQLConfig compares the memoized lookup against the json.Unmarshal it replaces.
func BenchmarkLoadSQLConfig(b *testing.B) {
	optStr := `{"dbms":"postgresql","obfuscation_mode":"obfuscate_and_normalize","table_names":true,"collect_commands":true,"collect_comments":true,"keep_sql_alias":true,"dollar_quoted_func":true,"return_json_metadata":true}`

	b.Run("Memoized", func(b *testing.B) {
		if _, err := loadSQLConfig(optStr); err != nil { // prime the cache
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := loadSQLConfig(optStr); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("UnmarshalEachCall", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var c sqlConfig
			if err := json.Unmarshal([]byte(optStr), &c); err != nil {
				b.Fatal(err)
			}
		}
	})
}
