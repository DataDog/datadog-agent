// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
)

// StartWorkloadAutoscaling starts the workload autoscaling controller
func StartWorkloadAutoscaling(ctx context.Context, apiCl *apiserver.APIClient, rcClient rcClient) (PODPatcher, error) {
	if apiCl == nil {
		return nil, fmt.Errorf("Impossible to start workload autoscaling without valid APIClient")
	}

	le, err := leaderelection.GetLeaderEngine()
	if err != nil {
		return nil, fmt.Errorf("Unable to start workload autoscaling as LeaderElection failed with: %v", err)
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: apiCl.Cl.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "datadog-workload-autoscaler"})

	store := autoscaling.NewStore[model.PodAutoscalerInternal]()

	_, err = newConfigRetriever(store, le.IsLeader, rcClient)
	if err != nil {
		return nil, fmt.Errorf("Unable to start workload autoscaling config retriever: %w", err)
	}

	controller, err := newController(eventRecorder, apiCl.RESTMapper, apiCl.ScaleCl, apiCl.DynamicInformerCl, apiCl.DynamicInformerFactory, le.IsLeader, store)
	if err != nil {
		return nil, fmt.Errorf("Unable to start workload autoscaling controller: %w", err)
	}

	// Start informers & controllers (informers can be started multiple times)
	apiCl.DynamicInformerFactory.Start(ctx.Done())
	apiCl.InformerFactory.Start(ctx.Done())

	go controller.Run(ctx)

	return newPODPatcher(store, eventRecorder), nil
}
