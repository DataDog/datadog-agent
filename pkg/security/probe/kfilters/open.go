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

var openCapabilities = Capabilities{
	"open.file.path": {
		PolicyFlags:     PolicyFlagBasename,
		FieldValueTypes: eval.ScalarValueType | eval.PatternValueType | eval.GlobValueType,
		ValidateFnc:     validateBasenameFilter,
		FilterWeight:    15,
	},
	"open.file.name": {
		PolicyFlags:     PolicyFlagBasename,
		FieldValueTypes: eval.ScalarValueType,
		FilterWeight:    10,
	},
	"open.flags": {
		PolicyFlags:     PolicyFlagFlags,
		FieldValueTypes: eval.ScalarValueType | eval.BitmaskValueType,
	},
}

func openOnNewApprovers(approvers rules.Approvers) (ActiveApprovers, error) {
	intValues := func(fvs rules.FilterValues) []int {
		var values []int
		for _, v := range fvs {
			values = append(values, v.Value.(int))
		}
		return values
	}

	openApprovers, err := onNewBasenameApprovers(model.FileOpenEventType, "file", approvers)
	if err != nil {
		return nil, err
	}

	for field, values := range approvers {
		switch field {
		case "open.file.name", "open.file.path": // already handled by onNewBasenameApprovers
		case "open.flags":
			activeApprover, err := approveFlags("open_flags_approvers", intValues(values)...)
			if err != nil {
				return nil, err
			}
			openApprovers = append(openApprovers, activeApprover)

		default:
			return nil, fmt.Errorf("unknown field '%s'", field)
		}

	}

	return newActiveKFilters(openApprovers...), nil
}
