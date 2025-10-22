// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cgo

package jmxclient

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "jmxclient"
)

// Factory creates a new check factory that returns None when CGo is not available
func Factory() option.Option[func() check.Check] {
	// Return None to indicate this check is not available without CGo
	return option.None[func() check.Check]()
}
