// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"slices"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

// filterProcessPayloadsByECSCluster selects whole payloads using container metadata,
// independently of the processes collected or the hostname being validated.
func filterProcessPayloadsByECSCluster(payloads []*aggregator.ProcessPayload, clusterName string) []*aggregator.ProcessPayload {
	clusterTag := "ecs_cluster_name:" + clusterName
	var filtered []*aggregator.ProcessPayload
	for _, payload := range payloads {
		for _, container := range payload.Containers {
			if slices.Contains(container.Tags, clusterTag) {
				filtered = append(filtered, payload)
				break
			}
		}
	}
	return filtered
}
