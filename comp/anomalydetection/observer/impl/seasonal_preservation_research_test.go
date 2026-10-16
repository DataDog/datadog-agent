// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build anomalydetectiontestbench

package observerimpl

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// Compare against the original JSON-based selection semantics, including both
// composite-tag halves, escaping, duplicate tags and malformed UTF-8.
func TestRealSeasonalSelectionSemantics(t *testing.T) {
	old := realSeasonalSelectors
	defer func() { realSeasonalSelectors = old }()
	tags := []string{"service:backend", "interface:eth0", "quote:\"\\\n", "unicode:λ", "", "dup", "bad:\xff\xfe", "bad:\ufffd\ufffd"}
	for split := 0; split <= len(tags); split++ {
		meta := observer.SeriesMeta{Name: "metric", Tags: tagset.NewCompositeTags(tags[:split], tags[split:])}
		for _, required := range append(append([]string{}, tags...), "absent") {
			realSeasonalSelectors = []realSeasonalSelector{{Name: "metric", Tags: []string{required, "service:backend"}}}
			data, err := json.Marshal(meta.Tags)
			if err != nil {
				t.Fatal(err)
			}
			var decoded []string
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			want := false
			for _, tag := range decoded {
				if tag == required {
					want = true
				}
			}
			if got := realSeasonalEligible(meta); got != want {
				t.Fatalf("split%d required%q: got%v want%v", split, required, got, want)
			}
		}
	}
	realSeasonalSelectors = []realSeasonalSelector{{Name: "other", Tags: nil}, {Name: "metric", Tags: []string{"absent"}}}
	if realSeasonalEligible(observer.SeriesMeta{Name: "metric"}) {
		t.Fatal("wrong role")
	}
}

func TestRealSeasonalFrozenStorageRelease(t *testing.T) {
	if os.Getenv("SEASONAL_EXPECT_RELEASE") == "" {
		t.Skip("reference retains history")
	}
	d := realSeasonalTestWrapper("phase_pooled_floor")
	storage := newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 120})
	for slot := 0; slot <= 2400; slot++ {
		v := 100 + 20*math.Sin(float64(slot%240)*2*math.Pi/240)
		storage.Add("contract", "system.load.1", v, 100000+int64(slot)*15, nil)
		d.Detect(storage, 100000+int64(slot)*15)
	}
	s := d.states[realSeasonalTestMeta(t, storage).Ref]
	if !s.mature || len(s.model.History) != 0 || len(s.model.Valid) != 0 || len(s.calibration) != 0 {
		t.Fatal("frozen state retained training/calibration data or did not mature")
	}
	if os.Getenv("SEASONAL_EXPECT_COMPACT") != "" {
		if len(s.model.Profile.Scale) != 0 || len(s.scale.Original) != 0 || len(s.model.Profile.Center) == 0 || len(s.scale.Applied) == 0 {
			t.Fatal("wrong inference-array release")
		}
	}
	d.Reset()
	if len(d.states) != 0 {
		t.Fatal("reset retained per-series seasonal state")
	}
	// Permanent disqualification also releases history; do not retain a failed candidate.
	d = realSeasonalTestWrapper("phase_pooled_floor")
	storage = newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 120})
	storage.Add("contract", "system.load.1", 1, 100000, nil)
	d.Detect(storage, 100000)
	storage.Add("contract", "system.load.1", 2, 100030, nil)
	d.Detect(storage, 100030)
	s = d.states[realSeasonalTestMeta(t, storage).Ref]
	if s.reason == "" || len(s.model.History) != 0 || len(s.model.Valid) != 0 {
		t.Fatal("failed candidate retained history")
	}
}

func seasonalPreservationValue(kind string, slot int) float64 {
	period := 240
	if kind == "half_hour_pulse" {
		period = 120
	}
	p := slot % period
	value := 100.0
	switch kind {
	case "sine", "timestamp_jitter", "training_gap", "mature_gap", "restart", "evict_recreate":
		value += 25 * math.Sin(float64(p)*2*math.Pi/float64(period))
	case "ramp":
		value += float64(p) / 3
	case "square":
		if p >= period/2 {
			value += 100
		}
	case "flat":
		return 100
	case "nonperiodic":
		value += 4*math.Sin(float64(slot)*.123) + 3*math.Cos(float64(slot)*.771)
		if slot >= 1000 {
			value += 75
		}
		if slot >= 2000 {
			value -= 180
		}
	default:
		if p >= period/2 && p < period/2+8 {
			value += 150
		}
	}
	if kind == "amplitude_jitter" && p >= 120 && p < 128 {
		value += 3 * math.Sin(float64(slot/240)*1.7)
	}
	if kind == "drift" {
		value += float64(slot) * .004
	}
	value += .2 * math.Sin(float64(slot)*.413)
	// Fixed, predeclared off-pattern excursions after readiness: both directions.
	if slot >= 2700 && slot < 2708 {
		value += 80
	}
	if slot >= 2800 && slot < 2808 {
		value -= 90
	}
	return value
}

