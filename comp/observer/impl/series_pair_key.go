// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// seriesPairKey identifies an unordered relationship between two series.
// A and B are descriptor key strings (from SeriesDescriptor.Key()).
// It is canonicalized so (A,B) and (B,A) map to the same key.
type seriesPairKey struct {
	A string
	B string
}

func newSeriesPairKey(a, b string) seriesPairKey {
	if a <= b {
		return seriesPairKey{A: a, B: b}
	}
	return seriesPairKey{A: b, B: a}
}

// hashKey returns a deterministic encoding used only for hashing/sketch updates.
func (k seriesPairKey) hashKey() string {
	return k.A + "|" + k.B
}
