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
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usersessions"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/usersession"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
)

// maxRetryForMsgWithSSHContext is the number of retries granted to an event to wait for the
// authentication log line of its SSH session to be tailed from the auth log.
//
// sshd logs the "Accepted <method>" line right before spawning the user session, so the event of
// the very first command of a session usually races with the auth log tailer. A couple of retries
// (each one is `retryDelay` apart) is enough to close that gap.
//
// This budget must stay small: the retry queue is ordered, so an event waiting here delays all the
// following events. Sessions that can never be resolved (typically because they were established
// before the agent started tailing the log) pay it only once, see MarkUnresolved.
const maxRetryForMsgWithSSHContext = 5

func (p *EBPFProbe) HandleSSHUserSessionFromEvent(event *model.Event) {
	if p.config.RuntimeSecurity.SSHUserSessionsEnabled {
		pc := event.ProcessContext
		envp := p.fieldHandlers.ResolveProcessEnvp(nil, &pc.Process)
		usersessions.HandleSSHUserSession(pc, envp)
	}
}

// SSHUserSessionPatcher defines a patcher for SSH user sessions
type SSHUserSessionPatcher struct {
	userSessionCtx *serializers.SSHSessionContextSerializer
	resolver       *usersessions.Resolver
	SSHDPid        uint32
}

// NewSSHUserSessionPatcher creates a new SSH user session patcher
func NewSSHUserSessionPatcher(userSessionCtx *serializers.SSHSessionContextSerializer, resolver *usersessions.Resolver, SSHDPid uint32) *SSHUserSessionPatcher {
	return &SSHUserSessionPatcher{
		userSessionCtx: userSessionCtx,
		resolver:       resolver,
		SSHDPid:        SSHDPid,
	}
}

// sessionKey returns the LRU key of the SSH session of the event
func (p *SSHUserSessionPatcher) sessionKey() usersessions.SSHSessionKey {
	return usersessions.SSHSessionKey{
		SSHDPid: strconv.FormatUint(uint64(p.SSHDPid), 10),
		IP:      p.userSessionCtx.SSHClientIP,
		Port:    strconv.Itoa(p.userSessionCtx.SSHClientPort),
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

	key := p.sessionKey()

	// Check in LRU
	if _, ok := p.resolver.GetSSHSession(key); ok {
		return nil
	}

	// the session was already given up on, don't delay this event
	if p.resolver.IsSSHSessionUnresolved(key) {
		return nil
	}

	return fmt.Errorf("ssh session not found in LRU for %s:%d",
		p.userSessionCtx.SSHClientIP, p.userSessionCtx.SSHClientPort)
}

// MaxRetry implements the DelayabledEvent interface for SSH user sessions
func (p *SSHUserSessionPatcher) MaxRetry() int {
	return maxRetryForMsgWithSSHContext
}

// MarkUnresolved flags the SSH session of the event as unresolvable so that the next events of the
// same session are sent without waiting for its authentication log line
func (p *SSHUserSessionPatcher) MarkUnresolved() {
	if p.userSessionCtx == nil || p.resolver == nil {
		return
	}
	p.resolver.MarkSSHSessionUnresolved(p.sessionKey())
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
		SSHDPid: strconv.FormatUint(uint64(p.SSHDPid), 10),
		IP:      p.userSessionCtx.SSHClientIP,
		Port:    strconv.Itoa(p.userSessionCtx.SSHClientPort),
	}
	value, ok := p.resolver.GetSSHSession(key)

	if ok {
		ev.ProcessContextSerializer.UserSession.SSHAuthMethod = model.SSHAuthMethodToString(usersession.AuthType(value.AuthenticationMethod))
		ev.ProcessContextSerializer.UserSession.SSHPublicKey = value.PublicKey
	}
}
