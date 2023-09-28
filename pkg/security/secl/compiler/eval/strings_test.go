// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"testing"

	"golang.org/x/exp/slices"
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

		if err := values.Compile(StringCmpOpts{ScalarCaseInsensitive: true}); err != nil {
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
		matcher, err := NewStringMatcher(ScalarValueType, "test123", StringCmpOpts{ScalarCaseInsensitive: true})
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
		matcher, err := NewStringMatcher(PatternValueType, "http://TEst*", StringCmpOpts{PatternCaseInsensitive: true})
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
		matcher, err := NewStringMatcher(PatternValueType, "http://test123", StringCmpOpts{PatternCaseInsensitive: true})
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
		matcher, err := NewStringMatcher(GlobValueType, "/etc/TEst*", StringCmpOpts{GlobCaseInsensitive: true})
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
		matcher, err := NewStringMatcher(GlobValueType, "/etc/test123", StringCmpOpts{GlobCaseInsensitive: true})
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
		matcher, err := NewStringMatcher(RegexpValueType, "test.*", StringCmpOpts{RegexpCaseInsensitive: true})
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
