// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import (
	agentModel "github.com/DataDog/agent-payload/v5/process"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CheckName is the cluster check name of the orchestrator check
var CheckName = "orchestrator"

// NodeTypes returns the current existing NodesTypes as a slice to iterate over.
func NodeTypes() []agentModel.K8SResource {
	resources := make([]agentModel.K8SResource, 0, len(agentModel.K8SResource_value))
	for _, v := range agentModel.K8SResource_value {
		r := agentModel.K8SResource(v)
		if r != agentModel.K8SResource_K8SRESOURCE_UNSPECIFIED {
			resources = append(resources, r)
		}
	}
	return resources
}

// Orchestrator returns the orchestrator name for a node type.
func Orchestrator(n agentModel.K8SResource) string {
	if name, ok := agentModel.K8SResource_name[int32(n)]; ok && name != agentModel.K8SResource_K8SRESOURCE_UNSPECIFIED.String() {
		return "k8s"
	}
	log.Errorf("Unknown NodeType %v", n)
	return ""

}

// TelemetryTags return tags used for telemetry.
func TelemetryTags(n agentModel.K8SResource) []string {
	if n.String() == "" {
		log.Errorf("Unknown NodeType %v", n)
		return []string{"unknown", "unknown"}
	}
	tags := getTelemetryTags(n)
	return tags
}

func getTelemetryTags(n agentModel.K8SResource) []string {
	return []string{
		Orchestrator(n),
		strings.ToLower(n.String()),
	}
}

// SetCacheStats sets the cache stats for each resource
func SetCacheStats(resourceListLen int, resourceMsgLen int, nodeType agentModel.K8SResource) {
	stats := CheckStats{
		CacheHits:   resourceListLen - resourceMsgLen,
		CacheMiss:   resourceMsgLen,
		K8SResource: nodeType,
	}
	KubernetesResourceCache.Set(BuildStatsKey(nodeType), stats, NoExpiration)
}
