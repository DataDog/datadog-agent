// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build secrets,!windows

package secrets

import (
	"fmt"
	"os/user"
	"strconv"
	"syscall"
)

func (info *SecretInfo) populateRights() {
	err := checkRights(info.ExecutablePath, secretBackendCommandAllowGroupExec)
	if err != nil {
		info.Rights = fmt.Sprintf("Error: %s", err)
	} else {
		info.Rights = fmt.Sprintf("OK, the executable has the correct rights")
	}

	var stat syscall.Stat_t
	if err := syscall.Stat(secretBackendCommand, &stat); err != nil {
		info.RightDetails = fmt.Sprintf("Could not stat %s: %s", secretBackendCommand, err)
	} else {
		info.RightDetails = fmt.Sprintf("file mode: %o", stat.Mode)
	}

	owner, err := user.LookupId(strconv.Itoa(int(stat.Uid)))
	if err != nil {
		info.UnixOwner = fmt.Sprintf("could not fetch name for UID %d: %s", stat.Uid, err)
	} else {
		info.UnixOwner = owner.Username
	}

	group, err := user.LookupGroupId(strconv.Itoa(int(stat.Gid)))
	if err != nil {
		info.UnixGroup = fmt.Sprintf("could not fetch name for GID %d: %s", stat.Gid, err)
	} else {
		info.UnixGroup = group.Name
	}
}
