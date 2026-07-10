// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package com_datadoghq_remoteaction_networkdevices provides PAR actions that run
// connectivity checks against network devices for NDM onboarding.
package com_datadoghq_remoteaction_networkdevices

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// NetworkDevicesBundle holds the actions for the network devices bundle.
type NetworkDevicesBundle struct {
	actions map[string]types.Action
}

// NewNetworkDevices creates a new network devices bundle.
func NewNetworkDevices(encryptionStore *encryptioncontext.Store) types.Bundle {
	return &NetworkDevicesBundle{
		actions: map[string]types.Action{
			"connectivityCheck": NewConnectivityCheckHandler(encryptionStore),
		},
	}
}

// GetAction returns the action with the given name.
func (b *NetworkDevicesBundle) GetAction(name string) types.Action {
	return b.actions[name]
}
