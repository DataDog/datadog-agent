// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package percentile

import (
	"math"
	"sort"

	log "github.com/cihub/seelog"
)

// CompleteDS stores all the data and therefore provides accurate
// percentiles to compare to.
type CompleteDS struct {
	Values []float64 `json:"vals"`
	Min    float64   `json:"min"`
	Max    float64   `json:"max"`
	Count  int64     `json:"cnt"`
	Sum    float64   `json:"sum"`
	Avg    float64   `json:"avg"`
	sorted bool
}

// NewCompleteDS returns a newly initialized CompleteDS
func NewCompleteDS() CompleteDS {
	return CompleteDS{Min: math.Inf(1), Max: math.Inf(-1)}
}

// Add a new value to the sketch
func (s CompleteDS) Add(v float64) QSketch {
	s.Count++
	s.Sum += v
	s.Avg += (v - s.Avg) / float64(s.Count)
	if v < s.Min {
		s.Min = v
	}
	if v > s.Max {
		s.Max = v
	}
	s.Values = append(s.Values, v)
	s.sorted = false

	return QSketch(s)
}

// Merge another CompleteDS to the current
func (s CompleteDS) Merge(o CompleteDS) CompleteDS {
	if o.Count == 0 {
		return s
	}
	if s.Count == 0 {
		return o
	}
	s.Count += o.Count
	s.Sum += o.Sum
	s.Avg = s.Avg + (o.Avg-s.Avg)*float64(o.Count)/float64(s.Count)
	if o.Min < s.Min {
		s.Min = o.Min
	}
	if o.Max > s.Max {
		s.Max = o.Max
	}
	s.Values = append(s.Values, o.Values...)
	s.sorted = false
	return s
}

// Quantile returns the accurate quantile q
func (s CompleteDS) Quantile(q float64) float64 {
	if q < 0 || q > 1 {
		log.Errorf("Quantile out of bounds")
		return math.NaN()
	}
	if s.Count == 0 {
		return math.NaN()
	}

	if q == 0 {
		return s.Min
	} else if q == 1 {
		return s.Max
	}

	if s.Count < int64(1/EPSILON) {
		return s.interpolatedQuantile(q)
	}
	if !s.sorted {
		sort.Float64s(s.Values)
		s.sorted = true
	}
	rank := int64(q * float64(s.Count-1))
	return s.Values[rank]
}

func (s CompleteDS) interpolatedQuantile(q float64) float64 {
	if !s.sorted {
		sort.Float64s(s.Values)
		s.sorted = true
	}
	rank := q * float64(s.Count-1)
	indexBelow := int64(rank)
	indexAbove := indexBelow + 1
	if indexAbove > s.Count-1 {
		indexAbove = s.Count - 1
	}
	weightAbove := rank - float64(indexBelow)
	weightBelow := 1.0 - weightAbove

	return weightBelow*s.Values[indexBelow] + weightAbove*s.Values[indexAbove]
}
