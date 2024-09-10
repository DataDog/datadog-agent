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
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var mprotectCapabilities = rules.FieldCapabilities{
	{
		Field:       "mprotect.req_protection",
		TypeBitmask: eval.ScalarValueType | eval.BitmaskValueType,
	},
	{
		Field:       "mprotect.vm_protection",
		TypeBitmask: eval.ScalarValueType | eval.BitmaskValueType,
	},
}

func mprotectKFilters(approvers rules.Approvers) (ActiveKFilters, error) {
	var mprotectKFilters []activeKFilter

	for field, values := range approvers {
		switch field {
		case "mprotect.vm_protection":
			kfilter, err := getFlagsKFilter("mprotect_vm_protection_approvers", uintValues[uint32](values)...)
			if err != nil {
				return nil, err
			}
			mprotectKFilters = append(mprotectKFilters, kfilter)
		case "mprotect.req_protection":
			kfilter, err := getFlagsKFilter("mprotect_req_protection_approvers", uintValues[uint32](values)...)
			if err != nil {
				return nil, err
			}
			mprotectKFilters = append(mprotectKFilters, kfilter)
		default:
			return nil, fmt.Errorf("unknown field '%s'", field)
		}
	}
	return newActiveKFilters(mprotectKFilters...), nil
}
