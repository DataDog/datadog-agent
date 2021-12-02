// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"regexp"

	"github.com/pkg/errors"
)

// StringValues describes a set of string values, either regex or scalar
type StringValues struct {
	scalars []string
	regexps []*regexp.Regexp

	// caches
	scalarCache map[string]bool
	fieldValues []FieldValue
}

// AppendFieldValue append a FieldValue
func (s *StringValues) AppendFieldValue(value FieldValue) error {
	if s.scalarCache == nil {
		s.scalarCache = make(map[string]bool)
	}

	switch value.Type {
	case PatternValueType, RegexpValueType:
		if err := value.Compile(); err != nil {
			return err
		}
		s.regexps = append(s.regexps, value.Regexp)
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

// GetRegexValues return the regex values
func (s *StringValues) GetRegexValues() []*regexp.Regexp {
	return s.regexps
}

// SetFieldValues apply field values
func (s *StringValues) SetFieldValues(values ...FieldValue) error {
	// reset internal caches
	s.regexps = []*regexp.Regexp{}
	s.scalarCache = nil

	for _, value := range values {
		if err := s.AppendFieldValue(value); err != nil {
			return err
		}
	}

	return nil
}

// AppendValue append a string value
func (s *StringValues) AppendScalarValue(value string) {
	if s.scalarCache == nil {
		s.scalarCache = make(map[string]bool)
	}

	s.scalars = append(s.scalars, value)
	s.scalarCache[value] = true
	s.fieldValues = append(s.fieldValues, FieldValue{Value: value, Type: ScalarValueType})
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

// Match returns whether the value matches the string values
func (s *StringValues) Match(value string) bool {
	if s.scalarCache != nil && s.scalarCache[value] {
		return true
	}
	for _, re := range s.regexps {
		if re.MatchString(value) {
			return true
		}
	}

	return false
}
