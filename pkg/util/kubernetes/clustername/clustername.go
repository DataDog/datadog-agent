// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package clustername

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/azure"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/gce"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type clusterNameData struct {
	clusterName string
	initDone    bool
	mutex       sync.Mutex
}

// Provider is a generic function to grab the clustername and return it
type Provider func() (string, error)

// ProviderCatalog holds all the various kinds of clustername providers
var ProviderCatalog = make(map[string]Provider)

func newClusterNameData() *clusterNameData {
	return &clusterNameData{}
}

var defaultClusterNameData *clusterNameData

func init() {
	defaultClusterNameData = newClusterNameData()
	ProviderCatalog["gce"] = gce.GetClusterName
	ProviderCatalog["azure"] = azure.GetClusterName
	ProviderCatalog["ec2"] = ec2.GetClusterName
}

func getClusterName(data *clusterNameData) string {
	data.mutex.Lock()
	defer data.mutex.Unlock()

	if !data.initDone {
		data.clusterName = config.Datadog.GetString("cluster_name")
		// autodiscover clustername through k8s providers' API
		if data.clusterName == "" {
			for cloudProvider, getClusterNameFunc := range ProviderCatalog {
				log.Debug("Trying to auto discover the clustername from the %s API...", cloudProvider)
				clusterName, err := getClusterNameFunc()
				if err != nil {
					log.Debug("Unable to auto discover the clustername from the %s API: %s", cloudProvider, err)
					// try the next cloud provider
					continue
				}
				if clusterName != "" {
					log.Debug("Using clustername %s auto discovered from the %s API", clusterName, cloudProvider)
					data.clusterName = clusterName
					break
				}
			}
		}
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
