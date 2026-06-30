// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package helmactionsimpl implements the helmactions component interface.
package helmactionsimpl

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	helmactions "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

// Requires defines the dependencies for the helmactions component.
type Requires struct {
	Lifecycle compdef.Lifecycle

	Log       log.Component
	Config    config.Component
	Hostname  hostnameinterface.Component
	APIClient *apiserver.APIClient
	Params    helmactions.Params
}

// Provides defines the output of the helmactions component.
type Provides struct {
	Comp helmactions.Component
}

type helmactionsImpl struct {
	log         log.Component
	config      config.Component
	apiClient   *apiserver.APIClient
	clusterID   string
	clusterName string
	params      helmactions.Params
	store       *ActionStore
	processor   *ActionProcessor
	watcher     *jobWatcher

	// Watcher lifecycle. watchCancel terminates the watch goroutine;
	// watchDone is closed once it has returned, letting stop() wait for it
	// without leaking the goroutine past container shutdown.
	watchCancel context.CancelFunc
	watchDone   chan struct{}
}

// NewComponent creates a new helmactions component.
func NewComponent(reqs Requires) (Provides, error) {
	ctx := context.Background()

	coreCl, ok := reqs.APIClient.Cl.CoreV1().(*corev1.CoreV1Client)
	if !ok {
		return Provides{}, fmt.Errorf("helmactions: unexpected CoreV1 client type %T", reqs.APIClient.Cl.CoreV1())
	}
	clusterID, err := common.GetOrCreateClusterID(coreCl)
	if err != nil {
		return Provides{}, fmt.Errorf("helmactions: get cluster ID: %w", err)
	}

	// clustername.GetClusterName needs the hostname as a fallback source for the
	// cluster name (it tries the apiserver / cloud provider / config first, then
	// falls back to host-based detection). An empty hostname is acceptable —
	// detection just skips that source.
	hostname, err := reqs.Hostname.Get(ctx)
	if err != nil {
		reqs.Log.Warnf("helmactions: hostname lookup failed, continuing with empty hostname: %v", err)
		hostname = ""
	}
	clusterName := clustername.GetClusterName(ctx, hostname)

	store := NewActionStore(ctx)
	reporter := NewResultReporter(clusterName, clusterID, store)

	comp := &helmactionsImpl{
		log:         reqs.Log,
		config:      reqs.Config,
		apiClient:   reqs.APIClient,
		clusterID:   clusterID,
		clusterName: clusterName,
		params:      reqs.Params,
		store:       store,
		processor:   NewActionProcessor(ctx, store, reporter),
		watcher:     newJobWatcher(reqs.APIClient.Cl, store),
	}

	reqs.Lifecycle.Append(compdef.Hook{OnStart: comp.start, OnStop: comp.stop})

	return Provides{Comp: comp}, nil
}

func (h *helmactionsImpl) start(_ context.Context) error {
	h.log.Infof("Starting helmactions component (clusterName=%s clusterID=%s)", h.clusterName, h.clusterID)

	// The watcher must outlive Fx's start context (which is bounded to startup
	// hooks), so derive a fresh context from Background and cancel it in stop().
	watchCtx, cancel := context.WithCancel(context.Background())
	h.watchCancel = cancel
	h.watchDone = make(chan struct{})
	go func() {
		defer close(h.watchDone)
		h.watcher.run(watchCtx)
	}()
	return nil
}

func (h *helmactionsImpl) stop(ctx context.Context) error {
	h.log.Info("Stopping helmactions component")
	if h.watchCancel != nil {
		h.watchCancel()
	}
	if h.watchDone != nil {
		// Wait for the goroutine to exit, bounded by the Fx stop context so a
		// stuck watcher can't hold up shutdown indefinitely.
		select {
		case <-h.watchDone:
		case <-ctx.Done():
			h.log.Warnf("helmactions: watcher did not exit before stop context cancellation: %v", ctx.Err())
		}
	}
	if h.store != nil {
		h.store.Stop()
	}
	return nil
}

// OnRollback records a newly-scheduled rollback Job in the store so the watcher
// can track its progress to completion. Called by the privateactionrunner
// rollback handler after it successfully creates the Job.
func (h *helmactionsImpl) OnRollback(job *batchv1.Job) {
	h.store.TrackJob(job)
}
