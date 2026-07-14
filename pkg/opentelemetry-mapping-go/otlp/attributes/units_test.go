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
		// Durations.
		{"s", "second", true},
		{"us", "microsecond", true},
		{"ns", "nanosecond", true},
		// Percentages and special symbols.
		{"%", "percent", true},
		// Frequency.
		{"Hz", "hertz", true},
		{"MHz", "megahertz", true},
		// Rate units.
		{"By/s", "byte/second", true},
		{"GiBy/h", "gibibyte/hour", true},
		// Dimensionless and empty.
		{"1", "", false},
		{"", "", false},
		// Annotations.
		{"{cpu}", "cpu", true},
		{"{connection}", "connection", true},
		{"{pod}", "", false},
		// Prefix/base decomposition that resolves to units.
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
		// Prefix/base decomposition that does not resolve to a unit.
		{"dBy", "", false},
		{"mBy", "", false},
		{"kV", "", false},
		{"GJ", "", false},
		// Prefix/base on annotations is not supported.
		{"{core}", "core", true},
		{"{millicore}", "", false},
		// Unknown units.
		{"teaspoon", "", false},
		{"By/teaspoon", "", false},
		{"{teaspoon}", "", false},
		{"{packet}/teaspoon", "", false},
		// Incoherent or unsupported units.
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
