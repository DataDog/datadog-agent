// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

import (
	"strconv"
	"sync"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/network"
)

func TestConnectionModelerAgentConfiguration(t *testing.T) {
	tests := []struct {
		npm, usm, ccm, csm bool
	}{
		{false, false, false, false},
		{false, false, true, false},
		{false, true, false, false},
		{false, true, true, false},
		{true, false, false, false},
		{true, false, true, false},
		{true, true, false, false},
		{true, true, true, false},
		{false, false, false, true},
		{false, false, true, true},
		{false, true, false, true},
		{false, true, true, true},
		{true, false, false, true},
		{true, false, true, true},
		{true, true, false, true},
		{true, true, true, true},
	}

	for _, te := range tests {
		t.Run("", func(t *testing.T) {
			t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLED", strconv.FormatBool(te.npm))
			t.Setenv("DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED", strconv.FormatBool(te.usm))
			t.Setenv("DD_CCM_NETWORK_CONFIG_ENABLED", strconv.FormatBool(te.ccm))
			t.Setenv("DD_RUNTIME_SECURITY_CONFIG_ENABLED", strconv.FormatBool(te.csm))
			mock.NewSystemProbe(t)
			cfgOnce = sync.Once{}
			conns := &network.Connections{}
			mod := NewConnectionsModeler(conns)
			streamer := NewProtoTestStreamer[*model.Connections]()
			builder := model.NewConnectionsBuilder(streamer)
			expected := &model.AgentConfiguration{
				CcmEnabled: te.ccm,
				CsmEnabled: te.csm,
				UsmEnabled: te.usm,
				NpmEnabled: te.npm,
			}

			mod.modelConnections(builder, conns)

			actual := streamer.Unwrap(t, &model.Connections{})
			assert.Equal(t, expected, actual.AgentConfiguration)
		})
	}
}
