// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"math/rand/v2"
	"net"
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

	envp := p.fieldHandlers.ResolveProcessEnvp(event, &event.ProcessContext.Process)
	sshClientVar := getEnvVar(envp, "SSH_CLIENT")

	// If the parent is a sshd process and the SSH_CLIENT environment variable is set, we consider it's a new ssh session
	if parent != nil && strings.Contains(parent.Comm, "sshd") && sshClientVar != "" {
		sshSessionId := rand.Uint64()
		event.ProcessContext.UserSession.ID = sshSessionId
		event.ProcessContext.UserSession.SessionType = 2
		parts := strings.Fields(sshClientVar)
		if len(parts) >= 2 {
			event.ProcessContext.UserSession.SSHClientIP = getIPfromEnv(parts[0])
			if port, err := strconv.Atoi(parts[1]); err != nil {
				seclog.Warnf("failed to parse SSH_CLIENT port from %q: %v", sshClientVar, err)
			} else {
				event.ProcessContext.UserSession.SSHPort = port
			}
		}

	}
}

func getIPfromEnv(ipStr string) net.IPNet {
	ip := net.ParseIP(ipStr)
	if ip != nil {
		if ip.To4() != nil {
			return net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(32, 32),
			}
		} else {
			return net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(128, 128),
			}
		}
	}
	return net.IPNet{}
}
