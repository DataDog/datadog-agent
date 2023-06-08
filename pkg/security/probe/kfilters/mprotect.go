// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kfilters

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var mprotectCapabilities = Capabilities{
	"mprotect.req_protection": {
		PolicyFlags:     PolicyFlagFlags,
		FieldValueTypes: eval.ScalarValueType | eval.BitmaskValueType,
	},
	"mprotect.vm_protection": {
		PolicyFlags:     PolicyFlagFlags,
		FieldValueTypes: eval.ScalarValueType | eval.BitmaskValueType,
	},
}

func mprotectOnNewApprovers(approvers rules.Approvers) (ActiveApprovers, error) {
	intValues := func(fvs rules.FilterValues) []int {
		var values []int
		for _, v := range fvs {
			values = append(values, v.Value.(int))
		}
		return values
	}

	var mprotectApprovers []activeApprover

	for field, values := range approvers {
		switch field {
		case "mprotect.vm_protection":
			approver, err := approveFlags("mprotect_vm_protection_approvers", intValues(values)...)
			if err != nil {
				return nil, err
			}
			mprotectApprovers = append(mprotectApprovers, approver)
		case "mprotect.req_protection":
			approver, err := approveFlags("mprotect_req_protection_approvers", intValues(values)...)
			if err != nil {
				return nil, err
			}
			mprotectApprovers = append(mprotectApprovers, approver)

		default:
			return nil, fmt.Errorf("unknown field '%s'", field)
		}
	}
	return newActiveKFilters(mprotectApprovers...), nil
}
