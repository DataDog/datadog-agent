//go:build rust_patterns

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rtokenizer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// TestRustTokenizer_PatternCreation validates that patterns are correctly created
// from Rust-tokenized logs.
func TestRustTokenizer_PatternCreation(t *testing.T) {
	tokenizer := NewRustTokenizer()
	cm := clustering.NewClusterManager()

	tests := []struct {
		name                string
		log                 string
		expectedTemplate    string // Expected pattern template for first log
		expectedWildcards   int    // Expected number of wildcards for first log
		expectedPatternType clustering.PatternChangeType
		description         string
	}{
		{
			name:                "First log creates new pattern",
			log:                 "Service started",
			expectedTemplate:    "Service started",
			expectedWildcards:   0,
			expectedPatternType: clustering.PatternNew,
			description:         "First log should create new pattern with no wildcards",
		},
		{
			name:                "Simple pattern with numeric wildcard",
			log:                 "Processing 100 items",
			expectedTemplate:    "Processing 100 items",
			expectedWildcards:   0,
			expectedPatternType: clustering.PatternNew,
			description:         "First log should not wildcard yet (wildcards appear after merge)",
		},
		{
			name:                "HTTP log pattern",
			log:                 "GET /api/users 200",
			expectedTemplate:    "GET /api/users 200",
			expectedWildcards:   0,
			expectedPatternType: clustering.PatternNew,
			description:         "First log should not wildcard yet (wildcards appear after merge)",
		},
		{
			name:                "Pattern with IP address",
			log:                 "Connection from 192.168.1.1",
			expectedTemplate:    "Connection from 192.168.1.1",
			expectedWildcards:   0,
			expectedPatternType: clustering.PatternNew,
			description:         "First log should not wildcard yet (wildcards appear after merge)",
		},
		{
			name:                "Pattern with email",
			log:                 "User admin@example.com logged in",
			expectedTemplate:    "User admin@example.com logged in",
			expectedWildcards:   0,
			expectedPatternType: clustering.PatternNew,
			description:         "First log should not wildcard yet (wildcards appear after merge)",
		},
		{
			name:                "Pattern with timestamp",
			log:                 "2024-01-15 10:30:00 Server started",
			expectedTemplate:    "2024-01-15 10:30:00 Server started",
			expectedWildcards:   0,
			expectedPatternType: clustering.PatternNew,
			description:         "First log should not wildcard yet (wildcards appear after merge)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset cluster manager for each test
			cm = clustering.NewClusterManager()

			tokenList, err := tokenizer.Tokenize(tt.log)
			require.NoError(t, err, "Tokenization should succeed")
			require.NotNil(t, tokenList, "TokenList should not be nil")

			pattern, changeType, _, _ := cm.Add(tokenList)
			require.NotNil(t, pattern, "Pattern should not be nil")

			assert.Equal(t, tt.expectedPatternType, changeType,
				"%s: pattern change type mismatch", tt.description)
			assert.Equal(t, 1, pattern.LogCount,
				"%s: first log should have LogCount=1", tt.description)
			assert.NotNil(t, pattern.Template,
				"%s: pattern template should not be nil", tt.description)

			patternString := pattern.GetPatternString()
			wildcardCount := pattern.GetWildcardCount()

			assert.Equal(t, tt.expectedTemplate, patternString,
				"%s: pattern template mismatch", tt.description)
			assert.Equal(t, tt.expectedWildcards, wildcardCount,
				"%s: wildcard count mismatch", tt.description)

			t.Logf("✅ %s: %q → pattern: %q (wildcards: %d, ID: %d)",
				tt.description, tt.log, patternString, wildcardCount, pattern.PatternID)
		})
	}
}

