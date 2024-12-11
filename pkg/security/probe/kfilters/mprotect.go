// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kfilters holds kfilters related files
package kfilters

import (
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

func mprotectKFiltersGetter(approvers rules.Approvers) (ActiveKFilters, []eval.Field, error) {
	var (
		kfilters     []activeKFilter
		fieldHandled []eval.Field
	)

	for field, values := range approvers {
		switch field {
		case "mprotect.vm_protection":
			kfilter, err := getFlagsKFilter("mprotect_vm_protection_approvers", uintValues[uint32](values)...)
			if err != nil {
				return nil, nil, err
			}
			kfilters = append(kfilters, kfilter)
			fieldHandled = append(fieldHandled, field)
		case "mprotect.req_protection":
			kfilter, err := getFlagsKFilter("mprotect_req_protection_approvers", uintValues[uint32](values)...)
			if err != nil {
				return nil, nil, err
			}
			kfilters = append(kfilters, kfilter)
			fieldHandled = append(fieldHandled, field)
		}
	}
	return newActiveKFilters(kfilters...), fieldHandled, nil
}
