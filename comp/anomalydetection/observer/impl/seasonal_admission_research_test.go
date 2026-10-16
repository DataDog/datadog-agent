// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build anomalydetectiontestbench

package observerimpl

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

type admissionReverseReader struct{ observer.StorageReader }

func (r admissionReverseReader) ListSeries(f observer.SeriesFilter) []observer.SeriesMeta {
	metas := r.StorageReader.ListSeries(f)
	sort.Slice(metas, func(i, j int) bool { return metas[i].Ref > metas[j].Ref })
	return metas
}

func TestSeasonalAdmissionFutureOrder(t *testing.T) {
	defer admissionTestSelection()()
	s := newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 120})
	for i := 0; i < 3; i++ {
		s.Add("admission", "unseen.workload", 100, 100015, []string{"candidate:yes", fmt.Sprintf("id:%d", i)})
	}
	metas := s.ListSeries(observer.WorkloadSeriesFilter())
	sort.Slice(metas, func(i, j int) bool { return metas[i].Ref < metas[j].Ref })
	for _, reverse := range []bool{false, true} {
		d := realSeasonalTestWrapper("phase_pooled_floor")
		d.budget = newSeasonalAdmissionBudget(1)
		d.Detect(s, 100000)
		if d.budget.active != 0 {
			t.Fatal("future point allocated a learner")
		}
		var reader observer.StorageReader = s
		if reverse {
			reader = admissionReverseReader{s}
		}
		d.Detect(reader, 100015)
		if !d.budget.entries[metas[0].Ref].active || d.budget.active != 1 {
			t.Fatal("same-tick admission depended on metadata ordering")
		}
		admissionAssertBound(t, d)
	}
}

func admissionTestSelection() func() {
	old := realSeasonalSelectors
	realSeasonalSelectors = []realSeasonalSelector{{Name: "unseen.workload", Tags: []string{"candidate:yes"}}}
	return func() { realSeasonalSelectors = old }
}

func admissionAssertBound(t *testing.T, d *realSeasonalDetector) {
	t.Helper()
	active := 0
	for ref, e := range d.budget.entries {
		s := d.states[ref]
		if s == nil {
			t.Fatal("orphaned admission entry")
		}
		if e.active {
			active++
		}
		if !e.active && !e.done && s.model != nil {
			t.Fatal("waiting series allocated a model")
		}
		if e.done && s.model != nil && (len(s.model.History) > 0 || len(s.calibration) > 0) {
			t.Fatal("finished learner retained training state")
		}
	}
	if active != d.budget.active || active > d.budget.limit || len(d.budget.entries) != len(d.states) {
		t.Fatalf("budget invariant: %v", d.budget.census())
	}
}

func admissionSortResult(r observer.DetectionResult) []byte {
	sort.Slice(r.Anomalies, func(i, j int) bool {
		a, b := r.Anomalies[i], r.Anomalies[j]
		if a.SourceRef.Ref != b.SourceRef.Ref {
			return a.SourceRef.Ref < b.SourceRef.Ref
		}
		if a.SourceRef.Aggregate != b.SourceRef.Aggregate {
			return a.SourceRef.Aggregate < b.SourceRef.Aggregate
		}
		return a.Timestamp < b.Timestamp
	})
	b, err := json.Marshal(r)
	if err != nil {
		panic(err)
	}
	return b
}

func TestSeasonalAdmissionRawContinuity(t *testing.T) {
	defer admissionTestSelection()()
	s := newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 120})
	d := realSeasonalTestWrapper("phase_pooled_floor")
	d.budget = newSeasonalAdmissionBudget(1)
	raw := realSeasonalTestFactory()
	for slot := 0; slot < 400; slot++ {
		now := int64(100000 + slot*15)
		// The first learner becomes idle without storage eviction. Waiting series
		// must keep exactly the same raw detector state before and after its grant.
		if slot < 100 {
			s.Add("admission", "unseen.workload", 100, now, []string{"candidate:yes", "id:0"})
		}
		v := 100 + float64(slot%9)
		if slot >= 90 && slot < 120 {
			v += 300
		}
		s.Add("admission", "unseen.workload", v, now, []string{"candidate:yes", "id:1"})
		a, b := admissionSortResult(raw.Detect(s, now)), admissionSortResult(d.Detect(s, now))
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("raw continuity changed at slot %d\n%s\n%s", slot, a, b)
		}
		admissionAssertBound(t, d)
	}
	if d.budget.grants != 2 || d.budget.idleReleases != 1 {
		t.Fatalf("did not exercise idle release: %v", d.budget.census())
	}
	d.Reset()
	if len(d.states) != 0 || len(d.budget.entries) != 0 || d.budget.queue.Len() != 0 || d.budget.active != 0 {
		t.Fatal("reset leaked state")
	}
}

