// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clusterid gets the Kubernetes cluster ID from the environment or the
// Cluster Agent. It has few dependencies so that agents that only need the
// cluster ID do not pull in the cluster name detection of package clustername.
package clusterid

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EnvVar is the environment variable that sets the cluster ID, in setups where
// the Cluster Agent exposes it through a ConfigMap.
const EnvVar = "DD_ORCHESTRATOR_CLUSTER_ID"

func cacheKey() string {
	return cache.BuildAgentKey(constants.ClusterIDCacheKey)
}

// Cached returns the cluster ID cached by a previous successful lookup.
func Cached() (string, bool) {
	if cachedClusterID, found := cache.Cache.Get(cacheKey()); found {
		return cachedClusterID.(string), true
	}
	return "", false
}

// Get returns the Kubernetes cluster ID from the DD_ORCHESTRATOR_CLUSTER_ID env
// variable when it is set, otherwise from the Cluster Agent API. It must not be
// called from the Cluster Agent: see clustername.GetClusterID.
func Get() (string, error) {
	if clusterID, found := Cached(); found {
		return clusterID, nil
	}

	// in older setups the cluster ID was exposed as an env var from a configmap created by the cluster agent
	clusterID, found := os.LookupEnv(EnvVar)
	if !found {
		log.Debugf("Cluster ID env variable %s is missing, calling the Cluster Agent", EnvVar)

		dcaClient, err := clusteragent.GetClusterAgentClient()
		if err != nil {
			return "", err
		}
		clusterID, err = dcaClient.GetKubernetesClusterID()
		if err != nil {
			return "", err
		}
		log.Debugf("Cluster ID retrieved from the Cluster Agent, set to %s", clusterID)
	}

	if len(clusterID) != 36 {
		err := fmt.Errorf("Unexpected value for Cluster ID: %s, ignoring it", clusterID)
		return "", err
	}

	cache.Cache.Set(cacheKey(), clusterID, cache.NoExpiration)
	return clusterID, nil
}
