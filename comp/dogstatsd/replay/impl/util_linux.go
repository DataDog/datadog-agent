// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package replayimpl

import (
	"os"
	"syscall"

	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
)

// GetUcredsForPid returns the replay ucreds for the specified pid
func GetUcredsForPid(pid int32) []byte {
	ucreds := &syscall.Ucred{
		Pid: int32(os.Getpid()),
		Uid: uint32(pid),
		Gid: replay.GUID,
	}

	return syscall.UnixCredentials(ucreds)
}
