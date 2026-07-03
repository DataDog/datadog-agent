// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

type checkUnwrapper interface {
	Unwrap() Check
}

// As finds the first check in c's unwrap chain that implements T.
func As[T any](c Check) (T, bool) {
	var zero T
	for c != nil {
		if typed, ok := any(c).(T); ok {
			return typed, true
		}

		unwrapper, ok := c.(checkUnwrapper)
		if !ok {
			return zero, false
		}

		next := unwrapper.Unwrap()
		if next == nil || next == c {
			return zero, false
		}
		c = next
	}
	return zero, false
}
