// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package attributes

import (
	"cmp"
	"maps"
	"slices"
	"strings"
)

// UnitMapper maps UCUM units to their Datadog equivalents.
type UnitMapper struct{}

func NewUnitMapper() *UnitMapper {
	return &UnitMapper{}
}

// Map an OTLP (UCUM) unit to its Datadog equivalent.
// Returns false if no mapping is possible.
//
// The mapping works as follows:
//  1. Rate units are split and each component is mapped separately.
//  2. Annotations are mapped verbatim if they are recognized by Datadog.
//  3. For non-annotations, prefixes are stripped and the base unit is mapped.
//     We validate if the prefix-unit combination is a valid Datadog unit.
func (*UnitMapper) Map(unit string) (string, bool) {
	if unit == "" || unit == "1" {
		// Empty or dimensionless.
		return "", false
	}

	parts := strings.Split(unit, "/")
	if len(parts) > 2 {
		// A Datadog rate is a single fraction; more parts cannot map.
		return "", false
	}
	mapped := make([]string, len(parts))
	for i, part := range parts {
		component, ok := mapUCUMComponent(part)
		if !ok {
			return "", false
		}
		mapped[i] = component
	}
	return strings.Join(mapped, "/"), true
}

// allowedAnnotations are the Datadog unit names accepted as UCUM annotations, e.g. "{alert}".
// See https://ucum.org/ucum#section-Style, 2.3 §12 for annotation definition and syntax.
//
// Currency and other units that can't be expressed easily as an annotation are omitted from the list.
// See https://github.com/DataDog/dogweb/blob/prod/integration/system/units_catalog.csv for the full list.
var allowedAnnotations = map[string]struct{}{
	"alert":             {},
	"apdex":             {},
	"assertion":         {},
	"attempt":           {},
	"block":             {},
	"buffer":            {},
	"build":             {},
	"cent":              {},
	"check":             {},
	"collection":        {},
	"column":            {},
	"command":           {},
	"commit":            {},
	"connection":        {},
	"container":         {},
	"core":              {},
	"cpu":               {},
	"cursor":            {},
	"datagram":          {},
	"day":               {},
	"decibel-milliwatt": {},
	"device":            {},
	"document":          {},
	"dollar":            {},
	"email":             {},
	"entity":            {},
	"entry":             {},
	"error":             {},
	"euro":              {},
	"event":             {},
	"eviction":          {},
	"exception":         {},
	"execution":         {},
	"fault":             {},
	"fetch":             {},
	"file":              {},
	"flush":             {},
	"fraction":          {},
	"frame":             {},
	"get":               {},
	"heap":              {},
	"hit":               {},
	"hop":               {},
	"host":              {},
	"index":             {},
	"inode":             {},
	"instance":          {},
	"invocation":        {},
	"item":              {},
	"job":               {},
	"key":               {},
	"location":          {},
	"lock":              {},
	"merge":             {},
	"message":           {},
	"method":            {},
	"minute":            {},
	"miss":              {},
	"monitor":           {},
	"node":              {},
	"object":            {},
	"occurrence":        {},
	"offset":            {},
	"operation":         {},
	"packet":            {},
	"page":              {},
	"payload":           {},
	"penny":             {},
	"permille":          {},
	"pound":             {},
	"prediction":        {},
	"process":           {},
	"query":             {},
	"question":          {},
	"read":              {},
	"record":            {},
	"refresh":           {},
	"request":           {},
	"resource":          {},
	"response":          {},
	"route":             {},
	"row":               {},
	"run":               {},
	"sample":            {},
	"scan":              {},
	"sector":            {},
	"segment":           {},
	"service":           {},
	"session":           {},
	"set":               {},
	"shard":             {},
	"span":              {},
	"split":             {},
	"stage":             {},
	"step":              {},
	"success":           {},
	"table":             {},
	"task":              {},
	"thread":            {},
	"throttle":          {},
	"ticket":            {},
	"time":              {},
	"timeout":           {},
	"token":             {},
	"transaction":       {},
	"unit":              {},
	"update":            {},
	"user":              {},
	"vector":            {},
	"view":              {},
	"volume":            {},
	"wait":              {},
	"watt-hour":         {},
	"week":              {},
	"worker":            {},
	"write":             {},
}

