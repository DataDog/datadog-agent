// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package misconfig

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/syndtr/gocapability/capability"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() {
	registerCheck("proc mount", procMount, map[AgentType]struct{}{CoreAgent: {}, ProcessAgent: {}})
}

func isHidepid2Set(mountsOptsLookup map[string]struct{}) (setting string, isSet bool) {
	if _, hidepid2Set := mountsOptsLookup["hidepid=2"]; hidepid2Set {
		return "hidepid=2", true
	}
	if _, hidepidInvisibleSet := mountsOptsLookup["hidepid=invisible"]; hidepidInvisibleSet {
		return "hidepid=invisible", true
	}
	return "", false
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

	egid := os.Getegid()
	var haveEgid bool
	// From `man getgroups`:
	// It is unspecified whether the effective group ID of the calling process is included in the returned list.
	for _, gid := range groups {
		if gid == egid {
			haveEgid = true
			break
		}
	}
	if !haveEgid {
		groups = append(groups, egid)
	}
	path := config.Datadog.GetString("container_proc_root")
	if config.IsContainerized() && path != "/proc" {
		path = filepath.Join(path, "1/mounts")
	} else {
		path = filepath.Join(path, "mounts")
	}
	return checkProcMountHidePid(path, os.Geteuid(), groups)
}

func checkProcMountHidePid(path string, uid int, groups []int) error {
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

		hidepidSetting, hidepid2Set := isHidepid2Set(mountOptsLookup)
		if !hidepid2Set {
			// hidepid is not set, no further checks necessary
			log.Tracef("Proc mounts hidepid=2 or hidepid=invisible option is not set")
			return nil
		}

		if !hasGidOpt {
			mountOptsLookup["gid=0"] = struct{}{}
		}

		gidList := make([]string, 0, len(groups))
		for _, gid := range groups {
			gidOpt := fmt.Sprintf("gid=%d", gid)
			gidList = append(gidList, strconv.Itoa(gid))
			if _, ok := mountOptsLookup[gidOpt]; ok {
				// While hidepid=2 is set, one of the groups is enabled
				log.Tracef("Proc mounts %s with %s - proc fs inspection is enabled", hidepidSetting, gidOpt)
				return nil
			}
		}

		return fmt.Errorf("%s option detected in %s (options=%s) - proc fs inspection may not work (uid=%d, groups=[%s])",
			hidepidSetting, path, fields[3], uid, strings.Join(gidList, ","))
	}

	return errors.Wrapf(scanner.Err(), "failed to scan %s", path)
}
