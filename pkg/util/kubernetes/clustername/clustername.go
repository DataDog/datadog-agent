// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package clustername

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	clusterName string
	initDone    = false
	mutex       = &sync.Mutex{}
)

func autoDiscoverClusterName() {
	// TODO
}

// GetClusterName returns a k8s cluster name if it exists, either directly specified or autodiscovered
func GetClusterName() string {
	mutex.Lock()
	defer mutex.Unlock()

	if !initDone {
		clusterName = config.Datadog.GetString("cluster_name")
		if clusterName == "" {
			autoDiscoverClusterName()
		}
		initDone = true
	}
	return clusterName
}
