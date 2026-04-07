// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package modules

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	httpprotocol "github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/messagestrings"
)

func init() { registerModule(NetworkTracer) }

// NetworkTracer is a factory for NPM's tracer
var NetworkTracer = &module.Factory{
	Name: config.NetworkTracerModule,
	Fn:   createNetworkTracerModule,
}

func logIISSiteTimeouts() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
		`Import-Module WebAdministration; Get-ChildItem IIS:\Sites | Select-Object name, @{n="ConnectionTimeout";e={$_.limits.connectionTimeout}} | Format-Table -AutoSize | Out-String`)
	// Prevent a console window flash when system-probe is run interactively during development.
	// No effect in production where the service runs in session 0.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Debugf("failed to query IIS site timeouts: %v", err)
		return
	}
	if result := strings.TrimSpace(string(out)); result != "" {
		log.Infof("IIS site connection timeouts:\n%s", result)
	}
}

func (nt *networkTracer) platformRegister(httpMux *module.Router) error {
	if !nt.cfg.DirectSend {
		nt.restartTimer = time.AfterFunc(inactivityRestartDuration, func() {
			log.Criticalf("%v since the process-agent last queried for data. It may not be configured correctly and/or running. Exiting system-probe to save system resources.", inactivityRestartDuration)
			winutil.LogEventViewer(config.ServiceName, messagestrings.MSG_SYSPROBE_RESTART_INACTIVITY, inactivityRestartDuration.String())
			nt.Close()
			os.Exit(1)
		})
	}

	go logIISSiteTimeouts()

	httpMux.HandleFunc("/iis_tags", func(w http.ResponseWriter, req *http.Request) {
		cache := httpprotocol.GetIISTagsCache()
		utils.WriteAsJSON(req, w, cache, utils.CompactOutput)
	})

	httpMux.HandleFunc("/process_cache_tags", func(w http.ResponseWriter, req *http.Request) {
		tags := nt.tracer.GetProcessCacheTags()
		utils.WriteAsJSON(req, w, tags, utils.CompactOutput)
	})

	return nil
}
