// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

// Package windowscertificate implements a windows certificate check. It does nothing on Linux.
package windowscertificate

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "windows_certificate"
)

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.None[func() check.Check]()
}
