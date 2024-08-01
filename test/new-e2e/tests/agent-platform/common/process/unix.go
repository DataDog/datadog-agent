// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/types"
)

func isProcessRunningUnix(host *types.Host, processName string) (bool, error) {
	_, err := host.Execute(fmt.Sprintf("pgrep -f %s", processName))
	if err != nil {
		return false, err
	}
	return true, nil
}

func findPIDUnix(host *types.Host, processName string) ([]int, error) {
	out, err := host.Execute(fmt.Sprintf("pgrep -f '%s'", processName))
	if err != nil {
		return nil, err
	}

	pids := []int{}
	for _, strPid := range strings.Split(out, "\n") {
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
