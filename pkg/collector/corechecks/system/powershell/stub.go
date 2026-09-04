// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

// Package powershell implements a Windows core check that runs allowlisted
// read-only PowerShell Get-* cmdlets. It does nothing on non-Windows platforms.
package powershell

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "powershell"
)

// Factory returns no factory on non-Windows platforms, so the loader skips it.
func Factory() option.Option[func() check.Check] {
	return option.None[func() check.Check]()
}
