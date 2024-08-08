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

var openCapabilities = Capabilities{
	"open.file.path": {
		ValueTypeBitmask: eval.ScalarValueType | eval.PatternValueType | eval.GlobValueType,
		ValidateFnc:      validateBasenameFilter,
		FilterWeight:     15,
	},
	"open.file.name": {
		ValueTypeBitmask: eval.ScalarValueType,
		FilterWeight:     10,
	},
	"open.flags": {
		ValueTypeBitmask: eval.ScalarValueType | eval.BitmaskValueType,
	},
}

func openOnNewApprovers(approvers rules.Approvers) (ActiveKFilters, error) {
	openKFilters, err := getBasenameKFilters(model.FileOpenEventType, "file", approvers)
	if err != nil {
		return nil, err
	}

	for field, values := range approvers {
		switch field {
		case "open.file.name", "open.file.path": // already handled by getBasenameKFilters
		case "open.flags":
			kfilter, err := getFlagsKFilters("open_flags_approvers", intValues[int32](values)...)
			if err != nil {
				return nil, err
			}
			openKFilters = append(openKFilters, kfilter)

		default:
			return nil, fmt.Errorf("unknown field '%s'", field)
		}

	}

	return newActiveKFilters(openKFilters...), nil
}
