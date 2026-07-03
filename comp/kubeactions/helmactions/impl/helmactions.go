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
	"sync"

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
	jobWatcher  *jobWatcher
	podWatcher  *podWatcher

	watchDone chan struct{}
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
		jobWatcher:  newJobWatcher(reqs.APIClient.Cl, store),
		podWatcher:  newPodWatcher(reqs.APIClient.Cl, store),
	}

	reqs.Lifecycle.Append(compdef.Hook{OnStart: comp.start, OnStop: comp.stop})

	return Provides{Comp: comp}, nil
}

func (h *helmactionsImpl) start(ctx context.Context) error {
	h.log.Infof("Starting helmactions component (clusterName=%s clusterID=%s)", h.clusterName, h.clusterID)

	h.processor.run(ctx)

	h.watchDone = make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); h.jobWatcher.run(ctx) }()
	go func() { defer wg.Done(); h.podWatcher.run(ctx) }()
	go func() {
		wg.Wait()
		close(h.watchDone)
	}()
	return nil
}

func (h *helmactionsImpl) stop(ctx context.Context) error {
	h.log.Info("Stopping helmactions component")

	if h.watchDone != nil {
		// Wait for the goroutine to exit, bounded by the Fx stop context so a
		// stuck watcher can't hold up shutdown indefinitely.
		select {
		case <-h.watchDone:

		// todo: is it the same ctx that is on start ?
		case <-ctx.Done():
			h.log.Warnf("helmactions: watcher did not exit before stop context cancellation: %v", ctx.Err())
		}
	}
	if h.store != nil {
		h.store.Stop()
	}

	return ctx.Err()
}

// OnRollback records a newly-scheduled rollback Job in the store so the watcher
// can track its progress to completion. Called by the privateactionrunner
// rollback handler after it successfully creates the Job.
func (h *helmactionsImpl) OnRollback(in *helmactions.RollbackInputs, job *batchv1.Job) {
	h.processor.OnRollbackAction(in, job)
}
