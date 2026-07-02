// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package api

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var afterFunc = time.AfterFunc

var kickstart = func(service string) error {
	cmd := exec.Command("/bin/launchctl", "kickstart", "-k", service)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("launchctl kickstart %s failed: %w", service, err)
		}
		return fmt.Errorf("launchctl kickstart %s failed: %w: %s", service, err, msg)
	}
	return nil
}

func handleAgentRestart(w http.ResponseWriter, _ *http.Request) {
	// Reply 200 immediately so the client receives the response before launchd
	// tears down this process when sysprobe is restarted.
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Restart both services after a short delay so the HTTP response has time
	// to be delivered before launchd sends SIGTERM to this process.
	afterFunc(100*time.Millisecond, func() {
		if err := kickstart("system/com.datadoghq.agent"); err != nil {
			log.Errorf("agent-restart: failed to restart com.datadoghq.agent: %v", err)
		}
		if err := kickstart("system/com.datadoghq.sysprobe"); err != nil {
			log.Errorf("agent-restart: failed to restart com.datadoghq.sysprobe: %v", err)
		}
	})
}
