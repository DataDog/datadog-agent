// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"testing"
)

func TestPatternNextSegment(t *testing.T) {
	star, segment, _ := nextSegment("*test123")
	if !star || segment != "test123" {
		t.Errorf("expected segment not found: %v, %v", star, segment)
	}

	star, segment, _ = nextSegment("test123*")
	if star || segment != "test123" {
		t.Errorf("expected segment not found: %v, %v", star, segment)
	}

	star, segment, _ = nextSegment("*test123*")
	if !star || segment != "test123" {
		t.Errorf("expected segment not found: %v, %v", star, segment)
	}

	star, segment, _ = nextSegment("*test*123*")
	if !star || segment != "test" {
		t.Errorf("expected segment not found: %v, %v", star, segment)
	}

	star, segment, _ = nextSegment("test")
	if star || segment != "test" {
		t.Errorf("expected segment not found: %v, %v", star, segment)
	}

	star, segment, _ = nextSegment("**")
	if !star || segment != "" {
		t.Errorf("expected segment not found: %v, %v", star, segment)
	}
}

func TestPatternMatches(t *testing.T) {
	t.Run("sensitive-case", func(t *testing.T) {
		if !PatternMatches("*abc*", "/abc/", false) {
			t.Error("should match")
		}

		if !PatternMatches("*test123", "aaatest123", false) {
			t.Error("should match")
		}

		if PatternMatches("*test456", "aaatest123", false) {
			t.Error("shouldn't match")
		}

		if !PatternMatches("*", "test123", false) {
			t.Error("should match")
		}

		if !PatternMatches("test*", "test123", false) {
			t.Error("should match")
		}

		if !PatternMatches("t*123", "test123", false) {
			t.Error("should match")
		}

		if !PatternMatches("t*1*3", "test123", false) {
			t.Error("should match")
		}

		if !PatternMatches("*t*1*3", "atest123", false) {
			t.Error("should match")
		}

		if PatternMatches("*t*9*3", "atest123", false) {
			t.Error("shouldn't match")
		}
	})

	t.Run("insensitive-case", func(t *testing.T) {
		if !PatternMatches("*TEST123", "aaatest123", true) {
			t.Error("should match")
		}

		if PatternMatches("*TEST456", "aaatest123", true) {
			t.Error("shouldn't match")
		}

		if !PatternMatches("test*", "TEST123", true) {
			t.Error("should match")
		}

		if !PatternMatches("T*123", "test123", true) {
			t.Error("should match")
		}

		if !PatternMatches("T*t123", "tEsT123", true) {
			t.Error("should match")
		}
	})
}
