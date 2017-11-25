// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package lifecycle

import (
	log "github.com/cihub/seelog"

	"github.com/coreos/go-systemd/daemon"
	systemdutil "github.com/coreos/go-systemd/util"
)

// notifySystemd call sd_notify(3) if the agent is running on systemd platform and in a systemd service
func notifySystemd() {
	if systemdutil.IsRunningSystemd() == false {
		log.Info("Not running on systemd platform")
		return
	}

	inService, err := systemdutil.RunningFromSystemService()
	if err != nil {
		log.Errorf("Fail to identify if running in systemd service: %s", err)
		return
	}
	if inService == false {
		log.Info("Not running in systemd service")
		return
	}

	sent, err := daemon.SdNotify(false, "READY=1")
	if err != nil {
		log.Errorf("Failed to notify systemd for readiness: %v", err)
		return
	}
	if sent == false {
		log.Errorf("Forgot to set Type=notify in systemd service file?")
	}
}
