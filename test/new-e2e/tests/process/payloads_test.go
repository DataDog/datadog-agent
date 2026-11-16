// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"testing"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

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
