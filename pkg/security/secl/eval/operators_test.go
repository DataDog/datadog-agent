package eval

import (
	"testing"
)

func TestPatternValue(t *testing.T) {
	re, err := patternToRegexp("^$[]{}+?/etc/?+*.conf")
	if err != nil {
		t.Fatal(err)
	}

	if re.String() != "^\\^\\$\\[\\]\\{\\}\\+\\?/etc/\\?\\+.*\\.conf$" {
		t.Fatalf("expected regexp not found: %s", re.String())
	}

	if _, err = patternToRegexp("*/passwd"); err == nil {
		t.Fatal("only suffix wildcard are accepted")
	}

	if _, err = patternToRegexp("/etc/*/passwd"); err == nil {
		t.Fatal("only suffix wildcard are accepted")
	}
}