// mapUCUMComponent to its Datadog equivalent.
func mapUCUMComponent(component string) (string, bool) {
	// Map annotation "{x}" to "x" if it is an allowed annotation.
	if strings.HasPrefix(component, "{") && strings.HasSuffix(component, "}") {
		annotation := component[1 : len(component)-1]
		if _, allowed := allowedAnnotations[annotation]; allowed {
			return annotation, true
		}
		return "", false
	}

	// Otherwise a bare or prefixed base unit.
	return decomposeBaseUnit(component)
}

// ucumPrefixes maps UCUM prefix symbols to their Datadog names.
// Only prefixes supported by Datadog are included.
var ucumPrefixes = map[string]string{
	// https://ucum.org/ucum#section-Prefixes
	"d": "deci",
	"m": "milli",
	"u": "micro",
	"n": "nano",
	"k": "kilo",
	"M": "mega",
	"G": "giga",
	"T": "tera",
	"P": "peta",
	"E": "exa",
	// https://ucum.org/ucum#section-Prefixes-and-Units-Used-in-Information-Technology
	"Ki": "kibi",
	"Mi": "mebi",
	"Gi": "gibi",
	"Ti": "tebi",
	"Pi": "pebi",
	"Ei": "exbi",
}

// ucumPrefixSymbols are the ucumPrefixes keys, longest first, so a binary prefix
// ("Mi") matches before the decimal prefix sharing its first letter ("M").
var ucumPrefixSymbols = sortedPrefixSymbols()

func sortedPrefixSymbols() []string {
	return slices.SortedFunc(maps.Keys(ucumPrefixes), func(a, b string) int {
		return cmp.Or(len(b)-len(a), strings.Compare(a, b))
	})
}

// baseUnit is a UCUM base unit: its Datadog name and the prefixes (from
// ucumPrefixes) that form a real Datadog unit with it.
type baseUnit struct {
	datadogUnitName   string
	supportedPrefixes map[string]struct{}
}

func set[T cmp.Ordered](items ...T) map[T]struct{} {
	set := make(map[T]struct{}, len(items))
	for _, symbol := range items {
		set[symbol] = struct{}{}
	}
	return set
}

// ucumBaseUnits maps each UCUM base-unit symbol to its Datadog name and the
// prefixes that form a real Datadog unit with it.
// See https://github.com/DataDog/dogweb/blob/prod/integration/system/units_catalog.csv for the full list.
var ucumBaseUnits = map[string]baseUnit{
	"By":  {datadogUnitName: "byte", supportedPrefixes: set("Ki", "Mi", "Gi", "Ti", "Pi", "Ei", "k", "M", "G", "T", "P", "E")},
	"bit": {datadogUnitName: "bit", supportedPrefixes: set("k", "M", "G", "T", "P", "E")},
	"s":   {datadogUnitName: "second", supportedPrefixes: set("m", "u", "n")},
	"h":   {datadogUnitName: "hour"},
	"Hz":  {datadogUnitName: "hertz", supportedPrefixes: set("k", "M", "G")},
	"W":   {datadogUnitName: "watt", supportedPrefixes: set("d", "m", "u", "n", "k", "M", "G", "T")},
	"V":   {datadogUnitName: "volt", supportedPrefixes: set("m")},
	"J":   {datadogUnitName: "joule", supportedPrefixes: set("n", "m", "k", "M")},
	"A":   {datadogUnitName: "ampere", supportedPrefixes: set("m")},
	"Cel": {datadogUnitName: "degree celsius", supportedPrefixes: set("d")},
	"%":   {datadogUnitName: "percent"},
}

// decomposeBaseUnit maps a bare base unit ("By") or a prefix+base unit ("KiBy")
// to its Datadog name, rejecting pairs that form no real unit ("mBy").
func decomposeBaseUnit(component string) (string, bool) {
	// Bare base unit.
	if base, ok := ucumBaseUnits[component]; ok {
		return base.datadogUnitName, true
	}

	// Prefix + base unit; the base must list the prefix, longest first.
	for _, prefix := range ucumPrefixSymbols {
		if len(prefix) < len(component) && strings.HasPrefix(component, prefix) {
			base, ok := ucumBaseUnits[component[len(prefix):]]
			if !ok {
				continue
			}
			if _, valid := base.supportedPrefixes[prefix]; !valid {
				continue
			}
			return ucumPrefixes[prefix] + base.datadogUnitName, true
		}
	}

	return "", false
}
