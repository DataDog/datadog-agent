// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !windows

package secrets

import (
	"fmt"
	"os/user"
	"syscall"
)

func checkRights(path string) error {
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		return fmt.Errorf("invalid executable '%s': can't stat it: %s", path, err)
	}

	// checking that group and others don't have any rights
	if stat.Mode&(syscall.S_IRWXG|syscall.S_IRWXO) != 0 {
		return fmt.Errorf("invalid executable '%s', 'groups' or 'others' have rights on it", path)
	}

	// checking that the owner have exec rights
	if stat.Mode&syscall.S_IXUSR == 0 {
		return fmt.Errorf("invalid executable: '%s' is not executable", path)
	}

	// checking that we own the executable
	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("can't query current user UID")
	}

	// checking we own the executable. This is useless since we won't be able
	// to execute it if not, but it gives a better error message to the
	// user.
	if fmt.Sprintf("%d", stat.Uid) != usr.Uid && usr.Uid != "0" {
		return fmt.Errorf("invalid executable: '%s' isn't owned by the user running the agent: name '%s', UID %s. We can't execute it", path, usr.Username, usr.Uid)
	}

	return nil
}
