// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collapse_selects

import (
	"sort"
	"testing"

	bzl "github.com/bazelbuild/buildtools/build"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers -----------------------------------------------------------------

// strList builds a *bzl.ListExpr from string values (e.g. dep labels).
func strList(items ...string) *bzl.ListExpr {
	exprs := make([]bzl.Expr, len(items))
	for i, s := range items {
		exprs[i] = &bzl.StringExpr{Value: s}
	}
	return &bzl.ListExpr{List: exprs}
}

// buildDict builds a DictExpr from (condition, list-of-values) pairs. Pair
// order is preserved so tests can control the entry order.
func buildDict(pairs ...interface{}) *bzl.DictExpr {
	if len(pairs)%2 != 0 {
		panic("buildDict: odd number of args")
	}
	var kvs []*bzl.KeyValueExpr
	for i := 0; i < len(pairs); i += 2 {
		cond := pairs[i].(string)
		var val bzl.Expr
		switch v := pairs[i+1].(type) {
		case []string:
			val = strList(v...)
		case *bzl.ListExpr:
			val = v
		default:
			panic("buildDict: unsupported value type")
		}
		kvs = append(kvs, &bzl.KeyValueExpr{
			Key:   &bzl.StringExpr{Value: cond},
			Value: val,
		})
	}
	return &bzl.DictExpr{List: kvs}
}

// buildSelect wraps a DictExpr in a select() call.
func buildSelect(dict *bzl.DictExpr) *bzl.CallExpr {
	return &bzl.CallExpr{
		X:    &bzl.Ident{Name: "select"},
		List: []bzl.Expr{dict},
	}
}

// dictConditions extracts the ordered list of condition keys from a dict.
func dictConditions(dict *bzl.DictExpr) []string {
	conds := make([]string, len(dict.List))
	for i, kv := range dict.List {
		conds[i] = kv.Key.(*bzl.StringExpr).Value
	}
	return conds
}

// dictValueKeys extracts a map[condition]valueKey from a dict.
func dictValueKeys(dict *bzl.DictExpr) map[string]string {
	m := make(map[string]string, len(dict.List))
	for _, kv := range dict.List {
		cond := kv.Key.(*bzl.StringExpr).Value
		m[cond] = exprKey(kv.Value)
	}
	return m
}

// allPlatformsExcept returns all knownOSPlatforms except the named ones.
func allPlatformsExcept(excluded ...string) []string {
	excl := make(map[string]struct{}, len(excluded))
	for _, e := range excluded {
		excl[e] = struct{}{}
	}
	var out []string
	for _, p := range knownOSPlatforms {
		if _, skip := excl[p]; !skip {
			out = append(out, p)
		}
	}
	return out
}

// nPlatforms returns the first n platforms from knownOSPlatforms.
func nPlatforms(n int) []string {
	return knownOSPlatforms[:n]
}

// --- exprKey -----------------------------------------------------------------

func TestExprKey(t *testing.T) {
	tests := []struct {
		name string
		a, b bzl.Expr
		same bool
	}{
		{
			name: "empty lists are equal",
			a:    strList(),
			b:    strList(),
			same: true,
		},
		{
			name: "same single-item lists",
			a:    strList("@foo"),
			b:    strList("@foo"),
			same: true,
		},
		{
			name: "order-independent list comparison",
			a:    strList("@foo", "@bar"),
			b:    strList("@bar", "@foo"),
			same: true,
		},
		{
			name: "different lists",
			a:    strList("@foo"),
			b:    strList("@bar"),
			same: false,
		},
		{
			name: "empty vs non-empty",
			a:    strList(),
			b:    strList("@foo"),
			same: false,
		},
		{
			name: "superset vs subset",
			a:    strList("@foo", "@bar", "@baz"),
			b:    strList("@foo", "@bar"),
			same: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ka, kb := exprKey(tt.a), exprKey(tt.b)
			if tt.same {
				assert.Equal(t, ka, kb)
			} else {
				assert.NotEqual(t, ka, kb)
			}
		})
	}
}

// --- extractOSPlatform -------------------------------------------------------

func TestExtractOSPlatform(t *testing.T) {
	for _, platform := range knownOSPlatforms {
		t.Run("valid/"+platform, func(t *testing.T) {
			got, ok := extractOSPlatform(rulesGoPrefix + platform)
			assert.True(t, ok)
			assert.Equal(t, platform, got)
		})
	}

	invalid := []struct {
		name      string
		condition string
	}{
		{"arch-qualified", rulesGoPrefix + "linux_amd64"},
		{"wrong prefix", "@platforms//os:linux"},
		{"unknown platform", rulesGoPrefix + "haiku"},
		{"empty", ""},
		{"just prefix", rulesGoPrefix},
	}
	for _, tt := range invalid {
		t.Run("invalid/"+tt.name, func(t *testing.T) {
			_, ok := extractOSPlatform(tt.condition)
			assert.False(t, ok)
		})
	}
}

