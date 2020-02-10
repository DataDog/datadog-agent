// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build secrets,!windows

package secrets

import (
	"fmt"
	"os/user"
	"strconv"
	"syscall"
)

func (info *SecretInfo) populateRights() {
	for x := 0; x < len(info.ExecutablePath); x++ {
		err := checkRights(info.ExecutablePath[x])
		if err != nil {
			info.Rights = append(info.Rights, fmt.Sprintf("Error: %s", err))
		} else {
			info.Rights = append(info.Rights, fmt.Sprintf("OK, the executable has the correct rights"))
		}

		var stat syscall.Stat_t
		if err := syscall.Stat(info.ExecutablePath[x], &stat); err != nil {
			info.RightDetails = append(info.RightDetails, fmt.Sprintf("Could not stat %s: %s", info.ExecutablePath[x], err))
		} else {
			info.RightDetails = append(info.RightDetails, fmt.Sprintf("file mode: %o", stat.Mode))
		}

		owner, err := user.LookupId(strconv.Itoa(int(stat.Uid)))
		if err != nil {
			info.UnixOwner = append(info.UnixOwner, fmt.Sprintf("could not fetch name for UID %d: %s", stat.Uid, err))
		} else {
			info.UnixOwner = append(info.UnixOwner, owner.Username)
		}

		group, err := user.LookupGroupId(strconv.Itoa(int(stat.Gid)))
		if err != nil {
			info.UnixGroup = append(info.UnixGroup, fmt.Sprintf("could not fetch name for GID %d: %s", stat.Gid, err))
		} else {
			info.UnixGroup = append(info.UnixGroup, group.Name)
		}
	}
}
