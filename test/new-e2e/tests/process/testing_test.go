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

// marshalCheckOutput renders a payload the way `agent processchecks <check> --json` does, see
// printResultsJSON in pkg/cli/subcommands/processchecks
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
					// Populated since the service discovery data is collected on agent startup,
					// which is what made the check output undecodable, see incident #61151
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

// TestAssertManualProcessCheckWithServiceDiscovery asserts that a check output carrying service
// discovery resources, whose protobuf oneof encoding/json cannot unmarshal on its own, is decoded
// and asserted on successfully
func TestAssertManualProcessCheckWithServiceDiscovery(t *testing.T) {
	check := marshalCheckOutput(t, collectorProcWithServiceDiscovery())

	assertManualProcessCheck(t, check, true, "stress", "stress-container")
}

// TestUnmarshalManualProcessCheck asserts that decoding the check output preserves the payload,
// including the service discovery resources behind the oneof
func TestUnmarshalManualProcessCheck(t *testing.T) {
	payload := collectorProcWithServiceDiscovery()

	procs, err := unmarshalManualProcessCheck(marshalCheckOutput(t, payload))
	require.NoError(t, err)

	assert.Equal(t, payload.Processes, procs)
}

// TestUnmarshalManualProcessCheckWithoutServiceDiscovery asserts that processes reported without
// service discovery data, as on the platforms where it does not run, are decoded unchanged
func TestUnmarshalManualProcessCheckWithoutServiceDiscovery(t *testing.T) {
	payload := collectorProcWithServiceDiscovery()
	payload.Processes[0].ServiceDiscovery = nil

	procs, err := unmarshalManualProcessCheck(marshalCheckOutput(t, payload))
	require.NoError(t, err)

	assert.Equal(t, payload.Processes, procs)
}
