// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package provider

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/external"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/local"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

const maxDatadogPodAutoscalerObjects int = 100

// StartWorkloadAutoscaling starts the workload autoscaling controller
func StartWorkloadAutoscaling(
	ctx context.Context,
	clusterID string,
	clusterName string,
	isLeaderFunc func() bool,
	apiCl *apiserver.APIClient,
	rcClient workload.RcClient,
	wlm workloadmeta.Component,
	senderManager sender.SenderManager,
) (workload.PodPatcher, error) {
	if apiCl == nil {
		return nil, fmt.Errorf("Impossible to start workload autoscaling without valid APIClient")
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: apiCl.Cl.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "datadog-workload-autoscaler"})

	store := autoscaling.NewStore[model.PodAutoscalerInternal]()
	podPatcher := workload.NewPodPatcher(store, isLeaderFunc, apiCl.DynamicCl, eventRecorder)
	podWatcher := workload.NewPodWatcher(wlm, podPatcher)

	_, err := workload.NewConfigRetriever(store, isLeaderFunc, rcClient)
	if err != nil {
		return nil, fmt.Errorf("Unable to start workload autoscaling config retriever: %w", err)
	}

	// We could consider the sender to be optional, but it's actually required for backend information
	sender, err := senderManager.GetSender("workload_autoscaling")
	sender.DisableDefaultHostname(true)
	if err != nil {
		return nil, fmt.Errorf("Unable to start local telemetry for autoscaling: %w", err)
	}

	limitHeap := autoscaling.NewHashHeap(maxDatadogPodAutoscalerObjects, store)

	controller, err := workload.NewController(clusterID, eventRecorder, apiCl.RESTMapper, apiCl.ScaleCl, apiCl.DynamicInformerCl, apiCl.DynamicInformerFactory, isLeaderFunc, store, podWatcher, sender, limitHeap)
	if err != nil {
		return nil, fmt.Errorf("Unable to start workload autoscaling controller: %w", err)
	}

	// Start informers & controllers (informers can be started multiple times)
	apiCl.DynamicInformerFactory.Start(ctx.Done())
	apiCl.InformerFactory.Start(ctx.Done())

	// TODO: Wait POD Watcher sync before running the controller
	go podWatcher.Run(ctx)
	go controller.Run(ctx)

	// Only start the local recommender if failover metrics collection is enabled
	if pkgconfigsetup.Datadog().GetBool("autoscaling.failover.enabled") {
		localRecommender := local.NewRecommender(podWatcher, store)
		go localRecommender.Run(ctx)
	}

	externalRecommender := external.NewRecommender(podWatcher, store, clusterName)
	go externalRecommender.Run(ctx)

	return podPatcher, nil
}
