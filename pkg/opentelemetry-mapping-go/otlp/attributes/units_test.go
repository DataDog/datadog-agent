// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package attributes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnitMapperMap(t *testing.T) {
	tests := []struct {
		ucum   string
		want   string
		wantOK bool
	}{
		// Bytes and byte prefixes.
		{"By", "byte", true},
		{"KiBy", "kibibyte", true},
		{"MiBy", "mebibyte", true},
		// Durations (prefix + second).
		{"s", "second", true},
		{"us", "microsecond", true},
		{"ns", "nanosecond", true},
		// Percentages and special symbols.
		{"%", "percent", true},
		// Frequency (prefix + hertz).
		{"Hz", "hertz", true},
		{"MHz", "megahertz", true},
		// Other base units.
		{"W", "watt", true},
		{"V", "volt", true},
		{"J", "joule", true},
		// Rate-style (fraction) units.
		{"By/s", "byte/second", true},
		{"GiBy/h", "gibibyte/hour", true},
		// Dimensionless and empty map to no unit.
		{"1", "", false},
		{"", "", false},
		// Annotations map to their contents.
		{"{cpu}", "cpu", true},
		{"{connection}", "connection", true},
		{"{pod}", "", false},
		// Generalized prefix/base decomposition that resolves to real catalog units.
		{"kBy", "kilobyte", true},
		{"EBy", "exabyte", true},
		{"EiBy", "exbibyte", true},
		{"kHz", "kilohertz", true},
		{"kW", "kilowatt", true},
		{"TW", "terawatt", true},
		{"mV", "millivolt", true},
		{"nJ", "nanojoule", true},
		{"KiBy/s", "kibibyte/second", true},
		{"{packet}/s", "packet/second", true},
		// Decomposable but not in the Datadog catalog: dropped, not invented.
		{"dBy", "", false},
		{"mBy", "", false},
		{"kV", "", false},
		{"GJ", "", false},
		// No UCUM "core" unit: {core} is annotation-only; prefixed cores drop.
		{"{core}", "core", true},
		{"{millicore}", "", false},
		// Unknown units are dropped.
		{"furlong", "", false},
		{"By/furlong", "", false},
		{"{furlong}", "", false},
		{"{packet}/furlong", "", false},
		// Malformed or unsupported shapes drop safely, never invent a unit.
		{"{}", "", false},
		{"By//s", "", false},
		{"By/s/s", "", false},
		{"W.h", "", false},
		{"By{transmitted}", "", false},
	}

	mapper := NewUnitMapper()
	for _, tt := range tests {
		t.Run(tt.ucum, func(t *testing.T) {
			got, ok := mapper.Map(tt.ucum)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestNoDeadPrefixes ensures every UCUM prefix combines with at least one base
// unit to form a real Datadog unit. A prefix that maps to nothing is dead weight
// and should be removed from ucumPrefixes.
func TestNoDeadPrefixes(t *testing.T) {
	mapper := NewUnitMapper()
	for symbol, name := range ucumPrefixes {
		var produces bool
		for base := range ucumBaseUnits {
			if _, ok := mapper.Map(symbol + base); ok {
				produces = true
				break
			}
		}
		assert.True(t, produces, "%q (%s) is not a supported prefix for any Datadog unit", symbol, name)
	}
}
