// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemprobeimpl

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/compliance/statusregistry"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestGetStatusDetailsIncludesSystemProbeDetailsWithoutCompliance(t *testing.T) {
	remoteAgent := &remoteagentImpl{
		getModuleStats: func() map[string]any {
			return map[string]any{
				"uptime":     "42s",
				"updated_at": float64(1),
				"process":    map[string]interface{}{"running": true},
			}
		},
	}
	t.Cleanup(func() {
		statusregistry.Set(nil)
	})
	statusregistry.Set(nil)

	response, err := remoteAgent.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})
	require.NoError(t, err)
	require.Contains(t, response.NamedSections, "Details")

	details := response.NamedSections["Details"].Fields[""]
	assert.Contains(t, details, "Status: Running")
	assert.Contains(t, details, "Uptime: 42s")
	assert.Contains(t, details, "Process")
	assert.NotContains(t, response.NamedSections, "Compliance")
}

func TestGetStatusDetailsPreservesCompliance(t *testing.T) {
	remoteAgent := &remoteagentImpl{
		getModuleStats: func() map[string]any {
			return map[string]any{
				"uptime":     "42s",
				"updated_at": float64(1),
			}
		},
	}
	t.Cleanup(func() {
		statusregistry.Set(nil)
	})
	statusregistry.Set(func() (string, error) {
		return "compliance status", nil
	})

	response, err := remoteAgent.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})
	require.NoError(t, err)
	require.Contains(t, response.NamedSections, "Details")
	require.Contains(t, response.NamedSections, "Compliance")
	assert.Equal(t, "compliance status", response.NamedSections["Compliance"].Fields[""])
}

func TestGetStatusDetailsDegradesComplianceRenderingError(t *testing.T) {
	remoteAgent := &remoteagentImpl{
		getModuleStats: func() map[string]any {
			return map[string]any{
				"uptime":     "42s",
				"updated_at": float64(1),
				"process":    map[string]interface{}{"running": true},
			}
		},
	}
	t.Cleanup(func() {
		statusregistry.Set(nil)
	})
	statusregistry.Set(func() (string, error) {
		return "", errors.New("render failed")
	})

	response, err := remoteAgent.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})
	require.NoError(t, err)
	require.Contains(t, response.NamedSections, "Details")

	details := response.NamedSections["Details"].Fields[""]
	assert.Contains(t, details, "Status: Running")
	assert.Contains(t, details, "Uptime: 42s")
	assert.Contains(t, details, "Process")
	assert.NotContains(t, response.NamedSections, "Compliance")
}
