// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"fmt"
	"regexp"
)

// PatternToRegexp converts pattern to regular expression
func PatternToRegexp(pattern string) (*regexp.Regexp, error) {
	// quote eveything except wilcard
	re := regexp.MustCompile(`[\.*+?()|\[\]{}^$]`)
	quoted := re.ReplaceAllStringFunc(pattern, func(s string) string {
		if s != "*" {
			return "\\" + s
		}
		return ".*"
	})

	return regexp.Compile("^" + quoted + "$")
}

func compilePattern(se *StringEvaluator) error {
	if se.regexp != nil {
		return nil
	}

	reg, err := PatternToRegexp(se.Value)
	if err != nil {
		return fmt.Errorf("invalid pattern '%s': %s", se.Value, err)
	}
	se.ValueType = PatternValueType
	se.regexp = reg

	return nil
}
