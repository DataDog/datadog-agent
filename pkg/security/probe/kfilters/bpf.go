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

var bpfCapabilities = Capabilities{
	"bpf.cmd": {
		ValueTypeBitmask: eval.ScalarValueType | eval.BitmaskValueType,
	},
}

func bpfKFilters(approvers rules.Approvers) (ActiveKFilters, error) {
	var bpfKFilters []activeKFilter

	for field, values := range approvers {
		switch field {
		case "bpf.cmd":
			kfilter, err := getEnumsKFilters("bpf_cmd_approvers", intValues[int64](values)...)
			if err != nil {
				return nil, err
			}
			bpfKFilters = append(bpfKFilters, kfilter)
		default:
			return nil, fmt.Errorf("unknown field '%s'", field)
		}
	}
	return newActiveKFilters(bpfKFilters...), nil
}
