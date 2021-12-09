// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver

package common

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
)

// apiDiscovery is a local struct adapted to the agent retry package.
// It allow retrieving the kubernetes api groups and resources with a retrier.
type apiDiscovery struct {
	retrier       retry.Retrier
	discoveryFunc func() ([]*v1.APIGroup, []*v1.APIResourceList, error)
	groups        []*v1.APIGroup
	resources     []*v1.APIResourceList
}

// fetch is a retriable method to retrieve the kubernetes api groups and resources.
func (a *apiDiscovery) fetch() error {
	var err error
	a.groups, a.resources, err = a.discoveryFunc()

	return err
}

// newAPIDiscovery returns a new apiDiscovery
func newAPIDiscovery(discoveryFunc func() ([]*v1.APIGroup, []*v1.APIResourceList, error)) (*apiDiscovery, error) {
	apis := &apiDiscovery{discoveryFunc: discoveryFunc}

	return apis, apis.retrier.SetupRetrier(&retry.Config{
		Name:              "kubeAPIDiscovery",
		AttemptMethod:     apis.fetch,
		Strategy:          retry.Backoff,
		InitialRetryDelay: 1 * time.Second,
		MaxRetryDelay:     3 * time.Second,
	})
}

// KubeGroupsAndResources returns the available kubernetes api groups and resources using a Discovery Client.
// It retries with an exponential backoff until timeout (60 seconds by default).
func KubeGroupsAndResources(discoveryCl discovery.DiscoveryInterface) ([]*v1.APIGroup, []*v1.APIResourceList, error) {
	apis, err := newAPIDiscovery(discoveryCl.ServerGroupsAndResources)
	if err != nil {
		return nil, nil, err
	}

	retryTimeout := time.Duration(config.Datadog.GetInt64("kubernetes_discovery_client_timeout")) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), retryTimeout)
	defer cancel()

	start := time.Now()

	for {
		err := apis.retrier.TriggerRetry()
		switch apis.retrier.RetryStatus() {
		case retry.OK:
			log.Infof("Got API groups and resources after %s", time.Since(start))
			return apis.groups, apis.resources, nil
		case retry.PermaFail:
			return nil, nil, err
		default:
			sleepFor := apis.retrier.NextRetry().UTC().Sub(time.Now().UTC()) + time.Second
			log.Debugf("Waiting for getting Kubernetes API groups and resources, next retry in %s", sleepFor)
			select {
			case <-ctx.Done():
				return nil, nil, fmt.Errorf("timeout %s reached while waiting for Kubernetes API groups and resources, last error: %w", retryTimeout, err)
			case <-time.After(sleepFor):
			}
		}
	}
}