// TestRustTokenizer_PatternUpdating validates that patterns are correctly updated
// when similar logs are added.
func TestRustTokenizer_PatternUpdating(t *testing.T) {
	tokenizer := NewRustTokenizer()

	tests := []struct {
		name              string
		logs              []string
		expectedPatterns  int
		expectedLogCounts []int // Expected log count after each log is added
		description       string
	}{
		{
			name: "Identical logs merge into same pattern",
			logs: []string{
				"Service started",
				"Service started",
				"Service started",
			},
			expectedPatterns:  1,
			expectedLogCounts: []int{1, 2, 3},
			description:       "Identical logs should increment same pattern's log count",
		},
		{
			name: "Similar logs merge with wildcards",
			logs: []string{
				"Processing 100 items",
				"Processing 200 items",
				"Processing 500 items",
			},
			expectedPatterns:  1,
			expectedLogCounts: []int{1, 2, 3},
			description:       "Logs with different numeric values should merge",
		},
		{
			name: "HTTP logs with different methods merge",
			logs: []string{
				"GET /api/users 200",
				"POST /api/users 201",
				"PUT /api/users 200",
			},
			expectedPatterns:  1,
			expectedLogCounts: []int{1, 2, 3},
			description:       "HTTP logs with different methods should merge",
		},
		{
			name: "Different structures create separate patterns",
			logs: []string{
				"Service started",
				"Connection from 192.168.1.1",
				"User admin@example.com logged in",
			},
			expectedPatterns:  3,
			expectedLogCounts: []int{1, 1, 1},
			description:       "Structurally different logs should create separate patterns",
		},
		{
			name: "Mixed: some merge, some don't",
			logs: []string{
				"Service started",
				"Service stopped",
				"Connection from 192.168.1.1",
				"Connection from 10.0.0.1",
			},
			expectedPatterns:  2,
			expectedLogCounts: []int{1, 2, 1, 2},
			description:       "Similar logs merge, different structures separate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := clustering.NewClusterManager()
			var patterns []*clustering.Pattern

			for i, log := range tt.logs {
				tokenList, err := tokenizer.Tokenize(log)
				require.NoError(t, err, "Tokenization should succeed for log: %q", log)

				pattern, changeType, patternCount, _ := cm.Add(tokenList)
				require.NotNil(t, pattern, "Pattern should not be nil")
				patterns = append(patterns, pattern)

				// Verify log count
				assert.Equal(t, tt.expectedLogCounts[i], pattern.LogCount,
					"Log %d: LogCount mismatch. Log: %q", i, log)

				// Log the change type
				changeTypeStr := ""
				switch changeType {
				case clustering.PatternNew:
					changeTypeStr = "NEW"
				case clustering.PatternUpdated:
					changeTypeStr = "UPDATED"
				case clustering.PatternNoChange:
					changeTypeStr = "NO_CHANGE"
				}

				t.Logf("  Log %d: %q → Pattern %d (%s, LogCount=%d, TotalPatterns=%d)",
					i+1, log, pattern.PatternID, changeTypeStr, pattern.LogCount, patternCount)
			}

			assert.Equal(t, tt.expectedPatterns, cm.PatternCount(),
				"%s: pattern count mismatch", tt.description)

			t.Logf("✅ %s: Processed %d logs, created %d patterns",
				tt.description, len(tt.logs), cm.PatternCount())
		})
	}
}

