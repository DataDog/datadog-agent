// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kfilters holds kfilters related files
package kfilters

import (
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

const (
	auidField               = "process.auid"
	maxAUID                 = model.AuditUIDUnset - 1
	auidApproversTable      = "auid_approvers"
	auidRangeApproversTable = "auid_range_approvers"
)

var processCapabilities = rules.FieldCapabilities{
	{
		Field:            "process.auid",
		TypeBitmask:      eval.ScalarValueType | eval.RangeValueType,
		FilterMode:       rules.ApproverOnlyMode,
		RangeFilterValue: &rules.RangeFilterValue{Min: 0, Max: maxAUID},
		FilterWeight:     100,
		// convert  `!= model.AuditUIDUnset`` to the max range
		HandleNotApproverValue: func(fieldValueType eval.FieldValueType, value interface{}) (eval.FieldValueType, interface{}, bool) {
			if fieldValueType != eval.ScalarValueType {
				return fieldValueType, value, false
			}

			if i, ok := value.(int); ok && uint32(i) == model.AuditUIDUnset {
				return eval.RangeValueType, rules.RangeFilterValue{Min: 0, Max: model.AuditUIDUnset - 1}, true
			}

			return fieldValueType, value, false
		},
	},
}

func getProcessKFilters(eventType model.EventType, approvers rules.Approvers) ([]activeKFilter, []eval.Field, error) {
	var fieldHandled []eval.Field

	values, exists := approvers[auidField]
	if !exists {
		return nil, nil, nil
	}

	var (
		kfilters     []activeKFilter
		auidRange    = rules.RangeFilterValue{Min: 0, Max: maxAUID}
		auidRangeSet bool
	)

	for _, value := range values {
		switch value.Type {
		case eval.ScalarValueType:
			kfilters = append(kfilters, &eventMaskEntry{
				approverType: AUIDApproverType,
				tableName:    auidApproversTable,
				tableKey:     ebpf.Uint32MapItem(value.Value.(int)),
				eventMask:    uint64(1 << (eventType - 1)),
			})
		case eval.RangeValueType:
			min, max := value.Value.(rules.RangeFilterValue).Min, value.Value.(rules.RangeFilterValue).Max
			if !auidRangeSet || auidRange.Min > min {
				auidRange.Min = min
			}
			if !auidRangeSet || auidRange.Max < max {
				auidRange.Max = max
			}
			auidRangeSet = true
		}
	}

	if auidRangeSet {
		kfilters = append(kfilters, &hashEntry{
			approverType: AUIDApproverType,
			tableName:    auidRangeApproversTable,
			tableKey:     eventType,
			value:        ebpf.NewUInt32RangeMapItem(uint32(auidRange.Min), uint32(auidRange.Max)),
		})
	}

	if len(kfilters) > 0 {
		fieldHandled = append(fieldHandled, auidField)
	}

	return kfilters, fieldHandled, nil
}
