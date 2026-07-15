// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package checks

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// LookupIDProbe is a no-op on Windows since user info is obtained from the process itself.
type LookupIDProbe struct{}

// NewLookupIDProbe returns a new LookupIDProbe
func NewLookupIDProbe(pkgconfigmodel.Reader) *LookupIDProbe {
	return &LookupIDProbe{}
}
