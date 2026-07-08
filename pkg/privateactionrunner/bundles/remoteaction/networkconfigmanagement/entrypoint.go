// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package com_datadoghq_remoteaction_networkconfigmanagement provides PAR actions for network configuration management
package com_datadoghq_remoteaction_networkconfigmanagement

import (
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// NetworkConfigManagementBundle holds the actions for the network config management bundle
type NetworkConfigManagementBundle struct {
	actions map[string]types.Action
}

// NewNetworkConfigManagement creates a new network config management bundle
func NewNetworkConfigManagement(client ipc.HTTPClient) types.Bundle {
	return &NetworkConfigManagementBundle{
		actions: map[string]types.Action{
			"rollbackConfig": NewRollbackConfigHandler(client),
		},
	}
}

// GetAction returns the action with the given name
func (b *NetworkConfigManagementBundle) GetAction(name string) types.Action {
	return b.actions[name]
}