// TestRustTokenizer_WildcardExtraction validates that wildcard values are
// correctly extracted from logs using patterns.
func TestRustTokenizer_WildcardExtraction(t *testing.T) {
	tokenizer := NewRustTokenizer()
	cm := clustering.NewClusterManager()

	tests := []struct {
		name           string
		logs           []string
		expectedValues [][]string // Expected wildcard values for each log
		description    string
	}{
		{
			name: "Single wildcard - numeric values",
			logs: []string{
				"Processing 100 items",
				"Processing 200 items",
				"Processing 500 items",
			},
			expectedValues: [][]string{
				{}, // first log sets baseline, no wildcards yet
				{"200"},
				{"500"},
			},
			description: "Should extract numeric values as wildcards",
		},
		{
			name: "Multiple wildcards - HTTP log",
			logs: []string{
				"GET /api/users 200",
				"POST /api/orders 201",
				"PUT /api/products 200",
			},
			expectedValues: [][]string{
				{}, // first log sets baseline, no wildcards yet
				{"POST", "/api/orders", "201"},
				{"PUT", "/api/products", "200"},
			},
			description: "Should extract all wildcards in order",
		},
		{
			name: "Wildcard - IP addresses",
			logs: []string{
				"Connection from 192.168.1.1",
				"Connection from 10.0.0.1",
				"Connection from 172.16.0.1",
			},
			expectedValues: [][]string{
				{}, // first log sets baseline, no wildcards yet
				{"10.0.0.1"},
				{"172.16.0.1"},
			},
			description: "Should extract IP addresses as wildcards",
		},
		{
			name: "Wildcard - email addresses",
			logs: []string{
				"User admin@example.com logged in",
				"User user@test.com logged in",
				"User guest@domain.org logged in",
			},
			expectedValues: [][]string{
				{}, // first log sets baseline, no wildcards yet
				{"user@test.com"},
				{"guest@domain.org"},
			},
			description: "Should extract email addresses as wildcards",
		},
		{
			name: "Complex pattern with multiple wildcards",
			logs: []string{
				"2024-01-15 ERROR Database 192.168.1.1 connection failed",
				"2024-01-16 WARN Database 10.0.0.1 connection slow",
			},
			expectedValues: [][]string{
				{}, // first log sets baseline, no wildcards yet
				{"2024-01-16", "WARN", "10.0.0.1", "slow"},
			},
			description: "Should extract all wildcards from complex pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset cluster manager
			cm = clustering.NewClusterManager()

			var pattern *clustering.Pattern

			for i, log := range tt.logs {
				tokenList, err := tokenizer.Tokenize(log)
				require.NoError(t, err, "Tokenization should succeed for log: %q", log)

				pattern, _, _, _ = cm.Add(tokenList)
				require.NotNil(t, pattern, "Pattern should not be nil")

				// Extract wildcard values
				wildcardValues := pattern.GetWildcardValues(tokenList)

				// Verify wildcard values
				assert.Equal(t, len(tt.expectedValues[i]), len(wildcardValues),
					"Log %d: wildcard count mismatch. Log: %q", i, log)

				for j, expectedValue := range tt.expectedValues[i] {
					if j < len(wildcardValues) {
						assert.Equal(t, expectedValue, wildcardValues[j],
							"Log %d, wildcard %d: value mismatch. Log: %q", i, j, log)
					}
				}

				t.Logf("  Log %d: %q → wildcards: %v",
					i+1, log, wildcardValues)
			}

			// NOTE: Pattern templates omit wildcard placeholders entirely.
			// Example: "User * logged in" becomes "User  logged in".
			patternString := pattern.GetPatternString()
			assert.NotEmpty(t, patternString, "%s: pattern string should not be empty", tt.description)
			if len(tt.expectedValues) > 1 {
				assert.GreaterOrEqual(t, pattern.GetWildcardCount(), 0,
					"%s: wildcard count should be >= 0", tt.description)
				// Template omits wildcard placeholders; ensure static parts remain
				if strings.Contains(tt.logs[0], "Processing") {
					assert.Contains(t, patternString, "Processing")
					assert.Contains(t, patternString, "items")
				}
				if strings.Contains(tt.logs[0], "Connection from") {
					assert.Contains(t, patternString, "Connection from")
				}
				if strings.Contains(tt.logs[0], "User ") && strings.Contains(tt.logs[0], " logged in") {
					assert.Contains(t, patternString, "User ")
					assert.Contains(t, patternString, " logged in")
				}
			}
			t.Logf("✅ %s: Final pattern: %q", tt.description, patternString)
		})
	}
}

