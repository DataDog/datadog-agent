// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

func isProcessRunningUnix(host *components.RemoteHost, processName string) (bool, error) {
	_, err := host.Execute(fmt.Sprintf("pgrep '%s'", processName))
	return err == nil, err
}

func findPIDUnix(host *components.RemoteHost, processName string) ([]int, error) {
	// `pgrep` is limited to matching the first 15 characters of a process name _unless_ the `-f` flag is used, but the
	// '-f' flag acts more like a substring match, and will catch processes with the given name anywhere in their
	// command line.. so we only use '-f' if the process name to match is longer than 15 characters
	matchCommand := fmt.Sprintf("pgrep '%s'", processName)
	if len(processName) > 15 {
		matchCommand = fmt.Sprintf("pgrep -f '%s'", processName)
	}

	out, err := host.Execute(matchCommand)
	if err != nil {
		return nil, err
	}

	pids := []int{}
	for strPid := range strings.SplitSeq(out, "\n") {
		strPid = strings.TrimSpace(strPid)
		if strPid == "" {
			continue
		}
		pid, err := strconv.Atoi(strPid)
		if err != nil {
			return nil, err
		}
		pids = append(pids, pid)
	}

	return pids, nil
}
