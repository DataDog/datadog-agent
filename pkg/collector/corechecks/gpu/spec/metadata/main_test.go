// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import "testing"

func TestMetadataMetricType(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"gauge":           {input: "gauge", want: "gauge"},
		"rate":            {input: "rate", want: "gauge"},
		"count":           {input: "count", want: "count"},
		"monotonic count": {input: "monotonic_count", want: "count"},
		"counter":         {input: "counter", want: "rate"},
		"histogram":       {input: "histogram", want: "rate"},
		"historate":       {input: "historate", want: "rate"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := metadataMetricType(test.input)
			if err != nil {
				t.Fatalf("metadataMetricType(%q) returned an error: %v", test.input, err)
			}
			if got != test.want {
				t.Errorf("metadataMetricType(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestMetadataMetricTypeRejectsUnknownType(t *testing.T) {
	_, err := metadataMetricType("distribution")
	if err == nil {
		t.Fatal("metadataMetricType() returned nil error, want error")
	}
}
