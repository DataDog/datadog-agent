// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package flare

import (
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
)

func addSystemProbePlatformSpecificEntries(fb flarehelpers.FlareBuilder) {}

func getLinuxKernelSymbols(fb flarehelpers.FlareBuilder) error {
	return nil
}

func getLinuxKprobeEvents(fb flarehelpers.FlareBuilder) error {
	return nil
}

func getLinuxDmesg(fb flarehelpers.FlareBuilder) error {
	return nil
}

func getLinuxPid1MountInfo(fb flarehelpers.FlareBuilder) error {
	return nil
}

func getLinuxTracingAvailableEvents(fb flarehelpers.FlareBuilder) error {
	return nil
}

func getLinuxTracingAvailableFilterFunctions(fb flarehelpers.FlareBuilder) error {
	return nil
}
