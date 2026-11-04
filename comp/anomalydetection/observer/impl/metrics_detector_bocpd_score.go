// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

const bocpdScorePoints = 60

// bocpdScoreWorkspace is shared by all series in the single-writer detector.
// History stays in storage; this workspace is allocated only on the first alert.
type bocpdScoreWorkspace struct {
	values  [bocpdScorePoints]float64
	scratch [bocpdScorePoints]float64
	count   int
	next    int
	collect func(*observer.Series, observer.Point)
}

func (w *bocpdScoreWorkspace) accept(_ *observer.Series, p observer.Point) {
	w.values[w.next] = p.Value
	w.next = (w.next + 1) % bocpdScorePoints
	if w.count < bocpdScorePoints {
		w.count++
	}
}

// score measures point extremeness, not changepoint confidence. The candidate
// and future observations cannot enter its reference window. Missing history or
// non-finite arithmetic leaves Score unavailable rather than fabricating zero.
func (w *bocpdScoreWorkspace) score(storage observer.StorageReader, ref observer.SeriesRef, agg observer.Aggregate, at int64, value, floor float64) *float64 {
	if at <= 0 || !finiteBOCPDScore(value) || !finiteBOCPDScore(floor) || floor <= 0 {
		return nil
	}
	w.count, w.next = 0, 0
	if w.collect == nil {
		w.collect = w.accept
	}
	if tail, ok := storage.(tailPointReader); ok {
		tail.ForEachLastPoints(ref, at-1, bocpdScorePoints, agg, w.collect)
	} else {
		// Keep wrappers and other StorageReader implementations compatible.
		// The fixed ring retains only the newest 60 observations in this path.
		storage.ForEachPoint(ref, 0, at-1, agg, w.collect)
	}
	if w.count < bocpdScorePoints {
		return nil
	}
	for i, v := range w.values {
		if !finiteBOCPDScore(v) {
			return nil
		}
		w.scratch[i] = v
	}
	sort.Float64s(w.scratch[:])
	median := w.scratch[29]/2 + w.scratch[30]/2
	for i, v := range w.values {
		w.scratch[i] = math.Abs(v - median)
	}
	sort.Float64s(w.scratch[:])
	mad := w.scratch[29]/2 + w.scratch[30]/2
	denominator := math.Max(1.4826*mad, floor)
	if !finiteBOCPDScore(denominator) {
		return nil
	}
	score := math.Abs(value-median) / denominator
	if !finiteBOCPDScore(score) {
		return nil
	}
	return &score
}

func finiteBOCPDScore(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
