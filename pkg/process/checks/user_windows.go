// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package checks

import (
	"github.com/DataDog/datadog-agent/pkg/config"
)

// On Windows the LookupIdProbe does nothing since we get the user info from the process itself.
//
//nolint:revive // TODO(PROC) Fix revive linter
type LookupIdProbe struct{}

// NewLookupIDProbe returns a new LookupIdProbe
//
//nolint:revive // TODO(PROC) Fix revive linter
func NewLookupIDProbe(config.Reader) *LookupIdProbe {
	return &LookupIdProbe{}
}