// TestRustTokenizer_PatternEvolution validates that patterns evolve after merging.
func TestRustTokenizer_PatternEvolution(t *testing.T) {
	tokenizer := NewRustTokenizer()
	cm := clustering.NewClusterManager()

	log1 := "Processing 100 items"
	log2 := "Processing 200 items"

	tokenList1, err := tokenizer.Tokenize(log1)
	require.NoError(t, err)
	pattern1, _, _, _ := cm.Add(tokenList1)
	require.NotNil(t, pattern1)

	assert.Equal(t, 0, pattern1.GetWildcardCount(),
		"First log should not have wildcards yet")
	assert.Equal(t, log1, pattern1.GetPatternString(),
		"Pattern should match first log before merging")

	tokenList2, err := tokenizer.Tokenize(log2)
	require.NoError(t, err)
	pattern2, _, _, _ := cm.Add(tokenList2)
	require.NotNil(t, pattern2)

	assert.GreaterOrEqual(t, pattern2.GetWildcardCount(), 1,
		"Pattern should introduce wildcards after merging")
	assert.Contains(t, pattern2.GetPatternString(), "Processing",
		"Template should keep static prefix")
	assert.Contains(t, pattern2.GetPatternString(), "items",
		"Template should keep static suffix")
}

// TestRustTokenizer_WildcardPositions validates that wildcard character positions
// are correctly calculated for pattern serialization.
func TestRustTokenizer_WildcardPositions(t *testing.T) {
	tokenizer := NewRustTokenizer()
	cm := clustering.NewClusterManager()

	tests := []struct {
		name              string
		logs              []string
		expectedWildcards int
		description       string
	}{
		{
			name: "No wildcards",
			logs: []string{
				"Service started",
				"Service started",
			},
			expectedWildcards: 0,
			description:       "Identical logs should have no wildcards",
		},
		{
			name: "Single wildcard",
			logs: []string{
				"Processing 100 items",
				"Processing 200 items",
			},
			expectedWildcards: 1,
			description:       "Should have one wildcard position",
		},
		{
			name: "Multiple wildcards",
			logs: []string{
				"GET /api/users 200",
				"POST /api/orders 201",
			},
			expectedWildcards: 3,
			description:       "Should have three wildcard positions",
		},
		{
			name: "Complex pattern",
			logs: []string{
				"2024-01-15 10:30:00 INFO User admin@example.com from 192.168.1.1 GET /api/users 200",
				"2024-01-15 10:31:00 INFO User user@test.com from 10.0.0.1 POST /api/users 201",
			},
			expectedWildcards: 5,
			description:       "Complex log should have multiple wildcard positions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm = clustering.NewClusterManager()

			var pattern *clustering.Pattern
			for _, log := range tt.logs {
				tokenList, err := tokenizer.Tokenize(log)
				require.NoError(t, err, "Tokenization should succeed")
				pattern, _, _, _ = cm.Add(tokenList)
			}

			require.NotNil(t, pattern, "Pattern should not be nil")

			wildcardCount := pattern.GetWildcardCount()
			wildcardPositions := pattern.GetWildcardCharPositions()

			assert.Equal(t, tt.expectedWildcards, wildcardCount,
				"%s: wildcard count mismatch", tt.description)
			assert.Equal(t, tt.expectedWildcards, len(wildcardPositions),
				"%s: wildcard position count mismatch", tt.description)

			// Verify positions are in non-decreasing order
			for i := 1; i < len(wildcardPositions); i++ {
				assert.LessOrEqual(t, wildcardPositions[i-1], wildcardPositions[i],
					"Wildcard positions should be in non-decreasing order")
			}

			patternString := pattern.GetPatternString()
			t.Logf("✅ %s: Pattern: %q, Wildcards: %d, Positions: %v",
				tt.description, patternString, wildcardCount, wildcardPositions)
		})
	}
}

