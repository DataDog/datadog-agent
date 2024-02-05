// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package flare

import (
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

func addSystemProbePlatformSpecificEntries(_ flaretypes.FlareBuilder) {}

func getLinuxKernelSymbols(_ flaretypes.FlareBuilder) error {
	return nil
}

func getLinuxKprobeEvents(_ flaretypes.FlareBuilder) error {
	return nil
}

func getLinuxDmesg(_ flaretypes.FlareBuilder) error {
	return nil
}

func getLinuxPid1MountInfo(_ flaretypes.FlareBuilder) error {
	return nil
}

func getLinuxTracingAvailableEvents(_ flaretypes.FlareBuilder) error {
	return nil
}

func getLinuxTracingAvailableFilterFunctions(_ flaretypes.FlareBuilder) error {
	return nil
}
