// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package patterns provides a simple demo of pattern extraction
package main

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/automaton"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

func main() {
	fmt.Println("=== Log Pattern Extraction Demo ===")

	// Step 1: Setup
	runBasicDemo()

	// Step 2: Advanced features
	runAdvancedDemo()

	fmt.Println("=== Demo Complete ===")
}

func runBasicDemo() {
	fmt.Println("1. BASIC PATTERN EXTRACTION")
	fmt.Println("   Processing HTTP requests to find patterns...")

	clusterManager := clustering.NewClusterManager()

	// Simple HTTP logs
	httpLogs := []string{
		"GET /api/users 200",
		"POST /api/users 201",
		"PUT /api/users 200",
		"GET /api/orders 200",
		"DELETE /api/users 204",
	}

	// Process logs and show tokenization
	for i, logMsg := range httpLogs {
		fmt.Printf("   Log %d: %s\n", i+1, logMsg)

		// Tokenize and show breakdown
		tokenList := automaton.TokenizeString(logMsg)
		fmt.Printf("   → Tokens: %s\n", formatTokens(tokenList))

		// Add to clustering
		cluster := clusterManager.Add(tokenList)
		fmt.Printf("   → Cluster size: %d\n\n", cluster.Size())
	}

	// Show discovered patterns
	showPatterns(clusterManager, "HTTP API Requests")
}

func runAdvancedDemo() {
	fmt.Println("2. ADVANCED TOKENIZATION")
	fmt.Println("   Showing specialized token detection...")

	clusterManager := clustering.NewClusterManager()

	// Advanced logs with different data types
	advancedLogs := []string{
		"ERROR Database connection to 192.168.1.100 failed",
		"ERROR Database connection to 192.168.1.101 failed",
		"ERROR Database connection to 192.168.1.102 failed",
		"INFO User admin@company.com logged in at 2024-01-15",
		"INFO User john@company.com logged in at 2024-01-16",
		"INFO User jane@company.com logged in at 2024-01-17",
	}

	for i, logMsg := range advancedLogs {
		fmt.Printf("   Log %d: %s\n", i+1, logMsg)

		tokenList := automaton.TokenizeString(logMsg)
		fmt.Printf("   → Specialized tokens: %s\n", formatSpecializedTokens(tokenList))

		cluster := clusterManager.Add(tokenList)
		fmt.Printf("   → Cluster size: %d\n\n", cluster.Size())
	}

	showPatterns(clusterManager, "Advanced Tokenization")
}

func formatTokens(tokenList *token.TokenList) string {
	if tokenList.IsEmpty() {
		return "none"
	}

	var parts []string
	for _, tok := range tokenList.Tokens {
		parts = append(parts, fmt.Sprintf("%s", tok.Value))
	}
	return strings.Join(parts, " | ")
}

func formatSpecializedTokens(tokenList *token.TokenList) string {
	if tokenList.IsEmpty() {
		return "none"
	}

	var parts []string
	for _, tok := range tokenList.Tokens {
		if tok.Type.String() != "Word" && tok.Type.String() != "Whitespace" {
			parts = append(parts, fmt.Sprintf("%s(%s)", tok.Type, tok.Value))
		}
	}

	if len(parts) == 0 {
		return "no specialized tokens"
	}
	return strings.Join(parts, ", ")
}

func showPatterns(clusterManager *clustering.ClusterManager, title string) {
	fmt.Printf("   PATTERNS DISCOVERED in %s:\n", title)

	allClusters := clusterManager.GetAllClusters()
	patternCount := 0

	for _, cluster := range allClusters {
		if cluster.Size() >= 3 { // Lower threshold for demo
			patternStr := cluster.GetPatternString()
			if patternStr != "" {
				patternCount++
				fmt.Printf("   → Pattern %d: %s (found %d times)\n",
					patternCount, patternStr, cluster.Size())
			}
		}
	}

	if patternCount == 0 {
		fmt.Printf("   → No patterns found (need at least 3 similar messages)\n")
	}

	// Show stats
	stats := clusterManager.GetStats()
	fmt.Printf("   → Stats: %d messages processed, %d clusters created\n\n",
		stats.TotalTokenLists, stats.TotalClusters)
}