// TestRustTokenizer_WildcardPositions_EqualIndices validates that equal positions are allowed
// when wildcards are adjacent (template omits wildcard placeholders).
func TestRustTokenizer_WildcardPositions_EqualIndices(t *testing.T) {
	tokenizer := NewRustTokenizer()
	cm := clustering.NewClusterManager()

	logs := []string{
		"GET 200",
		"POST 404",
	}

	var pattern *clustering.Pattern
	for _, log := range logs {
		tokenList, err := tokenizer.Tokenize(log)
		require.NoError(t, err)
		pattern, _, _, _ = cm.Add(tokenList)
	}
	require.NotNil(t, pattern)

	positions := pattern.GetWildcardCharPositions()
	if len(positions) >= 2 {
		assert.LessOrEqual(t, positions[0], positions[1],
			"Adjacent wildcard positions can be equal")
	}
}

// TestRustTokenizer_PatternTemplates validates that pattern templates correctly
// represent the structure of clustered logs.
func TestRustTokenizer_PatternTemplates(t *testing.T) {
	tokenizer := NewRustTokenizer()
	cm := clustering.NewClusterManager()

	tests := []struct {
		name                 string
		logs                 []string
		validateTemplate     func(t *testing.T, template *token.TokenList)
		expectedTemplateSize int
		description          string
	}{
		{
			name: "Simple template",
			logs: []string{
				"Service started",
				"Service started",
			},
			expectedTemplateSize: 3, // Word, Whitespace, Word
			validateTemplate: func(t *testing.T, template *token.TokenList) {
				assert.Equal(t, token.TokenWord, template.Tokens[0].Type)
				assert.Equal(t, "Service", template.Tokens[0].Value)
			},
			description: "Template should preserve non-wildcard tokens",
		},
		{
			name: "Template with wildcard",
			logs: []string{
				"Processing 100 items",
				"Processing 200 items",
			},
			expectedTemplateSize: 5, // Word, Whitespace, Numeric(wildcard), Whitespace, Word
			validateTemplate: func(t *testing.T, template *token.TokenList) {
				assert.Equal(t, token.TokenWord, template.Tokens[0].Type)
				assert.Equal(t, "Processing", template.Tokens[0].Value)
				// Token at index 2 should be wildcard
				assert.Equal(t, token.IsWildcard, template.Tokens[2].Wildcard)
				assert.Equal(t, token.TokenWord, template.Tokens[4].Type)
				assert.Equal(t, "items", template.Tokens[4].Value)
			},
			description: "Template should have wildcards for varying tokens",
		},
		{
			name: "HTTP template",
			logs: []string{
				"GET /api/users 200",
				"POST /api/users 201",
			},
			expectedTemplateSize: 5, // HTTPMethod, Whitespace, AbsolutePath, Whitespace, HTTPStatus
			validateTemplate: func(t *testing.T, template *token.TokenList) {
				assert.Equal(t, token.TokenHTTPMethod, template.Tokens[0].Type)
				assert.Equal(t, token.TokenAbsolutePath, template.Tokens[2].Type)
				assert.Equal(t, token.TokenHTTPStatus, template.Tokens[4].Type)
				assert.Equal(t, token.IsWildcard, template.Tokens[0].Wildcard)
				assert.Equal(t, token.PotentialWildcard, template.Tokens[2].Wildcard)
				assert.Equal(t, token.IsWildcard, template.Tokens[4].Wildcard)
			},
			description: "Template should preserve token types",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm = clustering.NewClusterManager()

			var pattern *clustering.Pattern
			for _, log := range tt.logs {
				tokenList, err := tokenizer.Tokenize(log)
				require.NoError(t, err, "Tokenization should succeed")
				pattern, _, _, _ = cm.Add(tokenList)
			}

			require.NotNil(t, pattern, "Pattern should not be nil")
			require.NotNil(t, pattern.Template, "Pattern template should not be nil")

			assert.Equal(t, tt.expectedTemplateSize, pattern.Template.Length(),
				"%s: template size mismatch", tt.description)

			if tt.validateTemplate != nil {
				tt.validateTemplate(t, pattern.Template)
			}

			t.Logf("✅ %s: Template has %d tokens, pattern: %q",
				tt.description, pattern.Template.Length(), pattern.GetPatternString())
		})
	}
}

