// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"testing"
)

func TestPatternScanSegment(t *testing.T) {
	star, segment, _ := scanSegment("*test123")
	if !star || segment != "test123" {
		t.Errorf("expected segment not found: %v, %v", star, segment)
	}

	star, segment, _ = scanSegment("test123*")
	if star || segment != "test123" {
		t.Errorf("expected segment not found: %v, %v", star, segment)
	}

	star, segment, _ = scanSegment("*test123*")
	if !star || segment != "test123" {
		t.Errorf("expected segment not found: %v, %v", star, segment)
	}

	star, segment, _ = scanSegment("*test*123*")
	if !star || segment != "test" {
		t.Errorf("expected segment not found: %v, %v", star, segment)
	}

	star, segment, _ = scanSegment("test")
	if star || segment != "test" {
		t.Errorf("expected segment not found: %v, %v", star, segment)
	}

	star, segment, _ = scanSegment("**")
	if !star || segment != "" {
		t.Errorf("expected segment not found: %v, %v", star, segment)
	}
}

func TestPatternMatches(t *testing.T) {
	if !PatternMatches("*test123", "aaatest123") {
		t.Error("should match")
	}

	if PatternMatches("*test456", "aaatest123") {
		t.Error("shouldn't match")
	}

	if !PatternMatches("*", "test123") {
		t.Error("should match")
	}

	if !PatternMatches("test*", "test123") {
		t.Error("should match")
	}

	if !PatternMatches("t*123", "test123") {
		t.Error("should match")
	}

	if !PatternMatches("t*1*3", "test123") {
		t.Error("should match")
	}

	if !PatternMatches("*t*1*3", "atest123") {
		t.Error("should match")
	}

	if PatternMatches("*t*9*3", "atest123") {
		t.Error("shouldn't match")
	}
}
