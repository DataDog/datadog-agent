// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build lodadebug

package observerimpl

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

func TestLODADebugBurst(t *testing.T) {
	d := NewLODADetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	storage := newTimeSeriesStorage()
	rng := rand.New(rand.NewSource(3))
	const stableMean = 100.0
	const stableStd = 1.0

	t1 := int64(1)
	for i := 0; i < 200; i++ {
		v := stableMean + rng.NormFloat64()*stableStd
		storage.Add("ns", "metric", v, t1, nil)
		d.Detect(storage, t1)
		t1++
	}
	// Examine the state right before the spike
	for _, st := range d.series {
		fmt.Printf("Pre-spike: recentFilled=%d warmupCount=%d\n", st.recentFilled, st.warmupCount)
		if st.recentFilled >= 64 {
			scores := make([]float64, st.recentFilled)
			copy(scores, st.recentScores[:st.recentFilled])
			sort.Float64s(scores)
			fmt.Printf("  scores min=%.3f p50=%.3f p99=%.3f max=%.3f\n",
				scores[0], scores[st.recentFilled/2],
				scores[(st.recentFilled*99)/100], scores[st.recentFilled-1])
		}
		fmt.Printf("  binMin/Max k=5 (level): [%.3f, %.3f]\n", st.binMin[5], st.binMax[5])
		fmt.Printf("  histTotal0=%.3f hist[0]: ", st.histTotal[0])
		for b := 0; b < lodaBins; b++ {
			fmt.Printf("%.2f ", st.hist[0][b])
		}
		fmt.Println()
	}
	// Add the spike
	spikeValue := 1e6
	spikeTS := t1
	storage.Add("ns", "metric", spikeValue, spikeTS, nil)
	res := d.Detect(storage, spikeTS)
	fmt.Printf("Spike value=%.2f ts=%d anomalies=%d\n", spikeValue, spikeTS, len(res.Anomalies))
	for _, a := range res.Anomalies {
		fmt.Printf("  fire ts=%d score=%v\n", a.Timestamp, *a.Score)
	}
	// State AFTER spike
	for _, st := range d.series {
		latestScore := st.recentScores[(st.recentHead-1+lodaRecentSize)%lodaRecentSize]
		fmt.Printf("Post-spike: recentFilled=%d latestScore=%.3f\n", st.recentFilled, latestScore)
		// Print all bin masses for projection 0
		fmt.Printf("  hist[0] post-spike: ")
		for b := 0; b < lodaBins; b++ {
			fmt.Printf("%.2f ", st.hist[0][b])
		}
		fmt.Println()
	}
}