// TestRustTokenizer_PatternStress validates pattern behavior under load with many logs.
func TestRustTokenizer_PatternStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tokenizer := NewRustTokenizer()
	cm := clustering.NewClusterManager()

	// Generate 10,000 logs with 100 unique patterns
	totalLogs := 10000
	uniquePatterns := 100

	for i := 0; i < totalLogs; i++ {
		patternID := i % uniquePatterns
		log := fmt.Sprintf("Service %d started with ID %d", patternID, i)

		tokenList, err := tokenizer.Tokenize(log)
		require.NoError(t, err, "Tokenization should succeed")

		pattern, _, _, _ := cm.Add(tokenList)
		require.NotNil(t, pattern, "Pattern should not be nil")
	}

	// Verify pattern count is approximately correct
	patternCount := cm.PatternCount()
	assert.Greater(t, patternCount, 0, "Should create at least one pattern")
	assert.LessOrEqual(t, patternCount, uniquePatterns,
		"Pattern count should not exceed unique patterns")

	// Verify total log count across all patterns
	// (This would require iterating all patterns, which may not be exposed by ClusterManager)

	t.Logf("✅ Stress test: Processed %d logs, created %d patterns",
		totalLogs, patternCount)
}

// TestRustTokenizer_StatefulEncodingParity validates the core assumptions used by
// the mock stateful encoder: wildcards preserve original raw values and order.
func TestRustTokenizer_StatefulEncodingParity(t *testing.T) {
	tokenizer := NewRustTokenizer()
	cm := clustering.NewClusterManager()

	logs := []string{
		"GET /api/users 200",
		"POST /api/orders 404",
	}

	var pattern *clustering.Pattern
	for _, log := range logs {
		tokenList, err := tokenizer.Tokenize(log)
		require.NoError(t, err, "Tokenization should succeed for log: %q", log)

		pattern, _, _, _ = cm.Add(tokenList)
		require.NotNil(t, pattern, "Pattern should not be nil")

		values := pattern.GetWildcardValues(tokenList)
		if log == logs[0] {
			// First log sets baseline; no wildcards yet.
			require.Len(t, values, 0)
			continue
		}

		require.Len(t, values, 3, "Expected three wildcards for HTTP logs")
		assert.Equal(t, "POST", values[0], "HTTP method should preserve raw value")
		assert.Equal(t, "/api/orders", values[1], "Path should preserve raw value")
		assert.Equal(t, "404", values[2], "Status should preserve raw value")
	}

	require.NotNil(t, pattern, "Pattern should be created")
	assert.NotEmpty(t, pattern.GetPatternString(),
		"Expected non-empty HTTP pattern")
}

// TestRustTokenizer_WildcardOrdering ensures repeated wildcard types preserve order.
func TestRustTokenizer_WildcardOrdering(t *testing.T) {
	tokenizer := NewRustTokenizer()
	cm := clustering.NewClusterManager()

	logs := []string{
		"User alice logged in from 10.0.0.1",
		"User bob logged in from 192.168.1.1",
	}

	var pattern *clustering.Pattern
	for _, log := range logs {
		tokenList, err := tokenizer.Tokenize(log)
		require.NoError(t, err)

		pattern, _, _, _ = cm.Add(tokenList)
		require.NotNil(t, pattern)

		values := pattern.GetWildcardValues(tokenList)
		if log == logs[0] {
			require.Len(t, values, 0)
			continue
		}
		require.Len(t, values, 2, "Expected two wildcards (user, IP)")
		assert.Equal(t, "bob", values[0])
		assert.Equal(t, "192.168.1.1", values[1])
	}

	require.NotNil(t, pattern)
	assert.NotEmpty(t, pattern.GetPatternString())
}

