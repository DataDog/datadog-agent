// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_networks

import (
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type NetworksBundle struct {
	actions map[string]types.Action
}

func NewNetworks(traceroute traceroute.Component, eventPlatform eventplatform.Component) types.Bundle {
	return &NetworksBundle{
		actions: map[string]types.Action{
			"runNetworkPath": NewRunNetworkPathHandler(traceroute, eventPlatform),
		},
	}
}

func (h *NetworksBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
