// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package clustername

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
)

type clusterNameData struct {
	clusterName string
	initDone    bool
	mutex       *sync.Mutex
}

func newClusterNameData() *clusterNameData {
	return &clusterNameData{}
}

var defaultClusterNameData *clusterNameData

func init() {
	defaultClusterNameData = newClusterNameData()
}

func getClusterName(data *clusterNameData) string {
	data.mutex.Lock()
	defer data.mutex.Unlock()

	if !data.initDone {
		data.clusterName = config.Datadog.GetString("cluster_name")
		// TODO: autodiscover clustername through k8s providers' API
		data.initDone = true
	}
	return data.clusterName
}

// GetClusterName returns a k8s cluster name if it exists, either directly specified or autodiscovered
func GetClusterName() string {
	return getClusterName(defaultClusterNameData)
}

func resetClusterName(data *clusterNameData) {
	data.mutex.Lock()
	defer data.mutex.Unlock()
	data.initDone = false
}

// ResetClusterName resets the clustername, which allows it to be detected again. Used for tests
func ResetClusterName() {
	resetClusterName(defaultClusterNameData)
}
