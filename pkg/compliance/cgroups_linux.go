// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package compliance

import (
	// We wrap pkg/security/utils here only for compat reason to be able to
	// still compile pkg/compliance on !linux.
	secutils "github.com/DataDog/datadog-agent/pkg/security/utils"
)

func getProcessContainerID(pid int32) (string, bool) {
	containerID, err := secutils.GetProcContainerID(uint32(pid), uint32(pid))
	if containerID == "" || err != nil {
		return "", false
	}
	return string(containerID), true
}

func getProcessRootPath(pid int32) (string, bool) {
	return secutils.ProcRootPath(uint32(pid)), true
}
