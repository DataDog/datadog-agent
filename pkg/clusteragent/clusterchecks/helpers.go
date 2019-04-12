// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

const (
	kubeServiceIDPrefix  = "kube_service://"
	KubePodPrefix        = "kubernetes_pod://"
	kubeEndpointIDPrefix = "kube_endpoint://"
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

// isKubeServiceCheck checks if a config template represents to a service check
func isKubeServiceCheck(config integration.Config) bool {
	return strings.HasPrefix(config.Entity, kubeServiceIDPrefix)
}

// getServiceUID retrieves service UID from entity
func getServiceUID(config integration.Config) string {
	return strings.TrimLeft(config.Entity, kubeServiceIDPrefix)
}

// getPodEntity returns pod entity
func getPodEntity(podUID string) string {
	return fmt.Sprintf("%s%s", KubePodPrefix, podUID)
}

func getNameAndNamespaceFromADIDs(configs []integration.Config) (string, string) {
	for _, config := range configs {
		for _, adID := range config.ADIdentifiers {
			namespace, name := getNameAndNameSpaceFromString(adID)
			if namespace != "" && name != "" {
				return namespace, name
			}
		}
	}
	return "", ""
}

func getNameAndNameSpaceFromString(s string) (string, string) {
	if !strings.HasPrefix(s, kubeEndpointIDPrefix) {
		return "", ""
	}
	split := strings.Split(s, "/")
	if len(split) == 4 {
		return split[2], split[3]
	}
	return "", ""
}