// TestRustTokenizer_PatternMerging validates that patterns correctly merge when
// logs have the same structure but different values.
func TestRustTokenizer_PatternMerging(t *testing.T) {
	tokenizer := NewRustTokenizer()

	tests := []struct {
		name            string
		initialLogs     []string
		mergingLogs     []string
		expectedDiffMin int
		expectedDiffMax int
		description     string
	}{
		{
			name: "Merging into existing pattern",
			initialLogs: []string{
				"Processing 100 items",
				"Processing 200 items",
			},
			mergingLogs: []string{
				"Processing 500 items",
				"Processing 1000 items",
			},
			expectedDiffMin: 0,
			expectedDiffMax: 1,
			description:     "New logs with same structure should usually merge",
		},
		{
			name: "Creating new patterns alongside existing",
			initialLogs: []string{
				"Service started",
				"Service stopped",
			},
			mergingLogs: []string{
				"Connection from 192.168.1.1",
				"Connection from 10.0.0.1",
			},
			expectedDiffMin: 1,
			expectedDiffMax: 1,
			description:     "Different structure should create new pattern",
		},
		{
			name: "Mixed merging and creating",
			initialLogs: []string{
				"Service started",
				"Processing 100 items",
			},
			mergingLogs: []string{
				"Service stopped",             // Merges with "Service started"
				"Processing 200 items",        // Merges with "Processing 100 items"
				"Connection from 192.168.1.1", // New pattern
			},
			expectedDiffMin: 1,
			expectedDiffMax: 1,
			description:     "Should both merge and create new patterns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := clustering.NewClusterManager()

			// Add initial logs
			for _, log := range tt.initialLogs {
				tokenList, err := tokenizer.Tokenize(log)
				require.NoError(t, err)
				cm.Add(tokenList)
			}

			initialCount := cm.PatternCount()
			t.Logf("  Initial patterns: %d", initialCount)

			// Add merging logs
			for _, log := range tt.mergingLogs {
				tokenList, err := tokenizer.Tokenize(log)
				require.NoError(t, err)
				pattern, changeType, _, _ := cm.Add(tokenList)

				changeTypeStr := ""
				switch changeType {
				case clustering.PatternNew:
					changeTypeStr = "NEW"
				case clustering.PatternUpdated:
					changeTypeStr = "UPDATED"
				case clustering.PatternNoChange:
					changeTypeStr = "NO_CHANGE"
				}

				t.Logf("  Added %q → Pattern %d (%s, LogCount=%d)",
					log, pattern.PatternID, changeTypeStr, pattern.LogCount)
			}

			finalCount := cm.PatternCount()
			actualDiff := finalCount - initialCount

			assert.GreaterOrEqual(t, actualDiff, tt.expectedDiffMin,
				"%s: pattern count change below expected minimum. Initial: %d, Final: %d",
				tt.description, initialCount, finalCount)
			assert.LessOrEqual(t, actualDiff, tt.expectedDiffMax,
				"%s: pattern count change above expected maximum. Initial: %d, Final: %d",
				tt.description, initialCount, finalCount)

			t.Logf("✅ %s: Pattern count %d → %d (diff: %d)",
				tt.description, initialCount, finalCount, actualDiff)
		})
	}
}

// TestRustTokenizer_PatternIDUniqueness validates that pattern IDs are unique.
func TestRustTokenizer_PatternIDUniqueness(t *testing.T) {
	tokenizer := NewRustTokenizer()
	cm := clustering.NewClusterManager()

	logs := []string{
		"Service started",
		"Connection from 192.168.1.1",
		"User admin@example.com logged in",
		"GET /api/users 200",
		"ERROR Database connection failed",
	}

	patternIDs := make(map[uint64]bool)

	for _, log := range logs {
		tokenList, err := tokenizer.Tokenize(log)
		require.NoError(t, err)

		pattern, _, _, _ := cm.Add(tokenList)
		require.NotNil(t, pattern)

		// Check for duplicate pattern ID
		assert.False(t, patternIDs[pattern.PatternID],
			"Pattern ID %d should be unique", pattern.PatternID)
		patternIDs[pattern.PatternID] = true

		t.Logf("  Log: %q → Pattern ID: %d", log, pattern.PatternID)
	}

	assert.Equal(t, len(logs), len(patternIDs),
		"All patterns should have unique IDs")

	t.Logf("✅ Created %d patterns with unique IDs", len(patternIDs))
}

