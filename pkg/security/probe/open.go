// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"path"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/pkg/errors"
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
	stringValues := func(fvs rules.FilterValues) []string {
		var values []string
		for _, v := range fvs {
			values = append(values, v.Value.(string))
		}
		return values
	}

	intValues := func(fvs rules.FilterValues) []int {
		var values []int
		for _, v := range fvs {
			values = append(values, v.Value.(int))
		}
		return values
	}

	var openApprovers []activeApprover
	for field, values := range approvers {
		switch field {
		case "open.basename":
			activeApprovers, err := approveBasenames("open_basename_approvers", stringValues(values)...)
			if err != nil {
				return nil, err
			}
			openApprovers = append(openApprovers, activeApprovers...)

		case "open.filename":
			for _, value := range stringValues(values) {
				basename := path.Base(value)
				activeApprover, err := approveBasename("open_basename_approvers", basename)
				if err != nil {
					return nil, err
				}
				openApprovers = append(openApprovers, activeApprover)
			}

		case "open.flags":
			activeApprover, err := approveFlags("open_flags_approvers", intValues(values)...)
			if err != nil {
				return nil, err
			}
			openApprovers = append(openApprovers, activeApprover)

		default:
			return nil, errors.New("field unknown")
		}

	}

	return newActiveKFilters(openApprovers...), nil
}
