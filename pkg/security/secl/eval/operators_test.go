// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"testing"
)

func TestPatternValue(t *testing.T) {
	re, err := patternToRegexp("^$[]{}+?/etc/?+*.conf")
	if err != nil {
		t.Fatal(err)
	}

	if re.String() != "^\\^\\$\\[\\]\\{\\}\\+\\?/etc/\\?\\+.*\\.conf$" {
		t.Fatalf("expected regexp not found: %s", re.String())
	}

	if _, err = patternToRegexp("*"); err == nil {
		t.Fatal("wildcard only pattern is not supported")
	}
}
