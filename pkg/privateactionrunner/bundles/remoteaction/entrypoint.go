// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RemoteAction struct {
	actions map[string]types.Action
}

func NewRemoteAction() *RemoteAction {
	return &RemoteAction{
		actions: map[string]types.Action{
			"testConnection": NewTestConnectionHandler(),
		},
	}
}

func (h *RemoteAction) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
