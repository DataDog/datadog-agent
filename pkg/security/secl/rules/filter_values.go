// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// FilterMode defines a filter mode
type FilterMode int

const (
	// DefaultMode enabled approver and discarder
	DefaultMode FilterMode = iota
	// ApproverOnlyMode not used to generate a discarder
	ApproverOnlyMode
)

// FilterValues is a list of FilterValue
type FilterValues []FilterValue

// FilterValue is an approver of a field: the comparison it approves, its value
// and its type, and the test the kernel approves events on.
type FilterValue struct {
	Field eval.Field          `json:"field"`
	Value interface{}         `json:"value"`
	Type  eval.FieldValueType `json:"type"`
	Mode  FilterMode          `json:"mode"`

	Approver Approver `json:"-"`
}

// Merge merges to FilterValues ensuring there is no duplicate value
func (fv FilterValues) Merge(n ...FilterValue) FilterValues {
LOOP:
	for _, v1 := range n {
		for _, v2 := range fv {
			if v1.Field == v2.Field && v1.Value == v2.Value && v1.Approver == v2.Approver {
				continue LOOP
			}
		}
		fv = append(fv, v1)
	}

	return fv
}

// Weight returns the weight of the approver of the value
func (fv FilterValue) Weight() int {
	return fv.Approver.Weight()
}

// RangeFilterValue defines a range value
type RangeFilterValue struct {
	Min int
	Max int
}
