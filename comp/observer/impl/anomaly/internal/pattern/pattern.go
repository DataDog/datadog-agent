// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package pattern

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly/internal/types"
)

// PatternDetector handles pattern detection in log messages
type PatternDetector struct {
	filePathRegex  *regexp.Regexp
	numberRegex    *regexp.Regexp
	uuidRegex      *regexp.Regexp
	ipRegex        *regexp.Regexp
	timestampRegex *regexp.Regexp
}

// NewPatternDetector creates a new pattern detector
func NewPatternDetector() *PatternDetector {
	return &PatternDetector{
		// Matches file paths like (pkg/process/runner.go:513)
		filePathRegex: regexp.MustCompile(`\([^)]*?/[^)]+\.[a-z]+:\d+[^)]*\)`),
		// Matches numbers (IPs, ports, IDs, etc.)
		numberRegex: regexp.MustCompile(`\b\d+\b`),
		// Matches UUIDs
		uuidRegex: regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`),
		// Matches IP addresses
		ipRegex: regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),
		// Matches timestamps at the beginning of messages
		timestampRegex: regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}[^\|]*\|`),
	}
}

// ExtractFilePath extracts the file path from a log message if present
// Returns the file path and true if found, empty string and false otherwise
func (pd *PatternDetector) ExtractFilePath(message string) (string, bool) {
	match := pd.filePathRegex.FindString(message)
	if match == "" {
		return "", false
	}
	// Remove parentheses and line numbers for grouping
	// Example: (pkg/process/runner.go:513 in readResponseStatuses) -> pkg/process/runner.go
	match = strings.Trim(match, "()")
	parts := strings.Split(match, ":")
	if len(parts) > 0 {
		return parts[0], true
	}
	return "", false
}

// NormalizeMessage normalizes a log message by replacing variable parts with placeholders
// This creates a pattern that can be used for grouping similar messages
func (pd *PatternDetector) NormalizeMessage(message string) string {
	normalized := message

	// Remove timestamp prefix
	normalized = pd.timestampRegex.ReplaceAllString(normalized, "")

	// Replace UUIDs with placeholder
	normalized = pd.uuidRegex.ReplaceAllString(normalized, "<UUID>")

	// Replace IP addresses with placeholder
	normalized = pd.ipRegex.ReplaceAllString(normalized, "<IP>")

	// Replace file paths with placeholder (keep for identification but normalize line numbers)
	normalized = pd.filePathRegex.ReplaceAllStringFunc(normalized, func(match string) string {
		// Keep the file path but replace line numbers
		return regexp.MustCompile(`:\d+`).ReplaceAllString(match, ":<LINE>")
	})

	// Replace standalone numbers with placeholder (but not in words like "http2")
	normalized = pd.numberRegex.ReplaceAllString(normalized, "<NUM>")

	// Collapse multiple spaces
	normalized = regexp.MustCompile(`\s+`).ReplaceAllString(normalized, " ")

	// Trim whitespace
	normalized = strings.TrimSpace(normalized)

	return normalized
}

// DetectPatternsByFilePath groups log errors by detected file paths
// This is Approach 1: Group by filename when detected
func (pd *PatternDetector) DetectPatternsByFilePath(logErrors []types.LogError) []types.LogPattern {
	// Map: filepath -> pattern data
	patternMap := make(map[string]*types.LogPattern)

	// Map: filepath -> list of full messages
	messagesByFile := make(map[string][]string)

	for _, logError := range logErrors {
		filepath, found := pd.ExtractFilePath(logError.Message)

		var key string
		if found {
			// Use the file path as the grouping key
			key = filepath
		} else {
			// If no file path, use the normalized message as key
			key = pd.NormalizeMessage(logError.Message)
		}

		// Update pattern count
		if pattern, exists := patternMap[key]; exists {
			pattern.Count += logError.Count
		} else {
			patternMap[key] = &types.LogPattern{
				Pattern:     key,
				Count:       logError.Count,
				Examples:    []string{},
				GroupingKey: key,
			}
		}

		// Store message examples (up to 3 per pattern)
		messagesByFile[key] = append(messagesByFile[key], logError.Message)
	}

	// Build final patterns with examples
	var patterns []types.LogPattern
	for key, pattern := range patternMap {
		// Add up to 3 example messages
		examples := messagesByFile[key]
		maxExamples := 3
		if len(examples) > maxExamples {
			pattern.Examples = examples[:maxExamples]
		} else {
			pattern.Examples = examples
		}
		patterns = append(patterns, *pattern)
	}

	// Sort by count (descending)
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Count > patterns[j].Count
	})

	return patterns
}