func TestLODADebugScores(t *testing.T) {
	d := NewLODADetector()
	d.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	storage := newTimeSeriesStorage()

	rng := rand.New(rand.NewSource(2))
	ts := int64(1)
	for i := 0; i < lodaWarmup; i++ {
		storage.Add("ns", "metric", 10+rng.NormFloat64()*0.5, ts, nil)
		ts++
	}
	for i := 0; i < 200; i++ {
		storage.Add("ns", "metric", 10+rng.NormFloat64()*0.5, ts, nil)
		ts++
	}
	shiftStart := ts
	for i := 0; i < 60; i++ {
		storage.Add("ns", "metric", 30+rng.NormFloat64()*0.5, ts, nil)
		ts++
	}

	res := d.Detect(storage, ts-1)
	fmt.Printf("anomalies=%d shiftStart=%d\n", len(res.Anomalies), shiftStart)
	for _, a := range res.Anomalies {
		fmt.Printf("  fire ts=%d score=%v\n", a.Timestamp, *a.Score)
	}

	// Incremental detect — Detect once per added point so we can grab the
	// score timeline.
	d3 := NewLODADetector()
	d3.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	storage3 := newTimeSeriesStorage()
	rng3 := rand.New(rand.NewSource(2))
	t3 := int64(1)
	prevScoreCount := 0
	for i := 0; i < lodaWarmup; i++ {
		storage3.Add("ns", "metric", 10+rng3.NormFloat64()*0.5, t3, nil)
		d3.Detect(storage3, t3)
		t3++
	}
	// Print scores during stable post-warmup phase
	fmt.Println("STABLE PHASE scores per tick:")
	for i := 0; i < 200; i++ {
		storage3.Add("ns", "metric", 10+rng3.NormFloat64()*0.5, t3, nil)
		d3.Detect(storage3, t3)
		for _, st := range d3.series {
			if st.recentFilled > prevScoreCount {
				latest := st.recentScores[(st.recentHead-1+lodaRecentSize)%lodaRecentSize]
				prevScoreCount = st.recentFilled
				if i < 10 || i > 195 || latest > 3.0 {
					fmt.Printf("  ts=%d score=%.3f recentFilled=%d\n", t3, latest, st.recentFilled)
				}
			}
		}
		t3++
	}
	fmt.Println("SHIFT PHASE scores per tick:")
	for i := 0; i < 60; i++ {
		storage3.Add("ns", "metric", 30+rng3.NormFloat64()*0.5, t3, nil)
		d3.Detect(storage3, t3)
		for _, st := range d3.series {
			latest := st.recentScores[(st.recentHead-1+lodaRecentSize)%lodaRecentSize]
			if i < 10 || latest > 3.0 {
				fmt.Printf("  ts=%d score=%.3f recentFilled=%d lastFireTime=%d\n", t3, latest, st.recentFilled, st.lastFireTime)
			}
		}
		t3++
	}

	// Step through the points one by one with a fresh detector to see scores per timestamp.
	d2 := NewLODADetector()
	d2.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	storage2 := newTimeSeriesStorage()
	rng2 := rand.New(rand.NewSource(2))
	allTS := []int64{}
	allValues := []float64{}
	tt := int64(1)
	for i := 0; i < lodaWarmup; i++ {
		v := 10 + rng2.NormFloat64()*0.5
		allTS = append(allTS, tt)
		allValues = append(allValues, v)
		tt++
	}
	for i := 0; i < 200; i++ {
		v := 10 + rng2.NormFloat64()*0.5
		allTS = append(allTS, tt)
		allValues = append(allValues, v)
		tt++
	}
	for i := 0; i < 60; i++ {
		v := 30 + rng2.NormFloat64()*0.5
		allTS = append(allTS, tt)
		allValues = append(allValues, v)
		tt++
	}
	// Add all and detect, one batch at a time would be expensive. Just add all and inspect state.
	for i := range allValues {
		storage2.Add("ns", "metric", allValues[i], allTS[i], nil)
	}
	d2.Detect(storage2, allTS[len(allTS)-1])
	for sk, st := range d2.series {
		fmt.Printf("ref=%d agg=%d recentFilled=%d warmupCount=%d lastFireTime=%d\n",
			sk.ref, sk.agg, st.recentFilled, st.warmupCount, st.lastFireTime)
		fmt.Printf("  histTotal=%.2f %.2f %.2f %.2f %.2f\n", st.histTotal[0], st.histTotal[1], st.histTotal[2], st.histTotal[3], st.histTotal[4])
		fmt.Printf("  binMin/Max k=5 (level): [%.3f, %.3f]\n", st.binMin[5], st.binMax[5])
		fmt.Printf("  binMin/Max k=6 (slope): [%.3f, %.3f]\n", st.binMin[6], st.binMax[6])
		fmt.Printf("  binMin/Max k=2 (slope+ema): [%.3f, %.3f]\n", st.binMin[2], st.binMax[2])
		// Distribution stats over scores
		n := st.recentFilled
		if n > 0 {
			scores := make([]float64, n)
			copy(scores, st.recentScores[:n])
			sort.Float64s(scores)
			fmt.Printf("  scores min=%.3f p50=%.3f p90=%.3f p99=%.3f max=%.3f\n",
				scores[0], scores[n/2], scores[(n*9)/10], scores[(n*99)/100], scores[n-1])
		}
	}

	// Print projection matrix
	fmt.Println("Projection matrix:")
	for k := 0; k < lodaProjections; k++ {
		fmt.Printf("  k=%d: %.3f %.3f %.3f %.3f %.3f\n", k, d.projection[k][0], d.projection[k][1], d.projection[k][2], d.projection[k][3], d.projection[k][4])
	}
}
