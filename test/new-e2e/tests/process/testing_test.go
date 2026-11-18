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

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
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

func TestFilterProcessPayloadsByECSCluster(t *testing.T) {
	const clusterName = "ci-2257835388-ecs-fargate"
	newPayload := func(hostname string, tags ...string) *aggregator.ProcessPayload {
		return &aggregator.ProcessPayload{CollectorProc: agentmodel.CollectorProc{
			HostName: hostname,
			Containers: []*agentmodel.Container{
				{Tags: tags},
			},
		}}
	}

	// Other suites can run stress-ng too, and other Fargate tasks have the same hostname prefix.
	kindPayload := newPayload("ci-2257687005-kind-control-plane", "container_name:stress-ng")
	otherFargatePayload := newPayload("fargate_task:other-task", "container_name:stress-ng", "ecs_cluster_name:other-cluster")
	prefixMatchPayload := newPayload("fargate_task:other-run", "ecs_cluster_name:"+clusterName+"-other")
	wrongTagPayload := newPayload("other-host", "kube_cluster_name:"+clusterName)

	// Keep this run's payloads even without stress-ng or with an incorrect hostname.
	matchingPayload := newPayload("fargate_task:this-task", "container_name:datadog-agent", "ecs_cluster_name:"+clusterName)
	incorrectHostnamePayload := newPayload("incorrect-hostname", "ecs_cluster_name:"+clusterName)
	// Matching more than one container must not duplicate the payload.
	matchingPayload.Containers = append(matchingPayload.Containers, &agentmodel.Container{Tags: []string{"ecs_cluster_name:" + clusterName}})

	for _, tt := range []struct {
		name     string
		payloads []*aggregator.ProcessPayload
		want     []*aggregator.ProcessPayload
	}{
		{name: "empty intake"},
		{
			name:     "foreign payloads only",
			payloads: []*aggregator.ProcessPayload{kindPayload, otherFargatePayload, prefixMatchPayload, wrongTagPayload, {}},
		},
		{
			name:     "mixed runs",
			payloads: []*aggregator.ProcessPayload{kindPayload, matchingPayload, otherFargatePayload, incorrectHostnamePayload, prefixMatchPayload, wrongTagPayload},
			want:     []*aggregator.ProcessPayload{matchingPayload, incorrectHostnamePayload},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, filterProcessPayloadsByECSCluster(tt.payloads, clusterName))
		})
	}
}