func TestSeasonalAdmissionChurn(t *testing.T) {
	if os.Getenv("SEASONAL_ADMISSION_OUTPUT") == "" {
		t.Skip("explicit long churn experiment")
	}
	defer admissionTestSelection()()
	report := map[string]any{}
	for _, limit := range []int{2, 4} {
		for _, reverse := range []bool{false, true} {
			s := newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 120})
			d := realSeasonalTestWrapper("phase_pooled_floor")
			d.budget = newSeasonalAdmissionBudget(limit)
			ids := []int{0, 1, 2, 3, 4, 5}
			if reverse {
				sort.Sort(sort.Reverse(sort.IntSlice(ids)))
			}
			ready := map[int]int{}
			refs := map[int]observer.SeriesRef{}
			maxStates, maxHistory := 0, 0
			for slot := 0; slot <= 9600; slot++ {
				now := int64(100000 + slot*15)
				// Two permanent nonseasonal occupants precede the useful series.
				for i := 0; i < 2; i++ {
					s.Add("admission", "unseen.workload", 100, now, []string{"candidate:yes", fmt.Sprintf("flat:%d", i)})
				}
				// Eight identities turn over every hour, including initial waiters.
				for i := 0; i < 8; i++ {
					s.Add("admission", "unseen.workload", float64(slot%37), now, []string{"candidate:yes", fmt.Sprintf("transient:%d:%d", slot/240, i)})
				}
				for _, i := range ids {
					phase := slot % 240
					v := 100.0
					switch i % 3 {
					case 0:
						if phase >= 120 && phase < 128 {
							v += 150
						}
					case 1:
						v += 25 * math.Sin(float64(phase)*2*math.Pi/240)
					case 2:
						v += float64(phase) / 3
					}
					v += .2 * math.Sin(float64(slot)*.413+float64(i))
					v *= 1 + float64(i)/3
					s.Add("admission", "unseen.workload", v, now, []string{"candidate:yes", fmt.Sprintf("stable:%d", i)})
				}
				removed := s.EvictInactiveBefore(now - 1)
				d.RemoveSeries(removed)
				d.Detect(s, now)
				if slot == 0 {
					for _, m := range s.ListSeries(observer.WorkloadSeriesFilter()) {
						for i := range 6 {
							if m.Tags.Find(func(tag string) bool { return tag == fmt.Sprintf("stable:%d", i) }) {
								refs[i] = m.Ref
							}
						}
					}
				}
				admissionAssertBound(t, d)
				if len(d.states) > 16 {
					t.Fatal("churn leaked metadata")
				}
				maxStates = max(maxStates, len(d.states))
				history := 0
				for _, state := range d.states {
					if state.model != nil {
						history += cap(state.model.History)*8 + cap(state.model.Valid)
					}
				}
				maxHistory = max(maxHistory, history)
				for i, ref := range refs {
					if d.states[ref].mature {
						if _, ok := ready[i]; !ok {
							ready[i] = slot
						}
					}
				}
			}
			if len(ready) != 6 || d.budget.failures < 2 || d.budget.removals < 300 {
				t.Fatalf("starvation or missing coverage: ready=%v budget=%v", ready, d.budget.census())
			}
			if maxHistory > limit*1920*9 {
				t.Fatal("training history exceeded learner bound")
			}
			for _, ref := range refs {
				if d.states[ref].resets != 1 {
					t.Fatal("arrival churn reset an admitted series")
				}
			}
			report[fmt.Sprintf("limit%d_reverse%v", limit, reverse)] = map[string]any{"ready_slots": ready, "budget": d.budget.census(), "max_states": maxStates, "max_history_payload_bytes": maxHistory, "duration_hours": 40}
			all := make([]observer.SeriesRef, 0, len(d.states))
			for ref := range d.states {
				all = append(all, ref)
			}
			d.RemoveSeries(all)
			if len(d.states) != 0 || len(d.budget.entries) != 0 || d.budget.queue.Len() != 0 || d.budget.active != 0 {
				t.Fatal("eviction leaked state")
			}
		}
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(os.Getenv("SEASONAL_ADMISSION_OUTPUT"), b, 0644); err != nil {
		t.Fatal(err)
	}
}
