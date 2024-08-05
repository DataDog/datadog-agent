// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kfilters holds kfilters related files
package kfilters

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var spliceCapabilities = Capabilities{
	"splice.file.path": {
		ValueTypeBitmask: eval.ScalarValueType | eval.PatternValueType,
		ValidateFnc:      validateBasenameFilter,
	},
	"splice.file.name": {
		ValueTypeBitmask: eval.ScalarValueType,
	},
	"splice.pipe_entry_flag": {
		ValueTypeBitmask: eval.ScalarValueType | eval.BitmaskValueType,
	},
	"splice.pipe_exit_flag": {
		ValueTypeBitmask: eval.ScalarValueType | eval.BitmaskValueType,
	},
}

func spliceKFilters(approvers rules.Approvers) (ActiveKFilters, error) {
	spliceKFilters, err := getBasenameKFilters(model.SpliceEventType, "file", approvers)
	if err != nil {
		return nil, err
	}

	for field, values := range approvers {
		switch field {
		case "splice.file.name", "splice.file.path": // already handled by getBasenameKFilters
		case "splice.pipe_entry_flag":
			kfilter, err := getFlagsKFilters("splice_entry_flags_approvers", intValues[int32](values)...)
			if err != nil {
				return nil, err
			}
			spliceKFilters = append(spliceKFilters, kfilter)
		case "splice.pipe_exit_flag":
			kfilter, err := getFlagsKFilters("splice_exit_flags_approvers", intValues[int32](values)...)
			if err != nil {
				return nil, err
			}
			spliceKFilters = append(spliceKFilters, kfilter)
		default:
			return nil, fmt.Errorf("unknown field '%s'", field)
		}
	}
	return newActiveKFilters(spliceKFilters...), nil
}
