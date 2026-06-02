// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestNetnsConsistent(t *testing.T) {
	const (
		hostNetNS      = 4026531840 // init/host netns from the wrong-attribution event
		containerNetNS = 4026532385 // container netns from the correct-attribution event
	)

	tests := []struct {
		name       string
		packet     uint32
		process    uint32
		consistent bool
	}{
		{"matching container netns (correct event)", containerNetNS, containerNetNS, true},
		{"host packet vs container process (wrong event)", hostNetNS, containerNetNS, false},
		{"container packet vs host process", containerNetNS, hostNetNS, false},
		{"unknown packet netns is accepted", 0, containerNetNS, true},
		{"unknown process netns is accepted", hostNetNS, 0, true},
		{"both unknown is accepted", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.consistent, netnsConsistent(tt.packet, tt.process))
		})
	}
}

func TestIsFlowClassifierNetworkEvent(t *testing.T) {
	// events whose PID is resolved from the flow_pid map (TC classifier) are guarded
	for _, et := range []model.EventType{
		model.DNSEventType,
		model.IMDSEventType,
		model.RawPacketFilterEventType,
		model.RawPacketActionEventType,
		model.NetworkFlowMonitorEventType,
	} {
		assert.Truef(t, isFlowClassifierNetworkEvent(et), "%s should be guarded", et)
	}

	// syscall-based events carry a reliable current PID and must not be guarded
	for _, et := range []model.EventType{
		model.ConnectEventType,
		model.AcceptEventType,
		model.BindEventType,
		model.ExecEventType,
		model.FileOpenEventType,
	} {
		assert.Falsef(t, isFlowClassifierNetworkEvent(et), "%s should not be guarded", et)
	}
}

func TestProcessRealNetNS(t *testing.T) {
	assert.Equal(t, uint32(0), processRealNetNS(nil))

	entry := &model.ProcessCacheEntry{}
	entry.PIDContext.NetNS = 4026532385
	assert.Equal(t, uint32(4026532385), processRealNetNS(entry))
}
