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

var bindCapabilities = rules.FieldCapabilities{
	{
		Field:        "bind.addr.family",
		TypeBitmask:  eval.ScalarValueType,
		FilterWeight: 50,
	},
	{
		Field:       "bind.addr.port",
		TypeBitmask: eval.ScalarValueType,
		// non-INET binds have a zero port, an approver on it can't be narrowed to AF_INET/AF_INET6
		ValidateFnc: func(value rules.FilterValue) bool {
			port, ok := value.Value.(int)
			return ok && port != 0
		},
		FilterWeight: 80,
	},
	{
		Field:        "bind.addr.ip",
		TypeBitmask:  eval.IPNetValueType,
		FilterWeight: 90,
	},
	{
		Field:       "bind.addr.is_public",
		TypeBitmask: eval.ScalarValueType,
		// non-INET binds have no IP, which resolves as public, an approver on it can't be narrowed to AF_INET/AF_INET6
		ValidateFnc: func(value rules.FilterValue) bool {
			isPublic, ok := value.Value.(bool)
			return ok && !isPublic
		},
		FilterWeight: 70,
	},
}

func bindKFiltersGetter(approvers rules.Approvers) (KFilters, []eval.Field, error) {
	var (
		fieldHandled []eval.Field
	)

	var bindAddrFamilyValues rules.FilterValues

	for field, values := range approvers {
		switch field {
		case "bind.addr.family":
			bindAddrFamilyValues = bindAddrFamilyValues.Merge(values...)
			fieldHandled = append(fieldHandled, field)
		case "bind.addr.port", "bind.addr.ip", "bind.addr.is_public":
			bindAddrFamilyValues = bindAddrFamilyValues.Merge(implicitAfInetFilterValues("bind.addr.family")...)
			fieldHandled = append(fieldHandled, field)
		}
	}

	kfilter, err := getEnumsKFiltersWithIndex("bind_addr_family_approvers", 0, uintValues[uint64](bindAddrFamilyValues)...)
	if err != nil {
		return nil, nil, err
	}

	return newKFilters(kfilter), fieldHandled, nil
}
