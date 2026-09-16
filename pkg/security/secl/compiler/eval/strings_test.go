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

		if !slices.Contains(values.scalars, "test123") {
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

		if slices.Contains(values.scalars, "test123") {
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

func TestCaptureStringMatcher(t *testing.T) {
	t.Run("ssm-command-id", func(t *testing.T) {
		var matcher CaptureStringMatcher
		if err := matcher.Compile("/orchestration/([^/]+)/"); err != nil {
			t.Fatal(err)
		}

		value, found := matcher.Capture("/var/lib/amazon/ssm/i-0abc/document/orchestration/a1b2c3d4/awsrunShellScript")
		if !found {
			t.Fatal("should have captured")
		}

		if value != "a1b2c3d4" {
			t.Errorf("should have captured the command id, got `%s`", value)
		}
	})

	t.Run("imds-role", func(t *testing.T) {
		var matcher CaptureStringMatcher
		if err := matcher.Compile("/security-credentials/([^/?]+)"); err != nil {
			t.Fatal(err)
		}

		value, found := matcher.Capture("/latest/meta-data/iam/security-credentials/my-role?x=1")
		if !found {
			t.Fatal("should have captured")
		}

		if value != "my-role" {
			t.Errorf("should have captured the role name, got `%s`", value)
		}
	})

	t.Run("no-match", func(t *testing.T) {
		var matcher CaptureStringMatcher
		if err := matcher.Compile("/orchestration/([^/]+)/"); err != nil {
			t.Fatal(err)
		}

		if value, found := matcher.Capture("/etc/passwd"); found {
			t.Errorf("shouldn't have captured, got `%s`", value)
		}
	})

	t.Run("no-capture-group", func(t *testing.T) {
		var matcher CaptureStringMatcher
		if err := matcher.Compile("/orchestration/[^/]+/"); err == nil {
			t.Error("should have failed to compile a pattern without a capture group")
		}
	})

	t.Run("non-capturing-group-only", func(t *testing.T) {
		var matcher CaptureStringMatcher
		if err := matcher.Compile("/orchestration/(?:[^/]+)/"); err == nil {
			t.Error("should have failed to compile a pattern with only a non-capturing group")
		}
	})

	t.Run("malformed-pattern", func(t *testing.T) {
		var matcher CaptureStringMatcher
		if err := matcher.Compile("/orchestration/([^/]+"); err == nil {
			t.Error("should have failed to compile a malformed pattern")
		}
	})

	t.Run("non-participating-group", func(t *testing.T) {
		var matcher CaptureStringMatcher
		if err := matcher.Compile("abc(x)?"); err != nil {
			t.Fatal(err)
		}

		// the pattern matches, but the optional first group took no part in it
		if value, found := matcher.Capture("abc"); found {
			t.Errorf("shouldn't have captured, got `%s`", value)
		}
	})

	t.Run("first-group-only", func(t *testing.T) {
		var matcher CaptureStringMatcher
		if err := matcher.Compile("/ecs/([0-9a-f-]+)/([0-9a-f-]+)"); err != nil {
			t.Fatal(err)
		}

		value, found := matcher.Capture("/ecs/a1b2c3d4-1111/e5f6a7b8-2222")
		if !found {
			t.Fatal("should have captured")
		}

		if value != "a1b2c3d4-1111" {
			t.Errorf("should have captured the first group only, got `%s`", value)
		}
	})

	t.Run("big-or-pattern", func(t *testing.T) {
		// RegexpStringMatcher takes a fast path for this shape and leaves its compiled
		// regexp nil, which is why capture cannot reuse it. Make sure we still extract.
		var matcher CaptureStringMatcher
		if err := matcher.Compile(".*(restore|recovery|ransom).*"); err != nil {
			t.Fatal(err)
		}

		value, found := matcher.Capture("123ransom456.txt")
		if !found {
			t.Fatal("should have captured")
		}

		if value != "ransom" {
			t.Errorf("should have captured the alternation, got `%s`", value)
		}
	})
}

func BenchmarkRegexpEvaluator(b *testing.B) {
	b.Run("with stars", func(b *testing.B) {
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
	})

	b.Run("without stars", func(b *testing.B) {
		pattern := "(restore|recovery|readme|instruction|how_to|ransom)"

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
	})
}
