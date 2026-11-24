// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

// contextSketches holds a sketch per context.
type contextSketches map[ckey.ContextKey]*quantile.Agent

// insert v into the sketch of a context.
func (m contextSketches) insert(ck ckey.ContextKey, v float64, sampleRate float64) bool {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return false
	}

	m.getOrCreate(ck).Insert(v, sampleRate)
	return true
}

// insertInterp inserts count values interpolated over [lower, upper] into the sketch of a context.
func (m contextSketches) insertInterp(ck ckey.ContextKey, lower float64, upper float64, count uint) bool {
	if math.IsInf(lower, 0) || math.IsNaN(lower) || math.IsInf(upper, 0) || math.IsNaN(upper) {
		return false
	}

	// Use the error to indicate whether the insertion happened.
	err := m.getOrCreate(ck).InsertInterpolate(lower, upper, count)
	return err == nil
}

func (m contextSketches) getOrCreate(ck ckey.ContextKey) *quantile.Agent {
	s, ok := m[ck]
	if !ok {
		s = &quantile.Agent{}
		m[ck] = s
	}
	return s
}
