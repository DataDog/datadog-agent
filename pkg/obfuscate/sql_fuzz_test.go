// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"encoding/json"
	"strings"
	"testing"
)

// FuzzSQLConfig represents a fuzzable version of SQLConfig with bounded values
type FuzzSQLConfig struct {
	DBMS                          uint8
	TableNames                    bool
	CollectCommands               bool
	CollectComments               bool
	CollectProcedures             bool
	ReplaceDigits                 bool
	KeepSQLAlias                  bool
	DollarQuotedFunc              bool
	ObfuscationMode               uint8
	RemoveSpaceBetweenParentheses bool
	KeepNull                      bool
	KeepBoolean                   bool
	KeepPositionalParameter       bool
	KeepTrailingSemicolon         bool
	KeepIdentifierQuotation       bool
	KeepJSONPath                  bool
}

func fuzzConfigToSQLConfig(fc FuzzSQLConfig) SQLConfig {
	// Map DBMS values to valid strings
	dbmsValues := []string{"", "mysql", "postgresql", "sqlserver", "oracle", "sqlite", "other"}
	dbms := dbmsValues[int(fc.DBMS)%len(dbmsValues)]

	// Map ObfuscationMode values
	obfuscationModes := []ObfuscationMode{"", NormalizeOnly, ObfuscateOnly, ObfuscateAndNormalize}
	mode := obfuscationModes[int(fc.ObfuscationMode)%len(obfuscationModes)]

	return SQLConfig{
		DBMS:                          dbms,
		TableNames:                    fc.TableNames,
		CollectCommands:               fc.CollectCommands,
		CollectComments:               fc.CollectComments,
		CollectProcedures:             fc.CollectProcedures,
		ReplaceDigits:                 fc.ReplaceDigits,
		KeepSQLAlias:                  fc.KeepSQLAlias,
		DollarQuotedFunc:              fc.DollarQuotedFunc,
		ObfuscationMode:               mode,
		RemoveSpaceBetweenParentheses: fc.RemoveSpaceBetweenParentheses,
		KeepNull:                      fc.KeepNull,
		KeepBoolean:                   fc.KeepBoolean,
		KeepPositionalParameter:       fc.KeepPositionalParameter,
		KeepTrailingSemicolon:         fc.KeepTrailingSemicolon,
		KeepIdentifierQuotation:       fc.KeepIdentifierQuotation,
		KeepJSONPath:                  fc.KeepJSONPath,
	}
}

