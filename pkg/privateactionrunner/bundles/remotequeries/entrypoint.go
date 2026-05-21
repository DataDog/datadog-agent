// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remotequeries

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

const (
	// BundleID is the local-only Remote Queries PAR bundle FQN.
	BundleID = "com.datadoghq.remotequeries"

	// ExecuteActionName is the action part of com.datadoghq.remotequeries.execute.
	ExecuteActionName = "execute"
)

type RemoteQueriesBundle struct {
	actions map[string]types.Action
}

func NewRemoteQueriesBundle() *RemoteQueriesBundle {
	return &RemoteQueriesBundle{
		actions: map[string]types.Action{
			ExecuteActionName: NewExecuteAction(NewDefaultBridgeClient),
		},
	}
}

func (b *RemoteQueriesBundle) GetAction(actionName string) types.Action {
	return b.actions[actionName]
}
