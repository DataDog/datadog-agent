// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// getEnvVar extracts a specific environment variable from a list of environment variables.
// Each environment variable is in the format "KEY=VALUE".
func getEnvVar(envp []string, key string) string {
	prefix := key + "="
	for _, env := range envp {
		if strings.HasPrefix(env, prefix) {
			return strings.TrimPrefix(env, prefix)
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
	envp := p.fieldHandlers.ResolveProcessEnvp(event, &event.ProcessContext.Process)
	sshClientVar := getEnvVar(envp, "SSH_CLIENT")

	// If the parent is a sshd process and the SSH_CLIENT environment variable is set, we consider it's a new ssh session
	if parent != nil && strings.Contains(parent.Comm, "sshd") && sshClientVar != "" {
		sshSessionId := rand.Uint64()
		event.ProcessContext.UserSession.ID = sshSessionId
		event.ProcessContext.UserSession.SessionType = 2
		parts := strings.Fields(sshClientVar)
		if len(parts) >= 2 {
			event.ProcessContext.UserSession.SSHClientIP = parts[0]
			if port, err := strconv.Atoi(parts[1]); err != nil {
				seclog.Warnf("failed to parse SSH_CLIENT port from %q: %v", sshClientVar, err)
			} else {
				event.ProcessContext.UserSession.SSHPort = port
			}
		}

	}
}