// --- collapseDict ------------------------------------------------------------

const unixPkg = "@org_golang_x_sys//unix"

// osKey builds a condition string for a rules_go OS platform.
func osKey(platform string) string {
	return rulesGoPrefix + platform
}

func TestCollapseDictInversion(t *testing.T) {
	// !windows: 15 platforms have [unix], default is [].
	// Expected: invert to {windows: [], //conditions:default: [unix]}.
	var args []interface{}
	for _, p := range allPlatformsExcept("windows") {
		args = append(args, osKey(p), []string{unixPkg})
	}
	args = append(args, condDefault, []string{})
	dict := buildDict(args...)

	result := collapseDict(dict)
	require.NotNil(t, result)

	assert.Equal(t, map[string]string{
		osKey("windows"): exprKey(strList()),
		condDefault:      exprKey(strList(unixPkg)),
	}, dictValueKeys(result))
}

func TestCollapseDictAllMatchDefault(t *testing.T) {
	// All explicit entries have the same value as the default.
	// Expected: only the default entry survives.
	dict := buildDict(
		osKey("linux"), []string{unixPkg},
		osKey("darwin"), []string{unixPkg},
		osKey("windows"), []string{unixPkg},
		condDefault, []string{unixPkg},
	)

	result := collapseDict(dict)
	require.NotNil(t, result)

	assert.Equal(t, []string{condDefault}, dictConditions(result))
	assert.Equal(t, exprKey(strList(unixPkg)), exprKey(result.List[0].Value))
}

func TestCollapseDictRemoveRedundantEntries(t *testing.T) {
	// Some explicit entries match the default (redundant), others don't.
	// Expected: keep only entries that differ from default.
	dict := buildDict(
		osKey("linux"), []string{unixPkg}, // different from default
		osKey("darwin"), []string{unixPkg}, // different from default
		osKey("windows"), []string{}, // same as default → redundant
		condDefault, []string{},
	)

	result := collapseDict(dict)
	require.NotNil(t, result)

	got := dictValueKeys(result)
	assert.Contains(t, got, osKey("linux"))
	assert.Contains(t, got, osKey("darwin"))
	assert.Contains(t, got, condDefault)
	assert.NotContains(t, got, osKey("windows"), "windows was redundant with default")
}

func TestCollapseDictNoInversionWhenNotBeneficial(t *testing.T) {
	// Single explicit entry differs from default: 15 platforms missing.
	// Inversion would create 15 entries — worse than current 2.
	dict := buildDict(
		osKey("linux"), []string{unixPkg},
		condDefault, []string{},
	)

	result := collapseDict(dict)
	assert.Nil(t, result, "inversion creates more entries than original")
}

func TestCollapseDictEvenSplitNoInversion(t *testing.T) {
	// Half explicit, half missing: inversion produces the same number of entries, so it provides no benefit.
	platforms := nPlatforms(len(knownOSPlatforms) / 2)
	var args []interface{}
	for _, p := range platforms {
		args = append(args, osKey(p), []string{unixPkg})
	}
	args = append(args, condDefault, []string{})
	dict := buildDict(args...)

	result := collapseDict(dict)
	assert.Nil(t, result, "even split: inversion does not reduce entry count")
}

func TestCollapseDictInversionBeneficial(t *testing.T) {
	// Use 1/4 of platforms as the "missing" minority so inversion clearly wins.
	numMissing := len(knownOSPlatforms) / 4
	numExplicit := len(knownOSPlatforms) - numMissing
	platforms := nPlatforms(numExplicit)
	var args []interface{}
	for _, p := range platforms {
		args = append(args, osKey(p), []string{unixPkg})
	}
	args = append(args, condDefault, []string{})
	dict := buildDict(args...)

	result := collapseDict(dict)
	require.NotNil(t, result)

	// Result has numMissing explicit entries plus one //conditions:default.
	assert.Equal(t, numMissing+1, len(result.List))

	got := dictValueKeys(result)
	assert.Equal(t, exprKey(strList(unixPkg)), got[condDefault])

	// All missing platforms are present with the old default value.
	missing := make(map[string]struct{})
	for _, p := range knownOSPlatforms[numExplicit:] {
		missing[osKey(p)] = struct{}{}
	}
	for cond, vk := range got {
		if cond == condDefault {
			continue
		}
		assert.Contains(t, missing, cond, "unexpected condition in result: %s", cond)
		assert.Equal(t, exprKey(strList()), vk)
	}
}

