// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_networkpath

import (
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// NetworkPath is a bundle that provides network path actions.
type NetworkPath struct {
	actions map[string]types.Action
}

// NewNetworkPath creates a new NetworkPath bundle with the given traceroute component.
func NewNetworkPath(traceroute traceroute.Component) *NetworkPath {
	return &NetworkPath{
		actions: map[string]types.Action{
			"traceroute": NewTracerouteHandler(traceroute),
		},
	}
}

// GetAction returns an action by name.
func (h *NetworkPath) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
