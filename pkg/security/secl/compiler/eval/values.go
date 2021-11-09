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
	values []string

	fieldValues []FieldValue
	scalars     map[string]bool
	regexps     []*regexp.Regexp
}

// AppendFieldValue append a FieldValue
func (s *StringValues) AppendFieldValue(value FieldValue) error {
	if s.scalars == nil {
		s.scalars = make(map[string]bool)
	}

	switch value.Type {
	case PatternValueType:
		if err := value.Compile(); err != nil {
			return err
		}

		s.regexps = append(s.regexps, value.Regexp)
		s.fieldValues = append(s.fieldValues, value)
	case RegexpValueType:
		if err := value.Compile(); err != nil {
			return err
		}

		s.regexps = append(s.regexps, value.Regexp)
		s.fieldValues = append(s.fieldValues, value)
	default:
		str := value.Value.(string)
		s.values = append(s.values, str)
		s.scalars[str] = true
		s.fieldValues = append(s.fieldValues, value)
	}

	return nil
}

// SetFieldValues apply field values
func (s *StringValues) SetFieldValues(values ...FieldValue) error {
	s.fieldValues = []FieldValue{}

	// reset internal caches
	s.regexps = []*regexp.Regexp{}
	s.scalars = nil

	for _, value := range values {
		if err := s.AppendFieldValue(value); err != nil {
			return err
		}
	}

	return nil
}

// AppendValue append a string value
func (s *StringValues) AppendValue(value string) {
	if s.scalars == nil {
		s.scalars = make(map[string]bool)
	}

	s.values = append(s.values, value)
	s.scalars[value] = true
	s.fieldValues = append(s.fieldValues, FieldValue{Value: value})
}

// AppendStringEvaluator append a string evalutator
func (s *StringValues) AppendStringEvaluator(evaluator *StringEvaluator) error {
	if evaluator.EvalFnc == nil {
		return errors.New("only scalar evaluator are supported")
	}

	fieldValue := FieldValue{
		Value: evaluator.Value,
		Type:  evaluator.ValueType,
	}

	return s.AppendFieldValue(fieldValue)
}
