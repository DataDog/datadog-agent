// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kfilters holds kfilters related files
package kfilters

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var spliceCapabilities = rules.FieldCapabilities{
	{
		Field:       "splice.pipe_entry_flag",
		TypeBitmask: eval.ScalarValueType | eval.BitmaskValueType,
	},
	{
		Field:       "splice.pipe_exit_flag",
		TypeBitmask: eval.ScalarValueType | eval.BitmaskValueType,
	},
}

func spliceKFiltersGetter(approvers rules.Approvers) (KFilters, []eval.Field, error) {
	kfilters, fieldHandled, err := getBasenameKFilters(model.SpliceEventType, "file", approvers)
	if err != nil {
		return nil, nil, err
	}

	for field, values := range approvers {
		switch field {
		case "splice.pipe_entry_flag":
			kfilter, err := getFlagsKFilter("splice_entry_flags_approvers", uintValues[uint32](values)...)
			if err != nil {
				return nil, nil, err
			}
			kfilters = append(kfilters, kfilter)
			fieldHandled = append(fieldHandled, field)
		case "splice.pipe_exit_flag":
			kfilter, err := getFlagsKFilter("splice_exit_flags_approvers", uintValues[uint32](values)...)
			if err != nil {
				return nil, nil, err
			}
			kfilters = append(kfilters, kfilter)
			fieldHandled = append(fieldHandled, field)
		}
	}
	return newKFilters(kfilters...), fieldHandled, nil
}
