// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usersessions"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/usersession"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
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
	// First, we check if this event is link to an existing ssh session from his parent
	ppid := event.ProcessContext.Process.PPid
	parent := p.Resolvers.ProcessResolver.Resolve(ppid, ppid, 0, false, nil)

	// If the parent is a sshd process, we consider it's a new ssh session
	if parent != nil && parent.Comm == "sshd" {
		sshSessionID := rand.Uint64()
		event.ProcessContext.UserSession.SSHSessionID = sshSessionID
		event.ProcessContext.UserSession.SessionType = int(usersession.UserSessionTypeSSH)
		// Try to extract the SSH client IP and port
		envp := p.fieldHandlers.ResolveProcessEnvp(event, &event.ProcessContext.Process)
		sshClientVar := getEnvVar(envp, "SSH_CLIENT")
		parts := strings.Fields(sshClientVar)
		if len(parts) >= 2 {
			event.ProcessContext.UserSession.SSHClientIP = getIPfromEnv(parts[0])
			if port, err := strconv.Atoi(parts[1]); err != nil {
				seclog.Warnf("failed to parse SSH_CLIENT port from %q: %v", sshClientVar, err)
			} else {
				event.ProcessContext.UserSession.SSHClientPort = port
			}
		} else {
			seclog.Warnf("SSH_CLIENT is not in the expected format: %q", sshClientVar)
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
		} else if ip.To16() != nil {
			return net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(128, 128),
			}
		}
	}
	return net.IPNet{}
}

// SSHUserSessionPatcher defines a patcher for SSH user sessions
type SSHUserSessionPatcher struct {
	userSessionCtx *serializers.SSHSessionContextSerializer
	resolver       *usersessions.Resolver
}

// NewSSHUserSessionPatcher creates a new SSH user session patcher
func NewSSHUserSessionPatcher(userSessionCtx *serializers.SSHSessionContextSerializer, resolver *usersessions.Resolver) *SSHUserSessionPatcher {
	return &SSHUserSessionPatcher{
		userSessionCtx: userSessionCtx,
		resolver:       resolver,
	}
}

// IsResolved implements the EventSerializerPatcher interface for SSH user sessions
func (p *SSHUserSessionPatcher) IsResolved() error {
	if p.userSessionCtx == nil {
		return errors.New("user session context is nil")
	}
	if p.resolver == nil {
		return errors.New("resolver is nil")
	}

	// Check in LRU
	key := usersessions.SSHSessionKey{
		IP:   p.userSessionCtx.SSHClientIP,
		Port: strconv.Itoa(p.userSessionCtx.SSHClientPort),
	}

	p.resolver.SSHSessionParsed.Mu.Lock()
	_, ok := p.resolver.SSHSessionParsed.Lru.Get(key)
	p.resolver.SSHSessionParsed.Mu.Unlock()

	if !ok {
		return fmt.Errorf("ssh session not found in LRU for %s:%d",
			p.userSessionCtx.SSHClientIP, p.userSessionCtx.SSHClientPort)
	}

	return nil
}

// PatchEvent implements the EventSerializerPatcher interface for SSH user sessions
func (p *SSHUserSessionPatcher) PatchEvent(ev *serializers.EventSerializer) {
	if ev.ProcessContextSerializer == nil || ev.ProcessContextSerializer.UserSession == nil {
		return
	}

	if p.userSessionCtx == nil {
		return
	}

	key := usersessions.SSHSessionKey{
		IP:   p.userSessionCtx.SSHClientIP,
		Port: strconv.Itoa(p.userSessionCtx.SSHClientPort),
	}
	p.resolver.SSHSessionParsed.Mu.Lock()
	value, ok := p.resolver.SSHSessionParsed.Lru.Get(key)
	p.resolver.SSHSessionParsed.Mu.Unlock()

	if ok {
		if model.SSHAuthMethodStrings == nil {
			model.InitSSHAuthMethodConstants()
		}
		ev.ProcessContextSerializer.UserSession.SSHAuthMethod = model.SSHAuthMethodStrings[usersession.AuthType(value.AuthenticationMethod)]
		ev.ProcessContextSerializer.UserSession.SSHPublicKey = value.PublicKey
	}
}
