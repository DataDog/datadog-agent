// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package misconfig

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/pkg/errors"
	"github.com/syndtr/gocapability/capability"
)

func init() {
	registerCheck("proc mount", procMount)
}

func procMount() error {

	if os.Geteuid() == 0 {
		// Check for root + SYS_PTRACE capability - if it is set we don't
		// need to check the rest of the logic here
		if caps, err := capability.NewPid2(0); err == nil && caps.Load() == nil {
			if caps.Get(capability.EFFECTIVE, capability.CAP_SYS_PTRACE) {
				log.Debugf("Running as root with cap_sys_ptrace - not checking for hidepid on proc fs")
				return nil
			}
		}
	}

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
		return errors.Wrapf(err, "failed to open %s - proc fs inspection may not work", path)
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
		var hasGidOpt bool
		for _, opt := range mountOpts {
			if strings.HasPrefix(opt, "gid=") {
				hasGidOpt = true
			}
			mountOptsLookup[opt] = struct{}{}
		}

		if _, ok := mountOptsLookup["hidepid=2"]; !ok {
			// hidepid is not set, no further checks necessary
			return nil
		}

		if !hasGidOpt {
			mountOptsLookup["gid=0"] = struct{}{}
		}

		for _, gid := range groups {
			gidOpt := fmt.Sprintf("gid=%d", gid)
			if _, ok := mountOptsLookup[gidOpt]; ok {
				// While hidepid=2 is set, one of the groups is enabled
				return nil
			}
		}

		return fmt.Errorf("hidepid=2 option detected in %s - proc fs inspection may not work", path)
	}

	return errors.Wrapf(scanner.Err(), "failed to scan %s", path)
}
