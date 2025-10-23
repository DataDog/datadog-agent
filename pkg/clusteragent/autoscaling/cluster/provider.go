// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"
)

func StartClusterAutoscaling(
	ctx context.Context,
	clusterID string,
	clusterName string,
	isLeaderFunc func() bool,
	apiCl *apiserver.APIClient,
	rcClient RcClient,
	senderManager sender.SenderManager,
) error {

	if apiCl == nil {
		return fmt.Errorf("Impossible to start cluster autoscaling without valid APIClient")
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: apiCl.Cl.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "datadog-cluster-autoscaler"})

	store := autoscaling.NewStore[minNodePool]()

	clock := clock.RealClock{}

	_, err := NewConfigRetriever(ctx, clock, store, isLeaderFunc, rcClient)
	if err != nil {
		return fmt.Errorf("unable to start cluster autoscaling config retriever: %w", err)
	}

	sender, err := senderManager.GetSender("cluster_autoscaling")
	sender.DisableDefaultHostname(true)
	if err != nil {
		return fmt.Errorf("unable to start local telemetry for cluster autoscaling: %w", err)
	}

	controller, err := NewController(clock, clusterID, eventRecorder, rcClient, apiCl.DynamicInformerCl, apiCl.DynamicInformerFactory, isLeaderFunc, store, sender)
	if err != nil {
		return fmt.Errorf("unable to start cluster autoscaling controller: %w", err)
	}

	// Start informers & controllers
	apiCl.DynamicInformerFactory.Start(ctx.Done())
	apiCl.InformerFactory.Start(ctx.Done())

	go controller.Run(ctx)

	return nil
}
