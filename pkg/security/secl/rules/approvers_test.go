// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package rules holds rules related files
package rules

import (
	"math"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

func TestApproversOfExpressions(t *testing.T) {
	caps := testModel(
		testCap{
			Field:  "open.file.path",
			Types:  eval.ScalarValueType | eval.GlobValueType,
			Weight: 300,
		},
		testCap{
			Field:  "open.flags",
			Types:  eval.ScalarValueType | eval.BitmaskValueType,
			Weight: 100,
		},
		testCap{
			Field:  "process.auid",
			Types:  eval.ScalarValueType | eval.RangeValueType,
			Range:  &RangeFilterValue{Min: 0, Max: math.MaxUint32 - 1},
			Weight: 10,
		},
	)

	// a nil map is no approvers: the rule has none
	for expr, expected := range map[string]map[eval.Field][]interface{}{
		`open.file.path == "/a"`:             {"open.file.path": {"/a"}},
		`"/a" == open.file.path`:             {"open.file.path": {"/a"}},
		`open.file.path in ["/a", ~"/b/*"]`:  {"open.file.path": {"/a", "/b/*"}},
		`open.file.path not in ["/a", "/b"]`: nil,

		// a disjunction needs approvers on both sides, on any field
		`open.file.path == "/a" || open.flags & O_CREAT > 0`:                         {"open.file.path": {"/a"}, "open.flags": {syscall.O_CREAT}},
		`open.file.path == "/a" || process.uid == 0`:                                 nil,
		`open.file.path == "/a" || (open.file.path != "" && open.file.path != "/x")`: nil,

		// a conjunction needs approvers on one side, the most selective one
		`process.uid == 0 && open.file.path == "/a"`:                                       {"open.file.path": {"/a"}},
		`(open.file.path == "/a" || open.file.path == "/b") && open.flags & O_CREAT > 0`:   {"open.file.path": {"/a", "/b"}},
		`(open.file.path == "/a" || open.flags & O_CREAT > 0) && open.flags & O_EXCL > 0`:  {"open.flags": {syscall.O_EXCL}},
		`(open.file.path == "/a" && process.uid == 0) || (open.file.path == "/b" && true)`: {"open.file.path": {"/a", "/b"}},

		// negations
		`!(open.file.path != "/a")`:                        {"open.file.path": {"/a"}},
		`!(open.file.path == "/a")`:                        nil,
		`!(open.file.path != "/a" || process.uid == 0)`:    {"open.file.path": {"/a"}},
		`!(open.file.path not in ["/a", "/b"])`:            {"open.file.path": {"/a", "/b"}},
		`!(process.auid < 1000) && open.file.path != "/a"`: nil,

		// integers and bitmasks
		`open.file.path != "" && process.auid >= 1000`: {"process.auid": {RangeFilterValue{Min: 1000, Max: math.MaxUint32 - 1}}},
		`open.file.path != "" && 1000 > process.auid`:  {"process.auid": {RangeFilterValue{Min: 0, Max: 999}}},
		`open.flags & (O_CREAT | O_EXCL) == O_CREAT`:   {"open.flags": {syscall.O_CREAT}},
		`open.flags & O_CREAT != 0`:                    {"open.flags": {syscall.O_CREAT}},
		`open.flags & O_CREAT == 0`:                    nil,
		`open.flags & O_CREAT > 0 && open.flags > 2`:   {"open.flags": {syscall.O_CREAT}},
		// never true, no event to approve
		`open.flags & O_CREAT == O_EXCL`: {},
	} {
		t.Run(expr, func(t *testing.T) {
			rs := newRuleSet()
			AddTestRuleExpr(t, rs, expr)

			approvers, _, _, err := rs.GetEventTypeApprovers("open", caps)
			require.NoError(t, err)

			if expected == nil {
				require.Nil(t, approvers)
				return
			}

			values := make(map[eval.Field][]interface{})
			for field, fieldValues := range approvers {
				for _, value := range fieldValues {
					values[field] = append(values[field], value.Value)
				}
			}
			require.Equal(t, expected, values)
		})
	}
}

// A rule is excluded from the discarders only if all its approvers are in
// ApproverOnly mode: the events approved by another approver aren't immune to
// the discarders.
func TestApproverOnlyRules(t *testing.T) {
	caps := testModel(
		testCap{
			Field:  "open.file.path",
			Types:  eval.ScalarValueType,
			Weight: 300,
		},
		testCap{
			Field:        "process.auid",
			Types:        eval.ScalarValueType,
			ApproverOnly: true,
			Weight:       10,
		},
	)

	for expr, excluded := range map[string]bool{
		`open.file.path != "" && process.auid == 1000`:                           true,
		`open.file.path != "" && (process.auid == 1000 || process.auid == 1001)`: true,
		`open.file.path == "/a" || process.auid == 1000`:                         false,
		`open.file.path == "/a"`:                                                 false,
	} {
		t.Run(expr, func(t *testing.T) {
			rs := newRuleSet()
			AddTestRuleExpr(t, rs, expr)

			_, _, noDiscarderRules, err := rs.GetEventTypeApprovers("open", caps)
			require.NoError(t, err)
			require.Equal(t, excluded, len(noDiscarderRules) == 1)
		})
	}
}

// testCap approves the comparisons of a field reported as one of some types, on
// their reported value.
type testCap struct {
	Field        eval.Field
	Types        eval.FieldValueType
	Weight       int
	ApproverOnly bool
	// Range bounds the ranges of values
	Range    *RangeFilterValue
	Validate func(value interface{}) bool
}

type testApprover struct {
	value        interface{}
	weight       int
	approverOnly bool
}

func (a testApprover) Weight() int        { return a.weight }
func (a testApprover) ApproverOnly() bool { return a.approverOnly }

// testRangeApprover approves a range of values, bounded by the capability
type testRangeApprover struct {
	testApprover
}

func (a testRangeApprover) Report() (eval.FieldValueType, interface{}) {
	return eval.RangeValueType, a.value
}

// testModel approves the comparisons of the fields of some capabilities. It
// has no approvers for the negations and for the bits of a field none of which
// is set.
func testModel(caps ...testCap) ApproverModel {
	return ApproverModel(func(c Comparison) ([]Approver, bool) {
		if _, ok := c.AnyBits(); c.Op == ast.ScalarNotEqual && !ok || c.Op == ast.ScalarNotMatch || c.masked() && !ok {
			return nil, false
		}
		valueType, value := c.report()
		if valueType == eval.GlobValueType {
			valueType |= eval.PatternValueType
		}
		for _, tc := range caps {
			if tc.Field != c.Field || valueType&tc.Types == 0 || tc.Validate != nil && !tc.Validate(value) {
				continue
			}
			approver := testApprover{value: value, weight: tc.Weight, approverOnly: tc.ApproverOnly}
			if r, ok := value.(RangeFilterValue); ok && tc.Range != nil {
				approver.value = RangeFilterValue{Min: max(r.Min, tc.Range.Min), Max: min(r.Max, tc.Range.Max)}
				return []Approver{testRangeApprover{approver}}, true
			}
			return []Approver{approver}, true
		}
		return nil, false
	})
}
