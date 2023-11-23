// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets && !windows

package secrets

import (
	_ "embed"
	"fmt"
	"os/user"
	"strconv"
	"syscall"
)

//go:embed info_nix.tmpl
var permissionsDetailsTemplate string

type permissionsDetails struct {
	FileMode string
	Owner    string
	Group    string
}

func getExecutablePermissions() (interface{}, error) {
	var stat syscall.Stat_t
	if err := syscall.Stat(secretBackendCommand, &stat); err != nil {
		return nil, fmt.Errorf("Could not stat %s: %s", secretBackendCommand, err)
	}

	details := permissionsDetails{
		FileMode: fmt.Sprintf("%o", stat.Mode),
	}

	owner, err := user.LookupId(strconv.Itoa(int(stat.Uid)))
	if err != nil {
		details.Owner = fmt.Sprintf("could not fetch name for UID %d: %s", stat.Uid, err)
	} else {
		details.Owner = owner.Username
	}

	group, err := user.LookupGroupId(strconv.Itoa(int(stat.Gid)))
	if err != nil {
		details.Group = fmt.Sprintf("could not fetch name for GID %d: %s\n", stat.Gid, err)
	} else {
		details.Group = group.Name
	}
	return details, nil
}
