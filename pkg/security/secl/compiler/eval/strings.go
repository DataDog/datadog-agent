// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"fmt"
	"regexp"

	"github.com/pkg/errors"
)

// StringValues describes a set of string values, either regex or scalar
type StringValues struct {
	scalars        []string
	stringMatchers []StringMatcher

	// caches, ensure that the Clone method handle all the caches
	scalarCache map[string]bool
	fieldValues []FieldValue
	exists      map[interface{}]bool
}

// Clone returns a copy of the StringValues object
func (s *StringValues) Clone() *StringValues {
	var n StringValues

	for _, value := range s.fieldValues {
		_ = n.AppendFieldValue(value)
	}

	return &n
}

// AppendFieldValue append a FieldValue
func (s *StringValues) AppendFieldValue(value FieldValue) error {
	if s.scalarCache == nil {
		s.scalarCache = make(map[string]bool)
	}

	if s.exists[value.Value] {
		return nil
	}
	if s.exists == nil {
		s.exists = make(map[interface{}]bool)
	}
	s.exists[value.Value] = true

	switch value.Type {
	case PatternValueType, RegexpValueType:
		str, ok := value.Value.(string)
		if !ok {
			return fmt.Errorf("invalid pattern `%v`", value)
		}

		matcher, err := NewStringMatcher(value.Type, str)
		if err != nil {
			return err
		}
		s.stringMatchers = append(s.stringMatchers, matcher)
	default:
		str := value.Value.(string)
		s.scalars = append(s.scalars, str)
		s.scalarCache[str] = true
	}
	s.fieldValues = append(s.fieldValues, value)

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
	s.exists = nil

	for _, value := range values {
		if err := s.AppendFieldValue(value); err != nil {
			return err
		}
	}

	return nil
}

// GetFieldValues returns FieldValues
func (s *StringValues) GetFieldValues() []FieldValue {
	return s.fieldValues
}

// AppendScalarValue append a scalar string value
func (s *StringValues) AppendScalarValue(value string) *FieldValue {
	if s.scalarCache == nil {
		s.scalarCache = make(map[string]bool)
	}

	if s.exists[value] {
		return nil
	}
	if s.exists == nil {
		s.exists = make(map[interface{}]bool)
	}
	s.exists[value] = true
	s.scalars = append(s.scalars, value)
	s.scalarCache[value] = true

	fieldValue := FieldValue{Value: value, Type: ScalarValueType}
	s.fieldValues = append(s.fieldValues, fieldValue)

	return &fieldValue
}

// AppendStringEvaluator append a string evalutator
func (s *StringValues) AppendStringEvaluator(evaluator *StringEvaluator) error {
	if evaluator.EvalFnc == nil {
		return errors.New("only scalar evaluator are supported")
	}

	return s.AppendFieldValue(FieldValue{
		Value: evaluator.Value,
		Type:  evaluator.ValueType,
	})
}

// Matches returns whether the value matches the string values
func (s *StringValues) Matches(value string) bool {
	if s.scalarCache != nil && s.scalarCache[value] {
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
	Compile(pattern string) error
	Matches(value string) bool
}

// RegexpStringMatcher defines a regular expression pattern matcher
type RegexpStringMatcher struct {
	re *regexp.Regexp
}

// Compile a regular expression based pattern
func (r *RegexpStringMatcher) Compile(pattern string) error {
	if r.re != nil {
		return nil
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

	pattern string
}

// Compile a simple pattern
func (g *GlobStringMatcher) Compile(pattern string) error {
	if g.glob != nil {
		return nil
	}

	glob, err := NewGlob(pattern)
	if err != nil {
		return err
	}
	g.glob = glob
	g.pattern = pattern

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

// NewStringMatcher returns a new string matcher
func NewStringMatcher(kind FieldValueType, pattern string) (StringMatcher, error) {
	switch kind {
	case PatternValueType:
		var matcher GlobStringMatcher
		if err := matcher.Compile(pattern); err != nil {
			return nil, fmt.Errorf("invalid pattern `%s`: %s", pattern, err)
		}
		return &matcher, nil
	case RegexpValueType:
		var matcher RegexpStringMatcher
		if err := matcher.Compile(pattern); err != nil {
			return nil, fmt.Errorf("invalid regexp `%s`: %s", pattern, err)
		}
		return &matcher, nil
	}

	return nil, errors.New("unknown type")
}
