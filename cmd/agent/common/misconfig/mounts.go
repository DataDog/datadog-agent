// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package misconfig

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/pkg/errors"
)

func init() {
	registerCheck("proc mount", procMount)
}

func procMount() error {
	groups, err := os.Getgroups()
	if err != nil {
		return fmt.Errorf("failed to get process groups: %v", err)
	}
	path := config.Datadog.GetString("container_proc_root")
	if config.IsContainerized() && path != "/proc" {
		path = filepath.Join(path, "1/mounts")
	} else {
		path = filepath.Join(path, "mounts")
	}
	return checkProcMountHidePid(path, groups)
}

func checkProcMountHidePid(path string, groups []int) error {
	file, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "failed to open %s", path)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 6 ||
			fields[1] != "/proc" || fields[2] != "proc" {
			continue
		}
		mountOpts := strings.Split(fields[3], ",")
		mountOptsLookup := map[string]struct{}{}
		for _, opt := range mountOpts {
			mountOptsLookup[opt] = struct{}{}
		}

		if _, ok := mountOptsLookup["hidepid=2"]; !ok {
			// hidepid is not set, no further checks necessary
			return nil
		}

		for _, gid := range groups {
			gidOpt := fmt.Sprintf("gid=%d", gid)
			if _, ok := mountOptsLookup[gidOpt]; ok {
				// While hidepid=2 is set, one of the groups is enabled
				return nil
			}
		}

		return fmt.Errorf("hidepid=2 option detected in %s - will prevent inspection of proc fs", path)
	}

	return errors.Wrapf(scanner.Err(), "failed to scan %s", path)
}
