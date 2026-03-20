// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"strconv"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// seriesPairKey identifies an unordered relationship between two series refs.
// It is canonicalized so (A,B) and (B,A) map to the same key.
type seriesPairKey struct {
	A observer.SeriesRef
	B observer.SeriesRef
}

func newSeriesPairKey(a, b observer.SeriesRef) seriesPairKey {
	if a <= b {
		return seriesPairKey{A: a, B: b}
	}
	return seriesPairKey{A: b, B: a}
}

// hashKey returns a deterministic encoding used only for hashing/sketch updates.
func (k seriesPairKey) hashKey() string {
	return strconv.Itoa(int(k.A)) + "|" + strconv.Itoa(int(k.B))
}
