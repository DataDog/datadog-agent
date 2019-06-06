// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

// EndLineMatcher defines the criterion to whether to end a line or not.
type EndLineMatcher interface {
	// Match takes the existing bytes and the bytes to be appended, returns
	// true if the combination matches the end of line condition.
	Match(exists []byte, appender []byte, start int, end int) bool
}

type newLineMatcher struct {
	EndLineMatcher
}

// Match returns true whenever a '\n' (newline) is met.
func (n *newLineMatcher) Match(exists []byte, appender []byte, start int, end int) bool {
	return appender[end] == '\n'
}
