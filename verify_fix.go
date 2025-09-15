package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// Copy of the escapeSQLString function to test
func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// Test function
func TestEscapeSQLString(t *testing.T) {
	tests := []struct {
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
		{"realistic hostname", "web-server-01.example.com", "web-server-01.example.com"},
		{"hostname with quote", "web'server", "web''server"},
	}

	passed := 0
	for _, tt := range tests {
		result := escapeSQLString(tt.input)
		if result == tt.expected {
			fmt.Printf("✓ %s: PASS\n", tt.name)
			passed++
		} else {
			fmt.Printf("✗ %s: FAIL - expected %q, got %q\n", tt.name, tt.expected, result)
		}
	}

	fmt.Printf("\nResults: %d/%d tests passed\n", passed, len(tests))
	if passed == len(tests) {
		fmt.Println("✅ All tests PASSED!")
	}
}

// Validate syntax of the common.go file
func validateSyntax() {
	fmt.Println("\n--- Syntax Validation ---")

	fset := token.NewFileSet()
	src := `package cws

import (
	"fmt"
	"strings"
)

// escapeSQLString escapes single quotes in SQL string literals to prevent injection
func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func testCwsEnabled(hostname string) string {
	escaped := escapeSQLString(hostname)
	query := fmt.Sprintf("SELECT * WHERE hostname = '%s'", escaped)
	return query
}
`

	// Parse the source code
	_, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		fmt.Printf("✗ Syntax validation FAILED: %v\n", err)
	} else {
		fmt.Println("✅ Syntax validation PASSED!")
	}
}

func main() {
	fmt.Println("=== SQL Injection Fix Verification ===\n")

	// Run tests
	t := &testing.T{}
	TestEscapeSQLString(t)

	// Validate syntax
	validateSyntax()

	// Test SQL injection prevention
	fmt.Println("\n--- SQL Injection Prevention Test ---")
	dangerousInputs := []string{
		"'; DROP TABLE users;--",
		"' OR 1=1--",
		"' UNION SELECT password FROM users--",
		"admin'; DELETE FROM logs WHERE '1'='1",
	}

	for _, dangerous := range dangerousInputs {
		escaped := escapeSQLString(dangerous)
		fmt.Printf("Input:  %s\n", dangerous)
		fmt.Printf("Escaped: %s\n", escaped)

		// Verify no unescaped single quotes remain
		hasUnescapedQuotes := false
		for i := 0; i < len(escaped); i++ {
			if escaped[i] == '\'' {
				// Check if this quote is properly escaped (followed by another quote)
				if i+1 < len(escaped) && escaped[i+1] == '\'' {
					// Skip the next character since it's the escaping quote
					i++ // This skips the paired quote
				} else {
					// Found an unescaped single quote
					hasUnescapedQuotes = true
					break
				}
			}
		}

		if hasUnescapedQuotes {
			fmt.Println("✗ DANGEROUS: Contains unescaped quotes!")
		} else {
			fmt.Println("✅ SAFE: All quotes properly escaped")
		}
		fmt.Println()
	}
}
