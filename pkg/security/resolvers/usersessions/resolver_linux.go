// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package usersessions holds model related to the user sessions resolver
package usersessions

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/hashicorp/golang-lru/v2/simplelru"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// Resolver is used to resolve the user sessions context
type Resolver struct {
	sync.RWMutex
	userSessions *simplelru.LRU[uint64, *model.UserSessionContext]

	userSessionsMap *ebpf.Map
}

// NewResolver returns a new instance of Resolver
func NewResolver(config *config.Config) (*Resolver, error) {
	lru, err := simplelru.NewLRU[uint64, *model.UserSessionContext](config.RuntimeSecurity.UserSessionsCacheSize, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create User Session resolver cache: %w", err)
	}

	return &Resolver{
		userSessions: lru,
	}, nil
}

// Start initializes the eBPF map of the resolver
func (r *Resolver) Start(manager *manager.Manager) error {
	r.Lock()
	defer r.Unlock()

	m, err := managerhelper.Map(manager, "user_sessions")
	if err != nil {
		return fmt.Errorf("couldn't start user session resolver: %w", err)
	}
	r.userSessionsMap = m
	return nil
}

// ResolveUserSession returns the user session associated to the provided ID
func (r *Resolver) ResolveUserSession(id uint64) *model.UserSessionContext {
	r.Lock()
	defer r.Unlock()

	if id == 0 {
		return nil
	}

	// is this session already in cache ?
	if session, ok := r.userSessions.Get(id); ok {
		return session
	}

	// lookup the session in kernel space
	key := model.UserSessionKey{
		ID:     id,
		Cursor: 1,
	}
	ctx := model.UserSessionContext{
		ID: id,
	}

	err := r.userSessionsMap.Lookup(&key, &ctx)
	for err == nil {
		key.Cursor++
		err = r.userSessionsMap.Lookup(&key, &ctx)
	}
	if key.Cursor == 1 && err != nil {
		// the session doesn't exist, leave now
		return nil
	}
	// parse the content of the user session context
	err = json.Unmarshal([]byte(ctx.RawData), &ctx)
	if err != nil {
		seclog.Debugf("failed to parse user session data: %v", err)
		return nil
	}

	ctx.Resolved = true
	ctx.RawData = ""

	// cache resolved context
	r.userSessions.Add(id, &ctx)
	return &ctx
}
