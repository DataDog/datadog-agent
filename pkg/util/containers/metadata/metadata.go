// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containers provides metadata for containers.
package containers

import (
	"maps"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers/cri"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type metadataProvider func() (map[string]string, error)

var (
	catalog = map[string]metadataProvider{
		"cri":     cri.GetMetadata,
		"docker":  docker.GetMetadata,
		"kubelet": kubelet.GetMetadata,
	}
)

// Get returns the metadata for the different container runtime the agent support (CRI, Docker and Kubelet)
func Get(timeout time.Duration) map[string]string {
	wg := sync.WaitGroup{}
	containerMeta := make(map[string]string)
	// protecting the above map from concurrent access
	mutex := &sync.Mutex{}

	for provider, getMeta := range catalog {
		wg.Add(1)
		go func(provider string, getMeta metadataProvider) {
			defer wg.Done()
			meta, err := getMeta()
			if err != nil {
				log.Debugf("Unable to get %s metadata: %s", provider, err)
				return
			}
			mutex.Lock()
			maps.Copy(containerMeta, meta)
			mutex.Unlock()
		}(provider, getMeta)
	}
	// we want to timeout even if the wait group is not done yet
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return containerMeta
	case <-time.After(timeout):
		// in this case the map might be incomplete so return a copy to avoid race
		incompleteMeta := make(map[string]string)
		mutex.Lock()
		maps.Copy(incompleteMeta, containerMeta)
		mutex.Unlock()
		return incompleteMeta
	}
}
