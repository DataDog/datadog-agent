// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package secretsimpl

import (
	_ "embed"
	"fmt"
	"os/user"
	"strconv"
	"syscall"
)

func (r *secretResolver) getExecutablePermissions() (map[string]string, error) {
	var stat syscall.Stat_t
	if err := syscall.Stat(r.backendCommand, &stat); err != nil {
		return nil, fmt.Errorf("could not stat %s: %s", r.backendCommand, err)
	}

	details := map[string]string{
		"FileMode": fmt.Sprintf("%o", stat.Mode),
	}

	if owner, err := user.LookupId(strconv.Itoa(int(stat.Uid))); err != nil {
		details["Owner"] = fmt.Sprintf("could not fetch name for UID %d: %s", stat.Uid, err)
	} else {
		details["Owner"] = owner.Username
	}

	if group, err := user.LookupGroupId(strconv.Itoa(int(stat.Gid))); err != nil {
		details["Group"] = fmt.Sprintf("could not fetch name for GID %d: %s", stat.Gid, err)
	} else {
		details["Group"] = group.Name
	}

	return details, nil
}
