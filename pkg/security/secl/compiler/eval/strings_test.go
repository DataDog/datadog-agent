// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"testing"
)

func TestStringValues(t *testing.T) {
	t.Run("scalar-fast-path", func(t *testing.T) {
		var values StringValues
		values.AppendScalarValue("test123")

		if err := values.Compile(DefaultStringMatcherOpts); err != nil {
			t.Error(err)
		}

		if !values.scalarCache["test123"] {
			t.Error("expected cache key not found")
		}

		if len(values.stringMatchers) != 0 {
			t.Error("shouldn't have a string matcher")
		}
	})

	t.Run("scalar-matcher", func(t *testing.T) {
		var values StringValues
		values.AppendScalarValue("test123")

		if err := values.Compile(StringMatcherOpts{CaseInsensitive: true}); err != nil {
			t.Error(err)
		}

		if values.scalarCache["test123"] {
			t.Error("expected cache key found")
		}

		if len(values.stringMatchers) == 0 {
			t.Error("should have a string matcher")
		}
	})
}

func TestScalar(t *testing.T) {
	t.Run("sensitive-case", func(t *testing.T) {
		matcher, err := NewStringMatcher(ScalarValueType, "test123", DefaultStringMatcherOpts)
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

	t.Run("insensitve-case", func(t *testing.T) {
		matcher, err := NewStringMatcher(ScalarValueType, "test123", StringMatcherOpts{CaseInsensitive: true})
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

func TestGlob(t *testing.T) {
	t.Run("sensitive-case", func(t *testing.T) {
		matcher, err := NewStringMatcher(PatternValueType, "test*", DefaultStringMatcherOpts)
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

	t.Run("insensitve-case", func(t *testing.T) {
		matcher, err := NewStringMatcher(PatternValueType, "TEst*", StringMatcherOpts{CaseInsensitive: true})
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

	t.Run("sensitive-case-scalar", func(t *testing.T) {
		matcher, err := NewStringMatcher(PatternValueType, "test123", DefaultStringMatcherOpts)
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

	t.Run("insensitve-case-scalar", func(t *testing.T) {
		matcher, err := NewStringMatcher(PatternValueType, "test123", StringMatcherOpts{CaseInsensitive: true})
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

func TestRegexp(t *testing.T) {
	t.Run("sensitive-case", func(t *testing.T) {
		matcher, err := NewStringMatcher(RegexpValueType, "test.*", DefaultStringMatcherOpts)
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

	t.Run("insensitve-case", func(t *testing.T) {
		matcher, err := NewStringMatcher(RegexpValueType, "test.*", StringMatcherOpts{CaseInsensitive: true})
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
