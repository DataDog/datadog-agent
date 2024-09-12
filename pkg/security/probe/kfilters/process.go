// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kfilters holds kfilters related files
package kfilters

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

const (
	maxAUID                 = model.AuditUIDUnset - 1
	auidApproversTable      = "auid_approvers"
	auidRangeApproversTable = "auid_range_approvers"
)

var processCapabilities = rules.FieldCapability{
	Field:            "process.auid",
	TypeBitmask:      eval.ScalarValueType | eval.RangeValueType,
	FilterMode:       rules.ApproverOnlyMode,
	RangeFilterValue: &rules.RangeFilterValue{Min: 0, Max: maxAUID},
	FilterWeight:     100,
}

func getProcessKFilters(eventType model.EventType, approvers rules.Approvers) ([]activeKFilter, error) {
	values, exists := approvers["process.auid"]
	if !exists {
		return nil, errors.New("process auid not present")
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
				tableName: auidApproversTable,
				tableKey:  ebpf.Uint32MapItem(value.Value.(int)),
				eventMask: uint64(1 << (eventType - 1)),
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
		default:
			return nil, errors.New("value type not supported")
		}
	}

	if auidRange.Min != 0 || auidRange.Max != maxAUID {
		kfilters = append(kfilters, &hashEntry{
			tableName: auidRangeApproversTable,
			tableKey:  eventType,
			value:     ebpf.NewUInt32RangeMapItem(uint32(auidRange.Min), uint32(auidRange.Max)),
		})
	}

	return kfilters, nil
}
