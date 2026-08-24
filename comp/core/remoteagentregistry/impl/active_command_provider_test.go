// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
)

func TestActiveCommandProviderUsesOldestRegistrationAndPromotesOnRemoval(t *testing.T) {
	oldest := &remoteAgentClient{
		RegisteredAgent: remoteagentregistry.RegisteredAgent{CommandName: "data-plane", SessionID: "oldest"},
		services:        []remoteAgentServiceName{CommandProviderServiceName},
		registeredAt:    time.Unix(1, 0),
	}
	newest := &remoteAgentClient{
		RegisteredAgent: remoteagentregistry.RegisteredAgent{CommandName: "data-plane", SessionID: "newest"},
		services:        []remoteAgentServiceName{CommandProviderServiceName},
		registeredAt:    time.Unix(2, 0),
	}
	registry := &remoteAgentRegistry{
		agentMap:   map[string]*remoteAgentClient{"oldest": oldest, "newest": newest},
		agentMapMu: sync.Mutex{},
	}

	registry.agentMapMu.Lock()
	require.Same(t, oldest, registry.activeCommandProviderForCommandNameLocked("data-plane"))
	delete(registry.agentMap, "oldest")
	require.Same(t, newest, registry.activeCommandProviderForCommandNameLocked("data-plane"))
	registry.agentMapMu.Unlock()
}
