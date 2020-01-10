// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

const (
	kubeServiceIDPrefix      = "kube_service_uid://"
	kubeEndpointIDPrefix     = "kube_endpoint_uid://"
	checkExecutionTimeWeight = 0.8
	checkMetricSamplesWeight = 0.2
)

// makeConfigArray flattens a map of configs into a slice. Creating a new slice
// allows for thread-safe usage by other external, as long as the field values in
// the config objects are not modified.
func makeConfigArray(configMap map[string]integration.Config) []integration.Config {
	configSlice := make([]integration.Config, 0, len(configMap))
	for _, c := range configMap {
		configSlice = append(configSlice, c)
	}
	return configSlice
}

// timestampNow provides a consistent way to keep a seconds timestamp
func timestampNow() int64 {
	return time.Now().Unix()
}

// isKubeServiceCheck checks if a config template represents a service check
func isKubeServiceCheck(config integration.Config) bool {
	return strings.HasPrefix(config.Entity, kubeServiceIDPrefix)
}

// getServiceUID retrieves service UID from service config
func getServiceUID(config integration.Config) string {
	return strings.TrimLeft(config.Entity, kubeServiceIDPrefix)
}

// getPodEntity returns pod entity
func getPodEntity(podUID string) string {
	return kubelet.KubePodPrefix + podUID
}

// getNameAndNamespaceFromADIDs extracts namespace
// and name from endpoints configs AD identifiers.
func getNameAndNamespaceFromADIDs(configs []integration.Config) (string, string) {
	for _, config := range configs {
		for _, adID := range config.ADIdentifiers {
			namespace, name := getNameAndNamespaceFromEntity(adID)
			if namespace != "" && name != "" {
				// All configs in the slice share the same namespace and name
				// and contain the same kube_endpoint AD identifier.
				// Return the first valid namespace and name found.
				return namespace, name
			}
		}
	}
	return "", ""
}

// getNameAndNamespaceFromEntity parses endpoints entity
// string to extract namespace and name.
func getNameAndNamespaceFromEntity(s string) (string, string) {
	if !strings.HasPrefix(s, kubeEndpointIDPrefix) {
		return "", ""
	}
	split := strings.Split(s, "/") // Format: kube_endpoint_uid://namespace/name
	if len(split) == 4 {
		return split[2], split[3]
	}
	return "", ""
}

// calculateBusyness returns the busyness value of a node
func calculateBusyness(checkStats types.CLCRunnersStats) int {
	busyness := 0
	for _, stats := range checkStats {
		busyness += busynessFunc(stats.AverageExecutionTime, stats.MetricSamples)
	}
	return busyness
}

// busynessFunc returns the weight of a check
func busynessFunc(avgExecTime, mSamples int) int {
	return int(checkExecutionTimeWeight*float64(avgExecTime) + checkMetricSamplesWeight*float64(mSamples))
}

// orderedKeys sorts the keys of a map and return them in a slice
func orderedKeys(m map[string]int) []string {
	keys := []string{}
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
