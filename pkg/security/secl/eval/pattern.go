// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"fmt"
	"regexp"
)

func patternToRegexp(pattern string) (*regexp.Regexp, error) {
	// do not accept full wildcard value
	if matched, err := regexp.Match(`[a-zA-Z0-9\.]+`, []byte(pattern)); err != nil || !matched {
		return nil, &ErrInvalidPattern{Pattern: pattern}
	}

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

func toPattern(se *StringEvaluator) error {
	if se.isRegexp {
		return nil
	}

	reg, err := patternToRegexp(se.Value)
	if err != nil {
		return fmt.Errorf("invalid pattern '%s': %s", se.Value, err)
	}
	se.valueType = PatternValueType
	se.isRegexp = true
	se.regexp = reg

	return nil
}
