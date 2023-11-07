// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package expvarcollector

import (
	"fmt"
	"testing"
)

func TestCollect(t *testing.T) {
	RegisterExpvarCallback("testing_no_error", func() (interface{}, error) {
		return "foo", nil
	})

	RegisterExpvarCallback("testing_with_error", func() (interface{}, error) {
		return nil, fmt.Errorf("testin the error case")
	})

	stats := make(map[string]interface{})
	stats, errors := Collect(stats)

	if len(errors) != 1 {
		t.Errorf("Expected to have one error got %d", len(errors))
	}
	_, ok := stats["testing_with_error"]
	if ok {
		t.Error("Reports with errors do not add an entry into the stats map.")
	}

	val, ok := stats["testing_no_error"]
	if !ok {
		t.Error("Reports without errors must add an entry into the stats map.")
	}

	stringVal := val.(string)
	if stringVal != "foo" {
		t.Errorf("Invaid report result expected 'foo' got: %s", stringVal)
	}

	ResetExpvarRegistry()

	stats = make(map[string]interface{})
	stats, errors = Report(stats)

	if len(errors) != 0 {
		t.Errorf("Expected to have zero errors got %d", len(errors))
	}

	_, ok = stats["testing_no_error"]
	if ok {
		t.Error("After calling ResetExpvarRegistry we should not populate stats with any report information")
	}
}
