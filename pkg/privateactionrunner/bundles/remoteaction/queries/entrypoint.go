// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_queries

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

const (
	// BundleID is the local-only Remote Queries PAR bundle FQN.
	BundleID = "com.datadoghq.remoteaction.queries"

	// ExecuteActionName is the action part of com.datadoghq.remoteaction.queries.execute.
	ExecuteActionName = "execute"
)

type RemoteQueriesBundle struct {
	actions map[string]types.Action
}

var defaultBridgeClientFactory BridgeClientFactory = NewDefaultBridgeClient

func NewRemoteQueriesBundle() *RemoteQueriesBundle {
	return &RemoteQueriesBundle{
		actions: map[string]types.Action{
			ExecuteActionName: NewExecuteAction(defaultBridgeClientFactory),
		},
	}
}

// SetBridgeClientFactoryForTest overrides the bridge client factory used by newly-created
// Remote Queries bundles. It is intended for tests that exercise the registered PAR
// registry/runner path without depending on a live Agent IPC server.
func SetBridgeClientFactoryForTest(factory BridgeClientFactory) func() {
	previousFactory := defaultBridgeClientFactory
	defaultBridgeClientFactory = factory
	return func() {
		defaultBridgeClientFactory = previousFactory
	}
}

func (b *RemoteQueriesBundle) GetAction(actionName string) types.Action {
	return b.actions[actionName]
}
