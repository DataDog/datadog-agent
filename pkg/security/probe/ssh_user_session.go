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

const maxRetryForMsgWithSSHContext = 15

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
		SSHDPid: strconv.FormatUint(uint64(p.SSHDPid), 10),
		IP:      p.userSessionCtx.SSHClientIP,
		Port:    strconv.Itoa(p.userSessionCtx.SSHClientPort),
	}

	_, ok := p.resolver.GetSSHSession(key)

	if !ok {
		return fmt.Errorf("ssh session not found in LRU for %s:%d",
			p.userSessionCtx.SSHClientIP, p.userSessionCtx.SSHClientPort)
	}

	return nil
}

// MaxRetry implements the DelayabledEvent interface for SSH user sessions
func (p *SSHUserSessionPatcher) MaxRetry() int {
	return maxRetryForMsgWithSSHContext
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
