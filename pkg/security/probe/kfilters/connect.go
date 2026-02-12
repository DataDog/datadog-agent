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
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var connectCapabilities = rules.FieldCapabilities{
	{
		Field:        "connect.addr.family",
		TypeBitmask:  eval.ScalarValueType,
		FilterWeight: 50,
	},
	{
		Field:        "connect.addr.port",
		TypeBitmask:  eval.ScalarValueType,
		FilterWeight: 80,
	},
	{
		Field:        "connect.addr.ip",
		TypeBitmask:  eval.IPNetValueType,
		FilterWeight: 90,
	},
	{
		Field:        "connect.addr.is_public",
		TypeBitmask:  eval.ScalarValueType,
		FilterWeight: 70,
	},
	{
		Field:        "connect.addr.hostname",
		TypeBitmask:  eval.ScalarValueType,
		FilterWeight: 100,
	},
}

func connectKFiltersGetter(approvers rules.Approvers) (KFilters, []eval.Field, error) {
	var (
		fieldHandled []eval.Field
	)

	var connectAddrFamilyValues rules.FilterValues

	for field, values := range approvers {
		switch field {
		case "connect.addr.family":
			connectAddrFamilyValues = connectAddrFamilyValues.Merge(values...)
			fieldHandled = append(fieldHandled, field)
		case "connect.addr.port", "connect.addr.ip", "connect.addr.is_public", "connect.addr.hostname":
			connectAddrFamilyValues = connectAddrFamilyValues.Merge(implicitAfInetFilterValues()...)
			fieldHandled = append(fieldHandled, field)
		}
	}

	kfilter, err := getEnumsKFiltersWithIndex("connect_addr_family_approvers", 0, uintValues[uint64](connectAddrFamilyValues)...)
	if err != nil {
		return nil, nil, err
	}

	return newKFilters(kfilter), fieldHandled, nil
}

func implicitAfInetFilterValues() rules.FilterValues {
	return rules.FilterValues{
		{
			Field: "connect.addr.family",
			Value: unix.AF_INET,
			Type:  eval.ScalarValueType,
		},
		{
			Field: "connect.addr.family",
			Value: unix.AF_INET6,
			Type:  eval.ScalarValueType,
		},
	}
}
