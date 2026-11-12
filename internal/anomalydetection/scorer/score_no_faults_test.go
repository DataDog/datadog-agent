// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNoFaultReplayValidation(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "healthy"), 0755); err != nil {
		t.Fatal(err)
	}
	episode := `{"mode":"healthy-control","expectation":"healthy","evaluation_window":{"start":"2026-01-01T00:05:00Z","end":"2026-01-01T00:35:00Z"}}`
	if err := os.WriteFile(filepath.Join(root, "healthy", "episode.json"), []byte(episode), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*ObserverOutput)
		valid  bool
	}{
		{"zero false positives", func(*ObserverOutput) {}, true},
		{"missing periods", func(o *ObserverOutput) { o.AnomalyPeriods = nil }, false},
		{"disabled scorer", func(o *ObserverOutput) { o.Metadata.CorrelatorsEnabled = nil }, false},
		{"no input", func(o *ObserverOutput) { o.Metadata.Stats = json.RawMessage(`{}`) }, false},
		{"missing stats", func(o *ObserverOutput) { o.Metadata.Stats = nil }, false},
		{"invalid scenario", func(o *ObserverOutput) { o.Metadata.Scenario = "../healthy" }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := ObserverOutput{Metadata: ObserverMetadata{Scenario: "healthy", CorrelatorsEnabled: []string{"anomaly_scorer"}, Stats: json.RawMessage(`{"input_metrics_count":100}`)}, AnomalyPeriods: []ObserverCorrelation{}}
			test.mutate(&output)
			data, err := json.Marshal(output)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "output.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			score, err := scoreNoFaults(path, root)
			if !test.valid {
				if err == nil {
					t.Fatal("invalid replay received a score")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if score.FalsePositives != 0 {
				t.Fatalf("expected no false positives, got %d", score.FalsePositives)
			}
		})
	}
}
