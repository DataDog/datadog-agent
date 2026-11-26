// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package configimpl

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clusterid"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	kubeClusterUIDMinRetry = 10 * time.Second
	kubeClusterUIDMaxRetry = 10 * time.Minute
)

// setupKubeClusterUID sets c.KubeClusterUID when the peer.k8s.* peer tags are
// enabled and the Agent runs in Kubernetes. The resolution stops with lc, when given.
func setupKubeClusterUID(c *config.AgentConfig, lc compdef.Lifecycle) {
	if !c.PeerTagsAggregation || !c.PeerTagsK8sScope || !env.IsFeaturePresent(env.Kubernetes) {
		return
	}
	r := newKubeClusterUIDResolver(clusterid.Get, kubeClusterUIDMinRetry, kubeClusterUIDMaxRetry)
	c.KubeClusterUID = r.get
	if lc != nil {
		lc.Append(compdef.Hook{OnStop: func(context.Context) error {
			r.stop()
			return nil
		}})
	}
}

// kubeClusterUIDResolver resolves the cluster UID in the background on first
// use, retrying with exponential backoff until it succeeds or is stopped, so
// that callers on the stats hot path never wait for the Cluster Agent.
type kubeClusterUIDResolver struct {
	getClusterID       func() (string, error)
	minRetry, maxRetry time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{} // closed when the resolution ended or, if it never started, on stop
	once   sync.Once     // starts the resolution, or is consumed by stop so that it never starts
	uid    atomic.Pointer[string]
	logged bool // whether the first failure was logged, only accessed by resolve
}

func newKubeClusterUIDResolver(getClusterID func() (string, error), minRetry, maxRetry time.Duration) *kubeClusterUIDResolver {
	ctx, cancel := context.WithCancel(context.Background())
	return &kubeClusterUIDResolver{
		getClusterID: getClusterID,
		minRetry:     minRetry,
		maxRetry:     maxRetry,
		ctx:          ctx,
		cancel:       cancel,
		done:         make(chan struct{}),
	}
}

// get returns the cluster UID, or "" while it is not resolved.
func (r *kubeClusterUIDResolver) get() string {
	if uid := r.uid.Load(); uid != nil {
		return *uid
	}
	r.once.Do(func() { go r.resolve() })
	return ""
}

// stop ends the resolution. A later get does not start it. It does not wait
// for an in-flight getClusterID call.
func (r *kubeClusterUIDResolver) stop() {
	r.cancel()
	r.once.Do(func() { close(r.done) })
}

func (r *kubeClusterUIDResolver) resolve() {
	defer close(r.done)
	retry := r.minRetry
	for {
		if r.ctx.Err() != nil {
			return
		}
		uid, err := r.getClusterID()
		if err == nil && uid != "" {
			log.Infof("Kubernetes cluster UID resolved to %s, adding the peer.k8s.* peer tags to trace stats", uid)
			r.uid.Store(&uid)
			return
		}
		if !r.logged {
			log.Infof("Unable to get the Kubernetes cluster UID, the peer.k8s.* peer tags are not added to trace stats until it is known. "+
				"They need %s or the Cluster Agent. Retrying in the background: %v", clusterid.EnvVar, err)
			r.logged = true
		} else {
			log.Debugf("Unable to get the Kubernetes cluster UID, retrying in %s: %v", retry, err)
		}
		select {
		case <-time.After(retry):
		case <-r.ctx.Done():
			return
		}
		retry = min(2*retry, r.maxRetry)
	}
}
