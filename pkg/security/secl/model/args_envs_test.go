// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"reflect"
	"testing"
)

func TestEnvsEntryKeyValues(t *testing.T) {
	entry := &EnvsEntry{Values: []string{
		"BASH_ENV=/tmp/evil.sh",
		"BASH_ENV=",
		"EMPTY=",
		"URL=a=b",
		"MALFORMED",
	}}

	expected := map[string]string{
		// a variable set more than once has its first value, the one getenv returns
		"BASH_ENV": "/tmp/evil.sh",
		"EMPTY":    "",
		"URL":      "a=b",
	}
	if kv := entry.KeyValues(); !reflect.DeepEqual(kv, expected) {
		t.Errorf("expected %v, got %v", expected, kv)
	}
	if value := entry.Get("BASH_ENV"); value != "/tmp/evil.sh" {
		t.Errorf("expected the first value of BASH_ENV, got %q", value)
	}
}