func FuzzObfuscateSQLWithConfig(f *testing.F) {
	// Add seed corpus with various queries and configs, chosen from unit test examples.
	queries := []string{
		"SELECT * FROM users WHERE id = 42",
		"SELECT host, status FROM ec2_status WHERE org_id = 42",
		`SELECT "table"."field" FROM "table" WHERE "table"."otherfield" = $? AND "table"."thirdfield" = $?`,
		"select * from users where float = .43422",
		"INSERT INTO users (name, email) VALUES ('John', 'john@example.com')",
		"UPDATE users SET name = 'Jane' WHERE id = 123",
		"DELETE FROM users WHERE created_at < '2020-01-01'",
		"SELECT a FROM b WHERE a.x !* 2",
		"SELECT a FROM b WHERE a.x !& 2",
		"USING $09 SELECT",
		"USING - SELECT",
		`SELECT nspname FROM pg_class where nspname ~ '.*matching.*'`,
		`SELECT nspname FROM pg_class where nspname !~ '.*toIgnore.*'`,
		`/* Multi-line comment */ SELECT * FROM clients WHERE (clients.first_name = 'Andy') LIMIT 1`,
		"SELECT Codi , Nom_CA AS Nom, Descripció_CAT AS Descripció FROM ProtValAptitud WHERE Vigent=1 ORDER BY Ordre, Codi",
		"SELECT 🥒",
		"",
		" ",
		"\n",
		"SELECT",
		"'unclosed string",
		`"unclosed double quote`,
		"SELECT %(asd)| FROM profile",
		"SELECT * FROM users WHERE id = -1 OR id = -01 OR id = -108 OR id = -.018 OR id = -.08 OR id = -908129",
		"SELECT * FROM `backslashes` WHERE id = 42",
		`SELECT * FROM "double-quotes" WHERE id = 42`,
		"BEGIN INSERT INTO owners (created_at, first_name) VALUES ('2011-08-30 05:22:57', 'Andy') COMMIT",
		"SELECT /*+ INDEX(t idx_col1) */ * FROM table t WHERE col1 = 1",
		`SELECT $1, $2, $3 FROM users WHERE id = $4`,
		`SELECT $$dollar quoted string$$ FROM users`,
		`SELECT $tag$dollar quoted with tag$tag$ FROM users`,
		"SELECT E'\\n' FROM users",
		"SELECT * FROM users WHERE data::jsonb @> '{\"key\": \"value\"}'",
		"SELECT * FROM users WHERE id IN (1, 2, 3, 4, 5)",
		"REPLACE INTO sales_2019_07_01 (`itemID`, `date`, `qty`, `price`) VALUES (1234, CURDATE(), 10, 0.00)",
		"WITH sales AS (SELECT * FROM revenue) SELECT * FROM sales",
		"SELECT ddh19.name FROM dd91219.host ddh19 WHERE ddh19.org_id = 2",
		"exec sp_executesql @statement = 'SELECT * FROM users WHERE id = @id', @params = '@id int', @id = 42",
		string([]byte{0x00, 0x01, 0x02}), // null bytes
		"\xc3\x28",                       // invalid UTF-8
		string(make([]byte, 10000)),      // large input
	}

	// Add test cases with specific configurations
	testCases := []struct {
		query  string
		config FuzzSQLConfig
	}{
		// Test each query with default config
		{
			"SELECT * FROM users WHERE id = 42",
			FuzzSQLConfig{},
		},
		// Test with various DBMS settings
		{
			"SELECT * FROM users WHERE id = 42",
			FuzzSQLConfig{DBMS: 1, TableNames: true, ReplaceDigits: true},
		},
		{
			"SELECT $1, $2, $3 FROM users WHERE id = $4",
			FuzzSQLConfig{DBMS: 2, KeepPositionalParameter: true, ObfuscationMode: 3},
		},
		{
			"SELECT data::jsonb -> 'name' FROM users",
			FuzzSQLConfig{DBMS: 2, KeepJSONPath: true, ObfuscationMode: 3},
		},
		{
			"SELECT * FROM [my_table] WHERE [column] = 'value'",
			FuzzSQLConfig{DBMS: 3, KeepIdentifierQuotation: true, ObfuscationMode: 2},
		},
		{
			`SELECT $$dollar quoted string$$ FROM users`,
			FuzzSQLConfig{DBMS: 2, DollarQuotedFunc: true},
		},
		{
			"/* comment */ SELECT * FROM users; -- another comment",
			FuzzSQLConfig{CollectComments: true, KeepTrailingSemicolon: true},
		},
		{
			"SELECT * FROM users123 WHERE active = TRUE AND deleted IS NULL",
			FuzzSQLConfig{ReplaceDigits: true, KeepBoolean: true, KeepNull: true, ObfuscationMode: 3},
		},
		{
			"EXEC sp_executesql @statement = 'SELECT * FROM users'",
			FuzzSQLConfig{DBMS: 3, CollectProcedures: true},
		},
		{
			"SELECT username AS person FROM users",
			FuzzSQLConfig{KeepSQLAlias: true},
		},
		{
			"SELECT COUNT( * ) FROM users",
			FuzzSQLConfig{RemoveSpaceBetweenParentheses: true, ObfuscationMode: 1},
		},
		{
			"BEGIN; INSERT INTO users VALUES (1); COMMIT;",
			FuzzSQLConfig{CollectCommands: true, TableNames: true},
		},
		// Edge case: conflicting options
		{
			"SELECT * FROM users WHERE id = 123",
			FuzzSQLConfig{
				ObfuscationMode: 1,    // NormalizeOnly
				KeepNull:        true, // Only valid for obfuscation modes
				KeepBoolean:     true, // Only valid for obfuscation modes
			},
		},
		// All options enabled
		{
			"SELECT * FROM users",
			FuzzSQLConfig{
				DBMS:                          5,
				TableNames:                    true,
				CollectCommands:               true,
				CollectComments:               true,
				CollectProcedures:             true,
				ReplaceDigits:                 true,
				KeepSQLAlias:                  true,
				DollarQuotedFunc:              true,
				ObfuscationMode:               3,
				RemoveSpaceBetweenParentheses: true,
				KeepNull:                      true,
				KeepBoolean:                   true,
				KeepPositionalParameter:       true,
				KeepTrailingSemicolon:         true,
				KeepIdentifierQuotation:       true,
				KeepJSONPath:                  true,
			},
		},
	}

	// Add all queries with various configs to the seed corpus
	for _, query := range queries {
		// Test with multiple different configs
		configs := []FuzzSQLConfig{
			{}, // Default config
			{DBMS: 1, ReplaceDigits: true},
			{DBMS: 2, DollarQuotedFunc: true},
			{ObfuscationMode: 1},
			{ObfuscationMode: 2},
			{ObfuscationMode: 3},
			{TableNames: true, CollectCommands: true, CollectComments: true},
		}
		for _, config := range configs {
			testCases = append(testCases, struct {
				query  string
				config FuzzSQLConfig
			}{query, config})
		}
	}

	for _, tc := range testCases {
		f.Add(tc.query, tc.config.DBMS, tc.config.TableNames, tc.config.CollectCommands,
			tc.config.CollectComments, tc.config.CollectProcedures, tc.config.ReplaceDigits,
			tc.config.KeepSQLAlias, tc.config.DollarQuotedFunc, tc.config.ObfuscationMode,
			tc.config.RemoveSpaceBetweenParentheses, tc.config.KeepNull, tc.config.KeepBoolean,
			tc.config.KeepPositionalParameter, tc.config.KeepTrailingSemicolon,
			tc.config.KeepIdentifierQuotation, tc.config.KeepJSONPath)
	}

	o := NewObfuscator(Config{})

	f.Fuzz(func(t *testing.T, query string, dbms uint8, tableNames, collectCommands,
		collectComments, collectProcedures, replaceDigits, keepSQLAlias, dollarQuotedFunc bool,
		obfuscationMode uint8, removeSpaceBetweenParentheses, keepNull, keepBoolean,
		keepPositionalParameter, keepTrailingSemicolon, keepIdentifierQuotation, keepJSONPath bool) {

		fuzzConfig := FuzzSQLConfig{
			DBMS:                          dbms,
			TableNames:                    tableNames,
			CollectCommands:               collectCommands,
			CollectComments:               collectComments,
			CollectProcedures:             collectProcedures,
			ReplaceDigits:                 replaceDigits,
			KeepSQLAlias:                  keepSQLAlias,
			DollarQuotedFunc:              dollarQuotedFunc,
			ObfuscationMode:               obfuscationMode,
			RemoveSpaceBetweenParentheses: removeSpaceBetweenParentheses,
			KeepNull:                      keepNull,
			KeepBoolean:                   keepBoolean,
			KeepPositionalParameter:       keepPositionalParameter,
			KeepTrailingSemicolon:         keepTrailingSemicolon,
			KeepIdentifierQuotation:       keepIdentifierQuotation,
			KeepJSONPath:                  keepJSONPath,
		}

		sqlConfig := fuzzConfigToSQLConfig(fuzzConfig)

		// The obfuscator should never panic with any configuration
		result, err := o.ObfuscateSQLStringWithOptions(query, &sqlConfig, "")

		// Empty or whitespace-only query behavior might vary based on obfuscation mode
		if strings.TrimSpace(query) == "" {
			// With go-sqllexer modes, empty queries might be
			// handled differently. Some configurations might return
			// empty results for whitespace-only queries.
			return
		}

		// Non-empty queries should either succeed or return an error, never panic
		if err == nil {
			// Successful obfuscation invariants
			if result == nil {
				t.Errorf("ObfuscateSQLStringWithOptions(%q, %+v): got nil result with nil error", query, sqlConfig)
			}
		}

		// Test that we can serialize the config to JSON (as done in NewObfuscator)
		// This should never panic
		_, _ = json.Marshal(sqlConfig)
	})
}

