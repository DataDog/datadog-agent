// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import "strconv"
import observer "github.com/DataDog/datadog-agent/comp/observer/def"

// seriesPairKey identifies an unordered relationship between two series IDs.
// It is canonicalized so (A,B) and (B,A) map to the same key.
type seriesPairKey struct {
	A observer.SeriesID
	B observer.SeriesID
}

func newSeriesPairKey(a, b observer.SeriesID) seriesPairKey {
	if a <= b {
		return seriesPairKey{A: a, B: b}
	}
	return seriesPairKey{A: b, B: a}
}

// hashKey returns a deterministic encoding used only for hashing/sketch updates.
// Length-prefixing avoids delimiter ambiguity without requiring parse/split logic.
func (k seriesPairKey) hashKey() string {
	a := string(k.A)
	b := string(k.B)
	return strconv.Itoa(len(a)) + ":" + a + strconv.Itoa(len(b)) + ":" + b
}

// displayKey returns a human-readable key for logs/debug payloads.
func (k seriesPairKey) displayKey() string {
	return string(k.A) + "<>" + string(k.B)
}
