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

var openCapabilities = mergeCapabilities(
	rules.FieldCapabilities{
		{
			Field:       "open.flags",
			TypeBitmask: eval.ScalarValueType | eval.BitmaskValueType,
		},
		{
			Field:        "open.file.path",
			TypeBitmask:  eval.ScalarValueType | eval.PatternValueType | eval.GlobValueType,
			ValidateFnc:  validateBasenameFilter,
			FilterWeight: 15,
		},
		{
			Field:        "open.file.name",
			TypeBitmask:  eval.ScalarValueType,
			FilterWeight: 300,
		},
	},
	processCapabilities,
)

func openKFiltersGetter(approvers rules.Approvers) (ActiveKFilters, []eval.Field, error) {
	kfilters, fieldHandled, err := getBasenameKFilters(model.FileOpenEventType, "file", approvers)
	if err != nil {
		return nil, nil, err
	}

	for field, values := range approvers {
		switch field {
		case "open.flags":
			kfilter, err := getFlagsKFilter("open_flags_approvers", uintValues[uint32](values)...)
			if err != nil {
				return nil, nil, err
			}
			kfilters = append(kfilters, kfilter)
			fieldHandled = append(fieldHandled, field)
		}
	}

	kfs, handled, err := getProcessKFilters(model.FileOpenEventType, approvers)
	if err != nil {
		return nil, nil, err
	}
	kfilters = append(kfilters, kfs...)
	fieldHandled = append(fieldHandled, handled...)

	return newActiveKFilters(kfilters...), fieldHandled, nil
}
