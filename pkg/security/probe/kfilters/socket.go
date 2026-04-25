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

var socketCapabilities = rules.FieldCapabilities{
	{
		Field:        "socket.domain",
		TypeBitmask:  eval.ScalarValueType,
		FilterWeight: 50,
	},
	{
		Field:        "socket.type",
		TypeBitmask:  eval.ScalarValueType | eval.BitmaskValueType,
		FilterWeight: 60,
	},
	{
		Field:        "socket.protocol",
		TypeBitmask:  eval.ScalarValueType,
		FilterWeight: 70,
	},
}

func socketKFiltersGetter(approvers rules.Approvers) (KFilters, []eval.Field, error) {
	var (
		kfilters     []kFilter
		fieldHandled []eval.Field
	)

	for field, values := range approvers {
		var mapName string
		switch field {
		case "socket.domain":
			mapName = "socket_domain_approvers"
		case "socket.type":
			mapName = "socket_type_approvers"
		case "socket.protocol":
			mapName = "socket_protocol_approvers"
		default:
			continue
		}

		kfilter, err := getEnumsKFiltersWithIndex(mapName, 0, uintValues[uint64](values)...)
		if err != nil {
			return nil, nil, err
		}
		kfilters = append(kfilters, kfilter)
		fieldHandled = append(fieldHandled, field)
	}
	return newKFilters(kfilters...), fieldHandled, nil
}
