// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kfilters holds kfilters related files
package kfilters

import (
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var openFlagsCapabilities = rules.FieldCapabilities{
	{
		Field:        "open.flags",
		TypeBitmask:  eval.ScalarValueType | eval.BitmaskValueType,
		FilterWeight: 100,
	},
	{
		Field:        "open.file.in_upper_layer",
		TypeBitmask:  eval.ScalarValueType,
		FilterWeight: 50,
	},
}

func openKFiltersGetter(approvers rules.Approvers) (KFilters, []eval.Field, error) {
	kfilters, fieldHandled, err := getBasenameKFilters(model.FileOpenEventType, "file", approvers)
	if err != nil {
		return nil, nil, err
	}

	for field, values := range approvers {
		switch field {
		case "open.flags":
			var hasRdOnlyScalarApprover bool
			otherFlagsFieldValues := make(rules.FilterValues, 0, len(values))
			for _, value := range values {
				if intValue, ok := value.Value.(int); ok && intValue == unix.O_RDONLY && value.Type == eval.ScalarValueType {
					hasRdOnlyScalarApprover = true
				} else {
					otherFlagsFieldValues = append(otherFlagsFieldValues, value)
				}
			}

			// we handle the O_RDONLY approver separately because (open.flags & O_RDONLY) will always be false
			// this flag value should only be set for ruleset containing rules matching O_RDONLY value specifically
			// i.e. `(open.flags & O_ACCMODE) == O_RDONLY` or `open.flags == O_RDONLY`
			kfilter, err := newKFilterZeroFlagValue("open_flags_rdonly_approver", hasRdOnlyScalarApprover)
			if err != nil {
				return nil, nil, err
			}
			kfilters = append(kfilters, kfilter)

			// this case handle other flags
			// i.e. a rule with `open.flags & O_CREAT > 0` shouldn't set the open_flags_rdonly_approver but only push
			// an approver for the O_CREAT flag.
			if len(otherFlagsFieldValues) > 0 {
				kfilter, err = getFlagsKFilter("open_flags_approvers", uintValues[uint32](otherFlagsFieldValues)...)
				if err != nil {
					return nil, nil, err
				}
				kfilters = append(kfilters, kfilter)
			}

			fieldHandled = append(fieldHandled, field)
		case "open.file.in_upper_layer":
			activeKFilter, err := newInUpperLayerKFilter(InUpperLayerApproverKernelMapName, model.FileOpenEventType)
			if err != nil {
				return nil, nil, err
			}
			kfilters = append(kfilters, activeKFilter)
			fieldHandled = append(fieldHandled, field)
		}
	}

	kfs, handled, err := getProcessKFilters(model.FileOpenEventType, approvers)
	if err != nil {
		return nil, nil, err
	}
	kfilters = append(kfilters, kfs...)
	fieldHandled = append(fieldHandled, handled...)

	return newKFilters(kfilters...), fieldHandled, nil
}
