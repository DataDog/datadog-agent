// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kfilters

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var mmapCapabilities = Capabilities{
	"mmap.file.path": {
		PolicyFlags:     PolicyFlagBasename,
		FieldValueTypes: eval.ScalarValueType | eval.PatternValueType,
		ValidateFnc:     validateBasenameFilter,
	},
	"mmap.file.name": {
		PolicyFlags:     PolicyFlagBasename,
		FieldValueTypes: eval.ScalarValueType,
	},
	"mmap.protection": {
		PolicyFlags:     PolicyFlagFlags,
		FieldValueTypes: eval.ScalarValueType | eval.BitmaskValueType,
	},
	"mmap.flags": {
		PolicyFlags:     PolicyFlagFlags,
		FieldValueTypes: eval.ScalarValueType | eval.BitmaskValueType,
	},
}

func mmapOnNewApprovers(approvers rules.Approvers) (ActiveApprovers, error) {
	intValues := func(fvs rules.FilterValues) []int {
		var values []int
		for _, v := range fvs {
			values = append(values, v.Value.(int))
		}
		return values
	}

	mmapApprovers, err := onNewBasenameApprovers(model.MMapEventType, "file", approvers)
	if err != nil {
		return nil, err
	}

	for field, values := range approvers {
		switch field {
		case "mmap.file.name", "mmap.file.path": // already handled by onNewBasenameApprovers
		case "mmap.flags":
			var approver activeApprover
			approver, err = approveFlags("mmap_flags_approvers", intValues(values)...)
			if err != nil {
				return nil, err
			}
			mmapApprovers = append(mmapApprovers, approver)
		case "mmap.protection":
			var approver activeApprover
			approver, err = approveFlags("mmap_protection_approvers", intValues(values)...)
			if err != nil {
				return nil, err
			}
			mmapApprovers = append(mmapApprovers, approver)

		default:
			return nil, fmt.Errorf("unknown field '%s'", field)
		}
	}
	return newActiveKFilters(mmapApprovers...), nil
}