func TestCollapseDictMixedValuesNoChange(t *testing.T) {
	// Non-default entries have different values and neither matches the default.
	dict := buildDict(
		osKey("linux"), []string{"@foo"},
		osKey("darwin"), []string{"@bar"},
		condDefault, []string{},
	)

	result := collapseDict(dict)
	assert.Nil(t, result)
}

func TestCollapseDictNoDefaultNoChange(t *testing.T) {
	dict := buildDict(
		osKey("linux"), []string{unixPkg},
		osKey("darwin"), []string{unixPkg},
	)

	result := collapseDict(dict)
	assert.Nil(t, result)
}

func TestCollapseDictOnlyDefaultNoChange(t *testing.T) {
	dict := buildDict(condDefault, []string{unixPkg})

	result := collapseDict(dict)
	assert.Nil(t, result)
}

func TestCollapseDictNonOSConditionSkipsInversion(t *testing.T) {
	// Mixed @platforms//os:* and @rules_go//go/platform:* — inversion is unsafe.
	// The partial-match removal should still apply if some entries match default.
	dict := buildDict(
		"@platforms//os:linux", []string{unixPkg}, // non-rules_go prefix
		osKey("darwin"), []string{unixPkg},
		condDefault, []string{unixPkg}, // matches both entries
	)

	result := collapseDict(dict)
	require.NotNil(t, result, "matching entries should be removed")

	// Both explicit entries matched default and should be removed.
	assert.Equal(t, []string{condDefault}, dictConditions(result))
}

// --- collapseExpr ------------------------------------------------------------

func TestCollapseExprBareSelect(t *testing.T) {
	// A bare select() is collapsed.
	var args []interface{}
	for _, p := range allPlatformsExcept("windows") {
		args = append(args, osKey(p), []string{unixPkg})
	}
	args = append(args, condDefault, []string{})
	sel := buildSelect(buildDict(args...))

	result := collapseExpr(sel)
	require.NotNil(t, result)

	call, ok := result.(*bzl.CallExpr)
	require.True(t, ok)
	dict := call.List[0].(*bzl.DictExpr)
	assert.Len(t, dict.List, 2)
}

func TestCollapseExprListPlusSelect(t *testing.T) {
	// [static] + select({...}) collapses only the select part.
	var args []interface{}
	for _, p := range allPlatformsExcept("windows") {
		args = append(args, osKey(p), []string{unixPkg})
	}
	args = append(args, condDefault, []string{})

	base := strList("//pkg/base")
	sel := buildSelect(buildDict(args...))
	expr := &bzl.BinaryExpr{X: base, Op: "+", Y: sel}

	result := collapseExpr(expr)
	require.NotNil(t, result)

	binExpr, ok := result.(*bzl.BinaryExpr)
	require.True(t, ok)
	assert.Equal(t, base, binExpr.X, "static list must be unchanged")

	call, ok := binExpr.Y.(*bzl.CallExpr)
	require.True(t, ok)
	dict := call.List[0].(*bzl.DictExpr)
	assert.Len(t, dict.List, 2)
}

func TestCollapseExprAlreadyCollapsedIsNoop(t *testing.T) {
	// Regression: collapseExpr must return an untyped nil (not a
	// (*bzl.CallExpr)(nil) wrapped in a non-nil bzl.Expr interface) when there
	// is nothing to collapse. The original bug caused a nil-pointer panic in
	// FixLoads on the second Gazelle run.
	sel := buildSelect(buildDict(
		osKey("windows"), []string{},
		condDefault, []string{unixPkg},
	))

	result := collapseExpr(sel)

	assert.Nil(t, result, "already-collapsed select must not be modified")
	var zero bzl.Expr
	assert.Equal(t, zero, result, "result must be an untyped nil, not a typed nil pointer")
}

// --- sorting of missing-platform entries -------------------------------------

func TestInvertedSelectMissingPlatformsSorted(t *testing.T) {
	// Missing platforms in the inverted select should appear in sorted order,
	// giving deterministic BUILD file output across runs.
	numMissing := len(knownOSPlatforms) / 4
	var args []interface{}
	for _, p := range knownOSPlatforms[numMissing:] {
		args = append(args, osKey(p), []string{unixPkg})
	}
	args = append(args, condDefault, []string{})
	dict := buildDict(args...)

	result := collapseDict(dict)
	require.NotNil(t, result)

	var conditions []string
	for _, kv := range result.List {
		cond := kv.Key.(*bzl.StringExpr).Value
		if cond != condDefault {
			conditions = append(conditions, cond)
		}
	}

	sorted := append([]string(nil), conditions...)
	sort.Strings(sorted)
	assert.Equal(t, sorted, conditions, "missing-platform entries must be in sorted order")
}
