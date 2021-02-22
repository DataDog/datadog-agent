// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

var openCapabilities = Capabilities{
	"open.filename": {
		PolicyFlags:     PolicyFlagBasename,
		FieldValueTypes: eval.ScalarValueType,
	},
	"open.basename": {
		PolicyFlags:     PolicyFlagBasename,
		FieldValueTypes: eval.ScalarValueType,
	},
	"open.flags": {
		PolicyFlags:     PolicyFlagFlags,
		FieldValueTypes: eval.ScalarValueType | eval.BitmaskValueType,
	},
}

func openOnNewApprovers(probe *Probe, approvers rules.Approvers) (activeApprovers, error) {
	intValues := func(fvs rules.FilterValues) []int {
		var values []int
		for _, v := range fvs {
			values = append(values, v.Value.(int))
		}
		return values
	}

	openApprovers, err := onNewBasenameApprovers(probe, model.FileOpenEventType, "", approvers)
	if err != nil {
		return nil, err
	}

	for field, values := range approvers {
		switch field {
		case "open.basename", "open.filename": // already handled by onNewBasenameApprovers
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
