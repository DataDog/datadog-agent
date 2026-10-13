// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package run

import (
	"context"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Must match procmgrJob in pkg/fleet/installer/packages/datadog_agent_darwin.go.
const procmgrLaunchdLabel = "com.datadoghq.procmgr"

// Kickstarts com.datadoghq.procmgr when process_manager.enabled, if its LaunchDaemon is already
// installed (RunAtLoad / installStableJobs). Does not create a missing plist or re-run leaf
// suppression: those stay install-time only.
func startDependentServicesAsync(coreConf model.Reader, _ model.Reader) {
	go kickstartProcmgrIfEnabled(coreConf)
}

func kickstartProcmgrIfEnabled(coreConf model.Reader) {
	if !coreConf.GetBool("process_manager.enabled") {
		return
	}
	cmd := exec.CommandContext(context.Background(), "/bin/launchctl", "kickstart", "system/"+procmgrLaunchdLabel)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Debugf("could not kickstart %s: %v (%s)", procmgrLaunchdLabel, err, out)
	}
}

// launchd owns procmgr and its children. Nothing to stop from the Agent.
func stopDependentServices(_ model.Reader, _ model.Reader) {}
