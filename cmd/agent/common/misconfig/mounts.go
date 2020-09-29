// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package misconfig

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
)

func procMount() error {
	groups, err := os.Getgroups()
	if err != nil {
		return fmt.Errorf("failed to get process groups: %v", err)
	}
	return checkProcMountHidePid("/host/proc/1/mounts", groups)
}

func checkProcMountHidePid(path string, groups []int) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to open %s", path)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 6 ||
			fields[0] != "proc" || fields[1] != "/proc" {
			continue
		}
		mountOpts := strings.Split(fields[3], ",")
		mountOptsLookup := map[string]bool{}
		for _, opt := range mountOpts {
			mountOptsLookup[opt] = true
		}

		if ok, _ := mountOptsLookup["hidepid=2"]; !ok {
			// hidepid is not set, no further checks necessary
			return nil
		}

		for _, gid := range groups {
			gidOpt := fmt.Sprintf("gid=%d", gid)
			if ok, _ := mountOptsLookup[gidOpt]; ok {
				// While hidepid=2 is set, one of the groups is enabled
				return nil
			}
		}

		return fmt.Errorf("hidepid=2 option detected in %s - will prevent procfs inspection", path)
	}

	return errors.Wrapf(scanner.Err(), "failed to scan %s", path)
}
