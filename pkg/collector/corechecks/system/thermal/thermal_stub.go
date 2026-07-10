// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

// Package thermal implements the thermal zone check for Windows.
package thermal

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the check
const CheckName = "thermal"

// Factory returns no factory on unsupported platforms; the loader skips the check.
func Factory() option.Option[func() check.Check] {
	return option.None[func() check.Check]()
}
