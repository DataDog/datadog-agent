// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/exp/slices"
)

// StringCmpOpts defines options to apply during string comparison
type StringCmpOpts struct {
	ScalarCaseInsensitive  bool
	PatternCaseInsensitive bool
	GlobCaseInsensitive    bool
	RegexpCaseInsensitive  bool
}

// DefaultStringCmpOpts defines the default comparison options
var DefaultStringCmpOpts = StringCmpOpts{}

// StringValues describes a set of string values, either regex or scalar
type StringValues struct {
	scalars        []string
	stringMatchers []StringMatcher

	// caches
	scalarCache []string
	fieldValues []FieldValue
}

// GetFieldValues return the list of FieldValue stored in the StringValues
func (s *StringValues) GetFieldValues() []FieldValue {
	return s.fieldValues
}

// AppendFieldValue append a FieldValue
func (s *StringValues) AppendFieldValue(value FieldValue) {
	if slices.Contains(s.fieldValues, value) {
		return
	}

	if value.Type == ScalarValueType {
		s.scalars = append(s.scalars, value.Value.(string))
	}

	s.fieldValues = append(s.fieldValues, value)
}

// Compile all the values
func (s *StringValues) Compile(opts StringCmpOpts) error {
	for _, value := range s.fieldValues {
		// fast path for scalar value without specific comparison behavior
		if opts == DefaultStringCmpOpts && value.Type == ScalarValueType {
			str := value.Value.(string)
			s.scalars = append(s.scalars, str)
			s.scalarCache = append(s.scalarCache, str)
		} else {
			str, ok := value.Value.(string)
			if !ok {
				return fmt.Errorf("invalid field value `%v`", value.Value)
			}

			matcher, err := NewStringMatcher(value.Type, str, opts)
			if err != nil {
				return err
			}
			s.stringMatchers = append(s.stringMatchers, matcher)
		}
	}

	return nil
}

// GetScalarValues return the scalar values
func (s *StringValues) GetScalarValues() []string {
	return s.scalars
}

// GetStringMatchers return the pattern matchers
func (s *StringValues) GetStringMatchers() []StringMatcher {
	return s.stringMatchers
}

// SetFieldValues apply field values
func (s *StringValues) SetFieldValues(values ...FieldValue) error {
	// reset internal caches
	s.stringMatchers = s.stringMatchers[:0]
	s.scalarCache = nil

	for _, value := range values {
		s.AppendFieldValue(value)
	}

	return nil
}

// AppendScalarValue append a scalar string value
func (s *StringValues) AppendScalarValue(value string) {
	s.AppendFieldValue(FieldValue{Value: value, Type: ScalarValueType})
}

// Matches returns whether the value matches the string values
func (s *StringValues) Matches(value string) bool {
	if slices.Contains(s.scalarCache, value) {
		return true
	}
	for _, pm := range s.stringMatchers {
		if pm.Matches(value) {
			return true
		}
	}

	return false
}

// StringMatcher defines a pattern matcher
type StringMatcher interface {
	Compile(pattern string, caseInsensitive bool) error
	Matches(value string) bool
}

// RegexpStringMatcher defines a regular expression pattern matcher
type RegexpStringMatcher struct {
	re *regexp.Regexp
}

// Compile a regular expression based pattern
func (r *RegexpStringMatcher) Compile(pattern string, caseInsensitive bool) error {
	if caseInsensitive {
		pattern = "(?i)" + pattern
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	r.re = re

	return nil
}

// Matches returns whether the value matches
func (r *RegexpStringMatcher) Matches(value string) bool {
	return r.re.MatchString(value)
}

// GlobStringMatcher defines a glob pattern matcher
type GlobStringMatcher struct {
	glob *Glob
}

// Compile a simple pattern
func (g *GlobStringMatcher) Compile(pattern string, caseInsensitive bool) error {
	if g.glob != nil {
		return nil
	}

	glob, err := NewGlob(pattern, caseInsensitive)
	if err != nil {
		return err
	}
	g.glob = glob

	return nil
}

// Matches returns whether the value matches
func (g *GlobStringMatcher) Matches(value string) bool {
	return g.glob.Matches(value)
}

// Contains returns whether the pattern contains the value
func (g *GlobStringMatcher) Contains(value string) bool {
	return g.glob.Contains(value)
}

// PatternStringMatcher defines a pattern matcher
type PatternStringMatcher struct {
	pattern         string
	caseInsensitive bool
}

// Compile a simple pattern
func (p *PatternStringMatcher) Compile(pattern string, caseInsensitive bool) error {
	// ** are not allowed in normal patterns
	if strings.Contains(pattern, "**") {
		return fmt.Errorf("`**` is not allowed in patterns")
	}

	p.pattern = pattern
	p.caseInsensitive = caseInsensitive
	return nil
}

// Matches returns whether the value matches
func (p *PatternStringMatcher) Matches(value string) bool {
	return PatternMatches(p.pattern, value, p.caseInsensitive)
}

// ScalarStringMatcher defines a scalar matcher
type ScalarStringMatcher struct {
	value           string
	caseInsensitive bool
}

// Compile a simple pattern
func (s *ScalarStringMatcher) Compile(pattern string, caseInsensitive bool) error {
	s.value = pattern
	s.caseInsensitive = caseInsensitive
	return nil
}

// Matches returns whether the value matches
func (s *ScalarStringMatcher) Matches(value string) bool {
	if s.caseInsensitive {
		return strings.EqualFold(s.value, value)
	}
	return s.value == value
}

// NewStringMatcher returns a new string matcher
func NewStringMatcher(kind FieldValueType, pattern string, opts StringCmpOpts) (StringMatcher, error) {
	switch kind {
	case PatternValueType:
		var matcher PatternStringMatcher
		if err := matcher.Compile(pattern, opts.PatternCaseInsensitive); err != nil {
			return nil, fmt.Errorf("invalid pattern `%s`: %s", pattern, err)
		}
		return &matcher, nil
	case GlobValueType:
		var matcher GlobStringMatcher
		if err := matcher.Compile(pattern, opts.GlobCaseInsensitive); err != nil {
			return nil, fmt.Errorf("invalid glob `%s`: %s", pattern, err)
		}
		return &matcher, nil
	case RegexpValueType:
		var matcher RegexpStringMatcher
		if err := matcher.Compile(pattern, opts.RegexpCaseInsensitive); err != nil {
			return nil, fmt.Errorf("invalid regexp `%s`: %s", pattern, err)
		}
		return &matcher, nil
	case ScalarValueType:
		var matcher ScalarStringMatcher
		if err := matcher.Compile(pattern, opts.ScalarCaseInsensitive); err != nil {
			return nil, fmt.Errorf("invalid regexp `%s`: %s", pattern, err)
		}
		return &matcher, nil
	}

	return nil, errors.New("unknown type")
}
