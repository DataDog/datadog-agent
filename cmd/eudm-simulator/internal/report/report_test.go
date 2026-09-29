// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
)

func fixture() *Report {
	plan := &schema.RunPlan{RunID: strings.Repeat("a", 32), ScenarioDigest: strings.Repeat("b", 64), AgentCommit: strings.Repeat("c", 40), Seed: 123, Start: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC), Bundle: schema.BundleRef{Digest: strings.Repeat("d", 64)}}
	scenario := &schema.Scenario{Fleet: []schema.GroupDef{{Group: "affected", Count: 2}}, Expectation: schema.Expectation{Conclusion: schema.VPNPath, AffectedCohorts: []string{"affected"}}, Phases: []schema.Phase{{Name: "healthy", Duration: schema.Duration{Duration: 20 * time.Minute}}, {Name: "onset", Duration: schema.Duration{Duration: time.Minute}}}}
	r := New(plan, scenario, "linux")
	for i := range 2 {
		r.AddDevice(i, "opaque-host-"+string(rune('a'+i)), "affected", plan.Bundle.Digest, []schema.Stream{schema.Metrics, schema.Processes})
		for _, counts := range r.Ledger[i].Streams {
			counts.Expected, counts.Delivered = 80, 80
		}
	}
	r.End = plan.Start.Add(21 * time.Minute)
	return r
}

func TestReportRejectsPartialFleetAndBatchFailure(t *testing.T) {
	r := fixture()
	if !r.Complete() {
		t.Fatal("complete ledger rejected")
	}
	for _, mutate := range []func(*Report){
		func(r *Report) { r.Ledger = r.Ledger[:1] },
		func(r *Report) { r.Ledger[1] = r.Ledger[0] },
		func(r *Report) { r.Ledger[1].Streams[schema.Processes].Delivered-- },
		func(r *Report) { r.Ledger[1].Streams[schema.Processes].Failed++ },
		func(r *Report) { r.Ledger[1].Streams[schema.Processes].Expected = 0 },
		func(r *Report) { r.NetworkStreams["ndm_metadata"] = &Counts{Expected: 2, Delivered: 1, Failed: 1} },
		func(r *Report) { r.Errors = append(r.Errors, "delivery exhausted") },
		func(r *Report) { r.End = time.Time{} },
	} {
		r := fixture()
		mutate(r)
		if r.Complete() {
			t.Fatal("partial or failed fleet reported as complete")
		}
	}
}

func TestLocalReportContainsPortableEvidenceAndRefusesOverwrite(t *testing.T) {
	r := fixture()
	r.Status = "succeeded"
	r.NetworkDevices = []string{"eudm-opaque:device-1"}
	r.NetworkStreams["ndm_metadata"] = &Counts{Expected: 1, Delivered: 1}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := r.Write(path); err != nil {
		t.Fatal(err)
	}
	if err := r.Write(path); err == nil {
		t.Fatal("overwrote a prior report")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got Report
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Complete() || got.Version != 2 || got.ReplayOS != "linux" || got.Seed != 123 || got.Status != "succeeded" || got.Expectation.Conclusion != schema.VPNPath || got.ScenarioDigest != r.ScenarioDigest || got.BundleDigest != r.BundleDigest {
		t.Fatal("lost local report contract")
	}
	if got.Phases[1].StartOffset != 20*time.Minute || got.Phases[1].Duration != time.Minute || got.Selectors["telemetry"] != "eudm_run_id:"+r.RunID || strings.Contains(got.Selectors["telemetry"], "affected") {
		t.Fatal("phase timing or opaque product selector changed")
	}
}
