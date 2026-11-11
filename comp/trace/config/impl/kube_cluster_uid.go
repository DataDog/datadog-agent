// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package configimpl

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	kubeClusterUIDMinRetry = 10 * time.Second
	kubeClusterUIDMaxRetry = 10 * time.Minute
)

// kubeClusterUIDFunc returns the getter for config.AgentConfig.KubeClusterUID,
// or nil when the Agent does not run in Kubernetes.
func kubeClusterUIDFunc() func() string {
	if !env.IsFeaturePresent(env.Kubernetes) {
		return nil
	}
	r := &kubeClusterUIDResolver{
		getClusterID: clustername.GetClusterID,
		minRetry:     kubeClusterUIDMinRetry,
		maxRetry:     kubeClusterUIDMaxRetry,
	}
	return r.get
}

// kubeClusterUIDResolver resolves the cluster UID in the background on first
// use, retrying with exponential backoff until it succeeds, so that callers on
// the stats hot path never wait for the Cluster Agent.
type kubeClusterUIDResolver struct {
	getClusterID       func() (string, error)
	minRetry, maxRetry time.Duration

	once sync.Once
	uid  atomic.Pointer[string]
}

// get returns the cluster UID, or "" while it is not resolved.
func (r *kubeClusterUIDResolver) get() string {
	if uid := r.uid.Load(); uid != nil {
		return *uid
	}
	r.once.Do(func() { go r.resolve() })
	return ""
}

func (r *kubeClusterUIDResolver) resolve() {
	retry := r.minRetry
	for {
		uid, err := r.getClusterID()
		if err == nil && uid != "" {
			log.Infof("Kubernetes cluster UID resolved to %s, adding the Kubernetes scope to peer tags", uid)
			r.uid.Store(&uid)
			return
		}
		log.Debugf("Unable to get the Kubernetes cluster UID, retrying in %s: %v", retry, err)
		time.Sleep(retry)
		retry = min(2*retry, r.maxRetry)
	}
}
