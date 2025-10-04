// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

func getEnvFromProc(pid uint32, key string) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return ""
	}
	for _, kv := range strings.Split(string(data), "\x00") {
		if kv == "" {
			continue
		}
		if strings.HasPrefix(kv, key+"=") {
			return strings.TrimPrefix(kv, key+"=")
		}
	}
	return ""
}

// HandleSSHUserSession handles the ssh user session
func (p *EBPFProbe) HandleSSHUserSession(event *model.Event) {
	//// First, we check if this event is link to an existing ssh session from his parent
	ppid := event.ProcessContext.Process.PPid
	parent := p.Resolvers.ProcessResolver.Resolve(ppid, ppid, 0, false, nil)

	// Inherit SSH session from parent if it exists and parent is not nil
	if parent != nil && parent.ProcessContext.UserSession.ID != 0 && parent.ProcessContext.UserSession.SessionType == 2 {
		// Copy all SSH session fields from parent
		event.ProcessContext.UserSession.ID = parent.ProcessContext.UserSession.ID
		event.ProcessContext.UserSession.SessionType = 2
		event.ProcessContext.UserSession.Resolved = parent.ProcessContext.UserSession.Resolved
		event.ProcessContext.UserSession.SSHClientIP = parent.ProcessContext.UserSession.SSHClientIP
		event.ProcessContext.UserSession.SSHPort = parent.ProcessContext.UserSession.SSHPort
		if event.ProcessContext.UserSession.Resolved {
			event.ProcessContext.UserSession.SSHAuthMethod = parent.ProcessContext.UserSession.SSHAuthMethod
			event.ProcessContext.UserSession.SSHPublicKey = parent.ProcessContext.UserSession.SSHPublicKey
			event.ProcessContext.UserSession.SSHUsername = parent.ProcessContext.UserSession.SSHUsername
		}
		return
	}
	pidToRead := event.ProcessContext.Process.Pid
	testOut := getEnvFromProc(pidToRead, "SSH_CLIENT")

	// If the parent is a sshd process and the SSH_CLIENT environment variable is set, we consider it's a new ssh session
	if parent != nil && strings.Contains(parent.Comm, "sshd") && testOut != "" {
		sshSessionId := rand.Uint64()
		event.ProcessContext.UserSession.ID = sshSessionId
		event.ProcessContext.UserSession.SessionType = 2
		parts := strings.Fields(testOut)
		if len(parts) >= 2 {
			event.ProcessContext.UserSession.SSHClientIP = parts[0]
			if port, err := strconv.Atoi(parts[1]); err != nil {
				seclog.Warnf("failed to parse SSH_CLIENT port from %q: %v", testOut, err)
			} else {
				event.ProcessContext.UserSession.SSHPort = port
			}
		}

	}
}