// DetectPatternsByNormalization groups log errors by normalized message patterns
// This is Approach 2: Use message normalization to detect similar patterns
func (pd *PatternDetector) DetectPatternsByNormalization(logErrors []types.LogError) []types.LogPattern {
	// Map: normalized pattern -> pattern data
	patternMap := make(map[string]*types.LogPattern)

	// Map: normalized pattern -> list of original messages
	messagesByPattern := make(map[string][]string)

	for _, logError := range logErrors {
		// Normalize the message to create a pattern
		normalizedPattern := pd.NormalizeMessage(logError.Message)

		// Update pattern count
		if pattern, exists := patternMap[normalizedPattern]; exists {
			pattern.Count += logError.Count
		} else {
			patternMap[normalizedPattern] = &types.LogPattern{
				Pattern:     normalizedPattern,
				Count:       logError.Count,
				Examples:    []string{},
				GroupingKey: normalizedPattern,
			}
		}

		// Store original message examples (up to 3 per pattern)
		messagesByPattern[normalizedPattern] = append(messagesByPattern[normalizedPattern], logError.Message)
	}

	// Build final patterns with examples
	var patterns []types.LogPattern
	for normalizedMsg, pattern := range patternMap {
		// Add up to 3 example messages
		examples := messagesByPattern[normalizedMsg]
		maxExamples := 3
		if len(examples) > maxExamples {
			pattern.Examples = examples[:maxExamples]
		} else {
			pattern.Examples = examples
		}
		patterns = append(patterns, *pattern)
	}

	// Sort by count (descending)
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Count > patterns[j].Count
	})

	return patterns
}

// PrintPatterns prints the detected patterns in a readable format
func PrintPatterns(patterns []types.LogPattern, title string) {
	fmt.Printf("\n%s\n", title)
	fmt.Printf("Found %d patterns:\n\n", len(patterns))

	for i, pattern := range patterns {
		fmt.Printf("Pattern %d [Count: %d]:\n", i+1, pattern.Count)
		fmt.Printf("  Pattern: %s\n", pattern.Pattern)
		if len(pattern.Examples) > 0 {
			fmt.Printf("  Examples:\n")
			for j, example := range pattern.Examples {
				// Truncate long messages
				displayExample := example
				if len(displayExample) > 150 {
					displayExample = displayExample[:150] + "..."
				}
				fmt.Printf("    %d) %s\n", j+1, displayExample)
			}
		}
		fmt.Println()
	}
}

// Additional Approach 3 (Not Implemented): Advanced Pattern Detection
//
// For more sophisticated pattern detection, you could implement:
//
// 1. **Levenshtein Distance / Edit Distance**:
//    - Measure similarity between log messages
//    - Group messages with distance below a threshold
//    - Libraries: github.com/agnivade/levenshtein
//    - Use case: Detecting typos or slight variations in error messages
//
// 2. **N-gram Based Similarity**:
//    - Break messages into character or word n-grams
//    - Calculate Jaccard similarity or cosine similarity
//    - Cluster similar messages together
//    - Use case: Finding structurally similar messages
//
// 3. **Machine Learning Clustering**:
//    - Use TF-IDF vectorization of log messages
//    - Apply clustering algorithms (K-means, DBSCAN, Hierarchical)
//    - Libraries: gonum.org/v1/gonum for matrix operations
//    - Use case: Automatic discovery of unknown patterns
//
// 4. **Regular Expression Generation**:
//    - Learn common patterns from examples
//    - Generate regex patterns that match similar messages
//    - Libraries: Could use github.com/dlclark/regexp2 for advanced features
//    - Use case: Creating reusable patterns for monitoring
//
// 5. **Log Template Mining**:
//    - Algorithms like Drain, Spell, or LenMa
//    - Automatically extract log templates from raw logs
//    - Research papers: "Drain: An Online Log Parsing Approach with Fixed Depth Tree"
//    - Use case: Production-grade log pattern extraction
//
// 6. **Semantic Similarity**:
//    - Use embeddings (word2vec, BERT) to capture semantic meaning
//    - Group logs with similar semantic content even if words differ
//    - Would require integration with ML models
//    - Use case: Understanding intent behind error messages
//
// Implementation Considerations:
// - Performance: Pattern detection on large log volumes needs optimization
// - Memory: Caching patterns and using streaming algorithms for large datasets
// - Accuracy: Balancing false positives (too broad) vs false negatives (too specific)
// - Configuration: Allowing users to tune similarity thresholds and grouping strategies
//
// Recommended Libraries for Go:
// - github.com/agnivade/levenshtein - Edit distance calculations
// - gonum.org/v1/gonum - Numerical computing and clustering
// - github.com/pemistahl/lingua-go - Language detection for multi-language logs
// - github.com/james-bowman/nlp - NLP and text vectorization in Go
