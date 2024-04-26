// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"slices"
	"testing"
)

func TestStringValues(t *testing.T) {
	t.Run("scalar-fast-path", func(t *testing.T) {
		var values StringValues
		values.AppendScalarValue("test123")

		if err := values.Compile(DefaultStringCmpOpts); err != nil {
			t.Error(err)
		}

		if !slices.Contains(values.scalarCache, "test123") {
			t.Error("expected cache key not found")
		}

		if len(values.stringMatchers) != 0 {
			t.Error("shouldn't have a string matcher")
		}
	})

	t.Run("scalar-matcher", func(t *testing.T) {
		var values StringValues
		values.AppendScalarValue("test123")

		if err := values.Compile(StringCmpOpts{CaseInsensitive: true}); err != nil {
			t.Error(err)
		}

		if slices.Contains(values.scalarCache, "test123") {
			t.Error("expected cache key found")
		}

		if len(values.stringMatchers) == 0 {
			t.Error("should have a string matcher")
		}
	})
}

func TestScalar(t *testing.T) {
	t.Run("sensitive-case", func(t *testing.T) {
		matcher, err := NewStringMatcher(ScalarValueType, "test123", DefaultStringCmpOpts)
		if err != nil {
			t.Error(err)
		}

		if !matcher.Matches("test123") {
			t.Error("should match")
		}

		if matcher.Matches("TEST123") {
			t.Error("shouldn't match")
		}
	})

	t.Run("insensitive-case", func(t *testing.T) {
		matcher, err := NewStringMatcher(ScalarValueType, "test123", StringCmpOpts{CaseInsensitive: true})
		if err != nil {
			t.Error(err)
		}

		if !matcher.Matches("test123") {
			t.Error("should match")
		}

		if !matcher.Matches("TEST123") {
			t.Error("should match")
		}
	})
}

func TestPattern(t *testing.T) {
	t.Run("sensitive-case", func(t *testing.T) {
		matcher, err := NewStringMatcher(PatternValueType, "http://test*", DefaultStringCmpOpts)
		if err != nil {
			t.Error(err)
		}

		if !matcher.Matches("http://test123") {
			t.Error("should match")
		}

		if matcher.Matches("http://TEST123") {
			t.Error("shouldn't match")
		}
	})

	t.Run("insensitive-case", func(t *testing.T) {
		matcher, err := NewStringMatcher(PatternValueType, "http://TEst*", StringCmpOpts{CaseInsensitive: true})
		if err != nil {
			t.Error(err)
		}

		if !matcher.Matches("http://test123") {
			t.Error("should match")
		}

		if !matcher.Matches("http://TEST123") {
			t.Error("should match")
		}
	})

	t.Run("sensitive-case-scalar", func(t *testing.T) {
		matcher, err := NewStringMatcher(PatternValueType, "http://test123", DefaultStringCmpOpts)
		if err != nil {
			t.Error(err)
		}

		if !matcher.Matches("http://test123") {
			t.Error("should match")
		}

		if matcher.Matches("http://TEST123") {
			t.Error("shouldn't match")
		}
	})

	t.Run("insensitive-case-scalar", func(t *testing.T) {
		matcher, err := NewStringMatcher(PatternValueType, "http://test123", StringCmpOpts{CaseInsensitive: true})
		if err != nil {
			t.Error(err)
		}

		if !matcher.Matches("http://test123") {
			t.Error("should match")
		}

		if !matcher.Matches("http://TEST123") {
			t.Error("should match")
		}
	})
}

func TestGlob(t *testing.T) {
	t.Run("sensitive-case", func(t *testing.T) {
		matcher, err := NewStringMatcher(GlobValueType, "/etc/test*", DefaultStringCmpOpts)
		if err != nil {
			t.Error(err)
		}

		if !matcher.Matches("/etc/test123") {
			t.Error("should match")
		}

		if matcher.Matches("/etc/TEST123") {
			t.Error("shouldn't match")
		}
	})

	t.Run("insensitive-case", func(t *testing.T) {
		matcher, err := NewStringMatcher(GlobValueType, "/etc/TEst*", StringCmpOpts{CaseInsensitive: true})
		if err != nil {
			t.Error(err)
		}

		if !matcher.Matches("/etc/test123") {
			t.Error("should match")
		}

		if !matcher.Matches("/etc/TEST123") {
			t.Error("should match")
		}
	})

	t.Run("sensitive-case-scalar", func(t *testing.T) {
		matcher, err := NewStringMatcher(GlobValueType, "/etc/test123", DefaultStringCmpOpts)
		if err != nil {
			t.Error(err)
		}

		if !matcher.Matches("/etc/test123") {
			t.Error("should match")
		}

		if matcher.Matches("/etc/TEST123") {
			t.Error("shouldn't match")
		}
	})

	t.Run("insensitive-case-scalar", func(t *testing.T) {
		matcher, err := NewStringMatcher(GlobValueType, "/etc/test123", StringCmpOpts{CaseInsensitive: true})
		if err != nil {
			t.Error(err)
		}

		if !matcher.Matches("/etc/test123") {
			t.Error("should match")
		}

		if !matcher.Matches("/etc/TEST123") {
			t.Error("should match")
		}
	})
}

func TestRegexp(t *testing.T) {
	t.Run("sensitive-case", func(t *testing.T) {
		matcher, err := NewStringMatcher(RegexpValueType, "test.*", DefaultStringCmpOpts)
		if err != nil {
			t.Error(err)
		}

		if !matcher.Matches("test123") {
			t.Error("should match")
		}

		if matcher.Matches("TEST123") {
			t.Error("shouldn't match")
		}
	})

	t.Run("insensitive-case", func(t *testing.T) {
		matcher, err := NewStringMatcher(RegexpValueType, "test.*", StringCmpOpts{CaseInsensitive: true})
		if err != nil {
			t.Error(err)
		}

		if !matcher.Matches("test123") {
			t.Error("should match")
		}

		if !matcher.Matches("TEST123") {
			t.Error("should match")
		}
	})

	t.Run("multiple-string-options", func(t *testing.T) {
		matcher, err := NewStringMatcher(RegexpValueType, ".*(restore|recovery|readme|instruction|how_to|ransom).*", StringCmpOpts{})
		if err != nil {
			t.Error(err)
		}

		if !matcher.Matches("123readme456") {
			t.Error("should match")
		}

		if matcher.Matches("TEST123") {
			t.Error("should not match")
		}

		reMatcher, ok := matcher.(*RegexpStringMatcher)
		if !ok {
			t.Error("should be a regex matcher")
		}

		if !slices.Equal([]string{"restore", "recovery", "readme", "instruction", "how_to", "ransom"}, reMatcher.stringOptionsOpt) {
			t.Error("should be an optimized string option re matcher")
		}
	})
}

func BenchmarkRegexpEvaluator(b *testing.B) {
	pattern := ".*(restore|recovery|readme|instruction|how_to|ransom).*"

	var matcher RegexpStringMatcher
	if err := matcher.Compile(pattern, false); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !matcher.Matches("123ransom456.txt") {
			b.Fatal("unexpected result")
		}
	}
}
