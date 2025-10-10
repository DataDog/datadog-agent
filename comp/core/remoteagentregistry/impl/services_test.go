// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package remoteagentregistryimpl implements the remoteagentregistry component interface
package remoteagentregistryimpl

import (
	"context"
	"testing"

	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	helpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
)

func TestGetRegisteredAgentStatuses(t *testing.T) {
	provides, _, _, _, ipcComp := buildComponent(t)
	component := provides.Comp.(*remoteAgentRegistry)

	_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "test-agent", "Test Agent", "123",
		withStatusProvider(map[string]string{
			"test_key": "test_value",
		}, nil),
	)

	statuses := component.GetRegisteredAgentStatuses()
	t.Logf("statuses: %v\n", statuses[0])
	require.Len(t, statuses, 1)
	require.Equal(t, "test-agent", statuses[0].Flavor)
	require.Equal(t, "Test Agent", statuses[0].DisplayName)
	require.Equal(t, "test_value", statuses[0].MainSection["test_key"])
}

func TestFlareProvider(t *testing.T) {
	provides, _, _, _, ipcComp := buildComponent(t)
	component := provides.Comp
	flareProvider := provides.FlareProvider

	remoteAgent := buildAndRegisterRemoteAgent(t, ipcComp, component, "test-agent", "Test Agent", "123",
		WithFlareProvider(map[string][]byte{
			"test_file.yaml": []byte("test_content"),
		}),
	)

	fb := helpers.NewFlareBuilderMock(t, false)
	fb.AssertNoFileExists("test-agent/test_file.yaml")

	err := flareProvider.FlareFiller.Callback(fb)
	require.NoError(t, err)

	flareFilePath := remoteAgent.RegistrationData.AgentDisplayName + "-" + remoteAgent.RegistrationData.AgentFlavor + "-" + remoteAgent.registeredSessionID + "/test_file.yaml"
	fb.AssertFileExists(flareFilePath)
	fb.AssertFileContent("test_content", flareFilePath)
}

func TestGetTelemetry(t *testing.T) {
	provides, lc, _, telemetry, ipcComp := buildComponent(t)
	lc.Start(context.Background())
	component := provides.Comp

	promText := `
		# HELP foobar foobarhelp
		# TYPE foobar counter
		foobar 1
		# HELP baz bazhelp
		# TYPE baz gauge
		baz{tag_one="1",tag_two="two"} 3
		`

	_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "test-agent", "Test Agent", "123",
		withTelemetryProvider(promText),
	)

	metrics, err := telemetry.Gather(false)
	require.NoError(t, err)
	assert.Contains(t, metrics, &io_prometheus_client.MetricFamily{
		Name: proto.String("foobar"),
		Type: io_prometheus_client.MetricType_COUNTER.Enum(),
		Help: proto.String("foobarhelp"),
		Metric: []*io_prometheus_client.Metric{
			{
				Counter: &io_prometheus_client.Counter{
					Value: proto.Float64(1),
				},
			},
		},
	})

	// assert.Contains does not work here because of the labels
	bazMetric := func() *io_prometheus_client.MetricFamily {
		for _, m := range metrics {
			if m.GetName() == "baz" {
				return m
			}
		}
		return nil
	}()
	require.NotNil(t, bazMetric)
	assert.Equal(t, bazMetric.GetType(), io_prometheus_client.MetricType_GAUGE)
	assert.Equal(t, bazMetric.GetMetric()[0].GetGauge().GetValue(), 3.0)
	assert.Equal(t, bazMetric.GetMetric()[0].GetLabel()[0].GetValue(), "1")
	assert.Equal(t, bazMetric.GetMetric()[0].GetLabel()[1].GetValue(), "two")
}

func TestStatusProvider(t *testing.T) {
	provides, _, _, _, ipcComp := buildComponent(t)
	component := provides.Comp
	statusProvider := provides.Status

	_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "test-agent", "Test Agent", "1234",
		withStatusProvider(map[string]string{
			"test_key": "test_value",
		}, nil),
	)

	statusData := make(map[string]interface{})
	err := statusProvider.Provider.JSON(false, statusData)
	require.NoError(t, err)

	require.Len(t, statusData, 2)

	registeredAgents, ok := statusData["registeredAgents"].([]remoteagent.RegisteredAgent)
	if !ok {
		t.Fatalf("registeredAgents is not a slice of RegisteredAgent")
	}
	require.Len(t, registeredAgents, 1)
	require.Equal(t, "Test Agent", registeredAgents[0].DisplayName)

	registeredAgentStatuses, ok := statusData["registeredAgentStatuses"].([]remoteagent.StatusData)
	if !ok {
		t.Fatalf("registeredAgentStatuses is not a slice of StatusData")
	}
	require.Len(t, registeredAgentStatuses, 1)
	require.Equal(t, "test-agent", registeredAgentStatuses[0].Flavor)
	require.Equal(t, "1234", registeredAgentStatuses[0].PID)
	require.Equal(t, "Test Agent", registeredAgentStatuses[0].DisplayName)
	require.Equal(t, "test_value", registeredAgentStatuses[0].MainSection["test_key"])
}
