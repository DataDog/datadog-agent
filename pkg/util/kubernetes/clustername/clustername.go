// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package clustername

import (
	"fmt"
	"os"
	"regexp"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/gce"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	clusterIDCacheKey = "clusterID"
	clusterIDFilePath = "/etc/datadog-agent/kube_cluster/id"
)

var (
	// validClusterName matches exactly the same naming rule as the one enforced by GKE:
	// https://cloud.google.com/kubernetes-engine/docs/reference/rest/v1beta1/projects.locations.clusters#Cluster.FIELDS.name
	// The cluster name can be up to 40 characters with the following restrictions:
	// * Lowercase letters, numbers, dots and hyphens only.
	// * Must start with a letter.
	// * Must end with a number or a letter.
	validClusterName = regexp.MustCompile(`^([a-z]([a-z0-9\-]*[a-z0-9])?\.)*([a-z]([a-z0-9\-]*[a-z0-9])?)$`)
)

type clusterNameData struct {
	clusterName string
	initDone    bool
	mutex       sync.Mutex
}

// Provider is a generic function to grab the clustername and return it
type Provider func() (string, error)

// ProviderCatalog holds all the various kinds of clustername providers
var ProviderCatalog map[string]Provider

func newClusterNameData() *clusterNameData {
	return &clusterNameData{}
}

var defaultClusterNameData *clusterNameData

func init() {
	defaultClusterNameData = newClusterNameData()
	ProviderCatalog = map[string]Provider{
		"gce":   gce.GetClusterName,
		"azure": azure.GetClusterName,
		"ec2":   ec2.GetClusterName,
	}
}

func getClusterName(data *clusterNameData) string {
	data.mutex.Lock()
	defer data.mutex.Unlock()

	if !data.initDone {
		data.clusterName = config.Datadog.GetString("cluster_name")
		if data.clusterName != "" {
			log.Infof("Got cluster name %s from config", data.clusterName)
			if !validClusterName.MatchString(data.clusterName) || len(data.clusterName) > 40 {
				log.Errorf("%q isn’t a valid cluster name. It must be dot-separated tokens where tokens "+
					"start with a lowercase letter followed by up to 39 lowercase letters, numbers, or "+
					"hyphens, and cannot end with a hyphen nor have a dot adjacent to a hyphen.", data.clusterName)
				log.Errorf("As a consequence, the cluster name provided by the config will be ignored")
				data.clusterName = ""
			}
		}

		// autodiscover clustername through k8s providers' API
		if data.clusterName == "" {
			for cloudProvider, getClusterNameFunc := range ProviderCatalog {
				log.Debugf("Trying to auto discover the cluster name from the %s API...", cloudProvider)
				clusterName, err := getClusterNameFunc()
				if err != nil {
					log.Debugf("Unable to auto discover the cluster name from the %s API: %s", cloudProvider, err)
					// try the next cloud provider
					continue
				}
				if clusterName != "" {
					log.Infof("Using cluster name %s auto discovered from the %s API", clusterName, cloudProvider)
					data.clusterName = clusterName
					break
				}
			}
		}

		if data.clusterName == "" {
			clusterName, err := hostinfo.GetNodeClusterNameLabel()
			if err != nil {
				log.Debugf("Unable to auto discover the cluster name from node label : %s", err)
			} else {
				data.clusterName = clusterName
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

// GetClusterID looks for a fixed path where the file containing the clusterID should be
// this file should come from a configmap, written by the cluster-agent
func GetClusterID() (string, error) {
	cacheClusterIDKey := cache.BuildAgentKey(clusterIDCacheKey)
	if cachedClusterID, found := cache.Cache.Get(cacheClusterIDKey); found {
		return cachedClusterID.(string), nil
	}

	clusterID, err := readClusterIDFile(clusterIDFilePath)
	if err != nil {
		return "", err
	}

	cache.Cache.Set(cacheClusterIDKey, clusterID, cache.NoExpiration)
	return clusterID, nil
}

func readClusterIDFile(path string) (string, error) {
	buf := make([]byte, 36)
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}

	count, err := f.Read(buf)
	if err != nil {
		return "", err
	} else if count < 36 {
		return "", fmt.Errorf("content from %s doesn't look like a UUID, ignoring it", path)
	}

	return string(buf), nil
}
