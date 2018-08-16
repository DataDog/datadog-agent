// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package clustername

import (
	// "fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	clusterName string
	initDone    = false
	mutex       = &sync.Mutex{}
)

func autoDiscoverClustername() {
	// TODO
}

func GetClustername() string {
	mutex.Lock()
	defer mutex.Unlock()

	if !initDone {
		clusterName = config.Datadog.GetString("cluster_name")
		if clusterName == "" {
			autoDiscoverClustername()
		}
		initDone = true
	}
	return clusterName
}
