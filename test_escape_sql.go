package main

import (
	"fmt"
	"strings"
)

// escapeSQLString escapes single quotes in SQL string literals to prevent injection
func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func main() {
	// Test cases for the escapeSQLString function
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"no special chars", "hostname", "hostname"},
		{"single quote", "host'name", "host''name"},
		{"multiple quotes", "host'name'test", "host''name''test"},
		{"sql injection basic", "'; DROP TABLE users;--", "''; DROP TABLE users;--"},
		{"sql injection complex", "admin' OR '1'='1", "admin'' OR ''1''=''1"},
		{"empty string", "", ""},
		{"only quotes", "'''", "''''''"},
	}

	fmt.Println("Testing escapeSQLString function...")
	allPassed := true

	for _, tc := range testCases {
		result := escapeSQLString(tc.input)
		if result == tc.expected {
			fmt.Printf("✓ %s: PASS\n", tc.name)
		} else {
			fmt.Printf("✗ %s: FAIL - expected %q, got %q\n", tc.name, tc.expected, result)
			allPassed = false
		}
	}

	if allPassed {
		fmt.Println("\n✅ All tests passed! The escapeSQLString function correctly prevents SQL injection.")
	} else {
		fmt.Println("\n❌ Some tests failed!")
	}
}