func FuzzObfuscateSQLExecPlan(f *testing.F) {
	// Add seed corpus for execution plans
	plans := []string{
		`{"Plan": {"Node Type": "Seq Scan", "Relation Name": "users", "Total Cost": 10.50}}`,
		`{"Plan": {"Node Type": "Index Scan", "Index Name": "users_pkey", "Filter": "(id = 42)"}}`,
		`[{"Plan": {"Node Type": "Nested Loop", "Plans": [{"Node Type": "Seq Scan"}]}}]`,
		`{"Plan": {"Node Type": "Hash Join", "Hash Cond": "(u.id = o.user_id)"}}`,
		"{}",
		"[]",
		"",
		"null",
		`{"Plan": null}`,
		`{"Plan": {"Filter": "((status)::text = 'active'::text)", "Rows": 1000}}`,
		`{"Plan": {"Output": ["id", "name", "email"], "Total Cost": 123.45}}`,
		`{"invalid json`,
		`{"Plan": {"Node Type": "Aggregate", "Group Key": ["user_id"], "Filter": "(count(*) > 10)"}}`,
		`{"Plan": {"Node Type": "Index Scan", "Total Cost": 123.45, "Rows": 1000}}`,
		`{"Plan": {"Filter": "((status)::text = 'active'::text)"}}`,
	}

	// Test with different normalize settings
	for _, plan := range plans {
		f.Add(plan, false)
		f.Add(plan, true)
	}

	o := NewObfuscator(Config{
		SQLExecPlan: JSONConfig{
			Enabled: true,
		},
		SQLExecPlanNormalize: JSONConfig{
			Enabled: true,
		},
	})

	f.Fuzz(func(t *testing.T, jsonPlan string, normalize bool) {
		// No panics
		result, err := o.ObfuscateSQLExecPlan(jsonPlan, normalize)

		// Basic invariants
		if err == nil && result == "" && jsonPlan != "" {
			t.Errorf("ObfuscateSQLExecPlan(%q, %v): got empty result for non-empty input", jsonPlan, normalize)
		}

		// If input was valid JSON and obfuscation succeeded, output
		// should be valid JSON.
		if err == nil && isValidJSON(jsonPlan) && !isValidJSON(result) {
			t.Errorf("ObfuscateSQLExecPlan(%q, %v): result is not valid JSON: %s", jsonPlan, normalize, result)
		}
	})
}

func isValidJSON(s string) bool {
	var v interface{}
	return json.Unmarshal([]byte(s), &v) == nil
}
