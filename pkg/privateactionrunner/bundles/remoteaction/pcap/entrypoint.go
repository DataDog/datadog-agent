// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_pcap

import (
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// PcapBundle implements types.Bundle for com.datadoghq.remoteaction.pcap.
type PcapBundle struct {
	actions map[string]types.Action
}

// NewPcap constructs a PcapBundle and registers the runCapture action.
func NewPcap(eventPlatform eventplatform.Component) types.Bundle {
	return &PcapBundle{
		actions: map[string]types.Action{
			"runCapture": NewRunCaptureHandler(eventPlatform),
		},
	}
}

// GetAction returns the Action registered under actionName, or nil if not found.
func (b *PcapBundle) GetAction(actionName string) types.Action {
	return b.actions[actionName]
}
