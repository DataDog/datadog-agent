// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import "github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"

// FilterValues is a list of FilterValue
type FilterValues []FilterValue

// FilterValue represents a field, its value, its type and whether it's a used
// to compare with or against its value
type FilterValue struct {
	Field eval.Field
	Value interface{}
	Type  eval.FieldValueType

	// indicate process.uid == process.gid for example
	isScalar bool
	// opposite value of the field Value
	notValue interface{}
	not      bool
}

// Merge merges to FilterValues ensuring there is no duplicate value
func (fv FilterValues) Merge(n ...FilterValue) FilterValues {
LOOP:
	for _, v1 := range n {
		for _, v2 := range fv {
			if v1.Field == v2.Field && v1.Value == v2.Value && v1.not == v2.not {
				continue LOOP
			}
		}
		fv = append(fv, v1)
	}

	return fv
}
