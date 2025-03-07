// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package securityagent

import (
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

func addSecurityAgentPlatformSpecificEntries(_ flaretypes.FlareBuilder) {}

// only used in tests when running on linux
var linuxKernelSymbols = getLinuxKernelSymbols //nolint:unused

func getLinuxKernelSymbols(_ flaretypes.FlareBuilder) error { //nolint:unused
	return nil
}
