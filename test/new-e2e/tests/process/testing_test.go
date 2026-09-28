// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"encoding/json"
	"testing"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Match printResultsJSON, which uses encoding/json rather than protobuf JSON encoding.
func marshalCheckOutput(t *testing.T, payload agentmodel.MessageBody) string {
	t.Helper()

	out, err := json.MarshalIndent(payload, "", "  ")
	require.NoError(t, err)
	return string(out)
}

func collectorProcWithServiceDiscovery() *agentmodel.CollectorProc {
	return &agentmodel.CollectorProc{
		HostName: "test-host",
		Processes: []*agentmodel.Process{
			{
				Pid:     42,
				Command: &agentmodel.Command{Args: []string{"stress"}, Ppid: 1},
				User:    &agentmodel.ProcessUser{Name: "root"},
				Cpu:     &agentmodel.CPUStat{UserPct: 1.5},
				Memory:  &agentmodel.MemoryStat{Rss: 1024},
				IoStat:  &agentmodel.IOStat{WriteRate: 1, WriteBytesRate: 1024},
				ServiceDiscovery: &agentmodel.ServiceDiscovery{
					GeneratedServiceName: &agentmodel.ServiceName{Name: "stress"},
					ApmInstrumentation:   true,
					Resources: []*agentmodel.Resource{
						{Resource: &agentmodel.Resource_Logs{Logs: &agentmodel.LogResource{Path: "/var/log/stress.log"}}},
					},
				},
			},
		},
		Containers: []*agentmodel.Container{
			{Id: "container-id", Tags: []string{"container_name:stress-container"}},
		},
	}
}

func TestAssertManualProcessCheckWithServiceDiscovery(t *testing.T) {
	check := marshalCheckOutput(t, collectorProcWithServiceDiscovery())

	assertManualProcessCheck(t, check, true, "stress", "stress-container")
}

func TestUnmarshalManualProcessCheck(t *testing.T) {
	payload := collectorProcWithServiceDiscovery()

	procs, err := unmarshalManualProcessCheck(marshalCheckOutput(t, payload))
	require.NoError(t, err)

	assert.Equal(t, payload.Processes, procs)
}

func TestUnmarshalManualProcessCheckWithoutServiceDiscovery(t *testing.T) {
	payload := collectorProcWithServiceDiscovery()
	payload.Processes[0].ServiceDiscovery = nil

	procs, err := unmarshalManualProcessCheck(marshalCheckOutput(t, payload))
	require.NoError(t, err)

	assert.Equal(t, payload.Processes, procs)
}
