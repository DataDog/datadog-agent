// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package attributes

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// recognizedUnits is a subset of unit names recognized by Datadog to fuzz against.
// See https://github.com/DataDog/dogweb/blob/prod/integration/system/units_catalog.csv
var recognizedUnits = map[string]struct{}{
	"ampere":             {},
	"bit":                {},
	"byte":               {},
	"decidegree celsius": {},
	"deciwatt":           {},
	"degree celsius":     {},
	"exabit":             {},
	"exabyte":            {},
	"exbibyte":           {},
	"gibibyte":           {},
	"gigabit":            {},
	"gigabyte":           {},
	"gigahertz":          {},
	"gigawatt":           {},
	"hertz":              {},
	"hour":               {},
	"joule":              {},
	"kibibyte":           {},
	"kilobit":            {},
	"kilobyte":           {},
	"kilohertz":          {},
	"kilojoule":          {},
	"kilowatt":           {},
	"mebibyte":           {},
	"megabit":            {},
	"megabyte":           {},
	"megahertz":          {},
	"megajoule":          {},
	"megawatt":           {},
	"microsecond":        {},
	"microwatt":          {},
	"milliampere":        {},
	"millijoule":         {},
	"millisecond":        {},
	"millivolt":          {},
	"milliwatt":          {},
	"nanojoule":          {},
	"nanosecond":         {},
	"nanowatt":           {},
	"pebibyte":           {},
	"percent":            {},
	"petabit":            {},
	"petabyte":           {},
	"second":             {},
	"tebibyte":           {},
	"terabit":            {},
	"terabyte":           {},
	"terawatt":           {},
	"volt":               {},
	"watt":               {},
}

// FuzzUnitMapperMap checks that Map either fails or returns a valid Datadog unit.
func FuzzUnitMapperMap(f *testing.F) {
	seeds := []string{
		"", "1", "By", "KiBy", "MiBy", "s", "ms", "us", "ns",
		"%", "Cel", "Hz", "MHz", "W", "V", "J",
		"By/s", "GiBy/h", "{cpu}", "{connection}", "kBy", "EiBy",
		"dBy", "ps", "dW", "dCel", "furlong", "By/furlong",
		"{}", "By//s", "By/s/s", "W.h", "By{transmitted}", "{pod}",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	var mapper UnitMapper
	f.Fuzz(func(t *testing.T, unit string) {
		got, ok := mapper.Map(unit)
		if !ok {
			return
		}
		require.NotEmpty(t, got, "Map(%q) returned ok=true but an empty unit", unit)

		for _, component := range strings.Split(got, "/") {
			_, recognized := recognizedUnits[component]
			_, annotation := allowedAnnotations[component]
			require.True(t, recognized || annotation,
				"Map(%q) = %q, but component %q is not a valid unit", unit, got, component)
		}
	})
}