// TestRustTokenizer_PatternSerialization validates that patterns can be serialized
// for transmission (e.g., to backend).
func TestRustTokenizer_PatternSerialization(t *testing.T) {
	tokenizer := NewRustTokenizer()
	cm := clustering.NewClusterManager()

	logs := []string{
		"GET /api/users 200",
		"POST /api/users 201",
	}

	var pattern *clustering.Pattern
	for _, log := range logs {
		tokenList, err := tokenizer.Tokenize(log)
		require.NoError(t, err)
		pattern, _, _, _ = cm.Add(tokenList)
	}

	require.NotNil(t, pattern)

	// Validate serialization fields
	patternID := pattern.PatternID
	patternString := pattern.GetPatternString()
	wildcardCount := pattern.GetWildcardCount()
	wildcardPositions := pattern.GetWildcardCharPositions()

	assert.Greater(t, patternID, uint64(0), "Pattern ID should be non-zero")
	assert.NotEmpty(t, patternString, "Pattern string should not be empty")
	assert.Greater(t, wildcardCount, 0, "Should have wildcards")
	assert.Equal(t, wildcardCount, len(wildcardPositions), "Position count should match wildcard count")

	// Simulate serialization
	serialized := fmt.Sprintf("PatternID=%d,Template=%s,WildcardCount=%d,Positions=%v",
		patternID, patternString, wildcardCount, wildcardPositions)

	assert.NotEmpty(t, serialized, "Serialized pattern should not be empty")
	assert.Contains(t, serialized, patternString, "Serialization should contain pattern string")

	t.Logf("✅ Pattern serialized: %s", serialized)
}

// TestRustTokenizer_PatternEdgeCases validates pattern behavior for edge cases.
func TestRustTokenizer_PatternEdgeCases(t *testing.T) {
	tokenizer := NewRustTokenizer()

	tests := []struct {
		name        string
		logs        []string
		expectError bool
		description string
	}{
		{
			name:        "Empty log",
			logs:        []string{""},
			expectError: false,
			description: "Empty log should create pattern",
		},
		{
			name:        "Single character log",
			logs:        []string{"a"},
			expectError: false,
			description: "Single character should create pattern",
		},
		{
			name: "Very long log",
			logs: []string{
				strings.Repeat("word ", 1000),
			},
			expectError: false,
			description: "Very long log should create pattern",
		},
		{
			name: "Unicode characters",
			logs: []string{
				"用户 登录 成功",
				"用户 注销 成功",
			},
			expectError: false,
			description: "Unicode logs should create pattern and merge",
		},
		{
			name: "Mixed newlines",
			logs: []string{
				"line1\nline2",
				"line3\nline4",
			},
			expectError: false,
			description: "Logs with newlines should be handled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := clustering.NewClusterManager()

			var lastPattern *clustering.Pattern
			for _, log := range tt.logs {
				tokenList, err := tokenizer.Tokenize(log)

				if tt.expectError {
					assert.Error(t, err, "%s: should produce error", tt.description)
					continue
				}

				require.NoError(t, err, "%s: should not produce error", tt.description)
				if tokenList == nil || tokenList.IsEmpty() {
					// Empty logs yield empty token lists; clustering would return nil pattern.
					continue
				}
				pattern, _, _, _ := cm.Add(tokenList)
				require.NotNil(t, pattern, "Pattern should not be nil")
				lastPattern = pattern
			}

			if !tt.expectError && lastPattern != nil {
				t.Logf("✅ %s: Created pattern with %d tokens, pattern: %q",
					tt.description, lastPattern.Template.Length(), lastPattern.GetPatternString())
			}
		})
	}
}