// This matrix tests distribution-independent behavior preservation, not a claim
// that every periodic input has fewer alerts. Compare the complete JSON result
// against the reference implementation before accepting an optimization.
func TestRealSeasonalPreservationMatrix(t *testing.T) {
	path := os.Getenv("SEASONAL_MATRIX_OUTPUT")
	if path == "" {
		t.Skip("explicit matrix run")
	}
	old := realSeasonalSelectors
	realSeasonalSelectors = nil
	defer func() { realSeasonalSelectors = old }()
	cases := []string{"hour_pulse", "half_hour_pulse", "sine", "ramp", "square", "amplitude_jitter", "timestamp_jitter", "drift", "flat", "nonperiodic", "training_gap", "mature_gap", "restart", "evict_recreate"}
	modes := []string{"raw_unwrapped", "raw_wrapped_nochange", "raw_fullcycle", "residual_fullcycle", "phase_pooled_floor"}
	out := map[string]any{}
	for _, kind := range cases {
		for _, mode := range modes {
			storage := newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 120})
			var d observer.Detector
			if mode == "raw_unwrapped" {
				d = realSeasonalTestFactory()
			} else {
				d = realSeasonalTestWrapper(mode)
			}
			h := sha256.New()
			events := []int64{}
			activations := []int{}
			previousMature := false
			for slot := 0; slot <= 2880; slot++ {
				if kind == "training_gap" && slot == 100 {
					continue
				}
				if kind == "mature_gap" && slot == 2500 {
					continue
				}
				if kind == "restart" && slot == 1800 {
					d.(interface{ Reset() }).Reset()
					storage = newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 120})
					previousMature = false
				}
				if kind == "evict_recreate" && slot == 2500 {
					refs := storage.EvictInactiveBefore(int64(100000 + slot*15))
					if len(refs) != 1 {
						t.Fatal("eviction fixture did not remove the live series")
					}
					d.(observer.SeriesRemover).RemoveSeries(refs)
					previousMature = false
				}
				ts := int64(100000 + slot*15)
				if kind == "timestamp_jitter" && slot%4 == 1 {
					ts++
				}
				storage.Add("contract", "system.load.1", seasonalPreservationValue(kind, slot), ts, []string{"fixture:general"})
				r := d.Detect(storage, ts)
				realSeasonalSortAnomalies(r.Anomalies)
				data, err := json.Marshal(r)
				if err != nil {
					t.Fatal(err)
				}
				h.Write(data)
				for _, a := range r.Anomalies {
					events = append(events, a.Timestamp)
				}
				if w, ok := d.(*realSeasonalDetector); ok {
					for _, s := range w.states {
						if s.mature && !previousMature {
							activations = append(activations, slot)
						}
						previousMature = s.mature
					}
				}
			}
			row := map[string]any{"detector_output_sha256": hex.EncodeToString(h.Sum(nil)), "event_timestamps": events, "activation_slots": activations, "ready": d.Ready()}
			if w, ok := d.(*realSeasonalDetector); ok {
				for _, s := range w.states {
					row["reason"] = s.reason
					row["resets"] = s.resets
					row["calibration_variance"] = s.calibrationVariance
					if s.model.Profile != nil {
						row["period"] = s.model.Profile.Period
						row["centers"] = s.model.Profile.Center
					}
					if s.scale != nil {
						row["applied_scales"] = s.scale.Applied
					}
				}
			}
			out[fmt.Sprintf("%s/%s", kind, mode)] = row
		}
	}
	// Ensure this matrix contains admitted models and genuine fallback paths.
	admitted := out["hour_pulse/phase_pooled_floor"].(map[string]any)
	if !reflect.DeepEqual(admitted["activation_slots"], []int{2400}) {
		t.Fatalf("hourly fixture failed to mature: %v", admitted["activation_slots"])
	}
	if out["training_gap/phase_pooled_floor"].(map[string]any)["reason"] != "missing_prereadiness_slot" {
		t.Fatal("missing fallback coverage")
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}
