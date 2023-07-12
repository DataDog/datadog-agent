// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package system

import "github.com/DataDog/datadog-agent/pkg/util/pointer"

func convertField(s *uint64, t **float64) {
	if s != nil {
		*t = pointer.Ptr(float64(*s))
	}
}

func convertFieldAndUnit(s *uint64, t **float64, multiplier float64) {
	if s != nil {
		*t = pointer.Ptr(float64(*s) * multiplier)
	}
}
