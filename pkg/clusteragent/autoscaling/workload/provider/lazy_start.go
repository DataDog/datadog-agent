// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package provider

import (
	"context"
	"fmt"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/autoscalinggate"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var podAutoscalerClusterProfileGVR = datadoghq.GroupVersion.WithResource("datadogpodautoscalerclusterprofiles")

// RegisterAutoscalingGateHandlers installs informer event handlers that enable
// the gate on the first observation of a DatadogPodAutoscaler or
// DatadogPodAutoscalerClusterProfile resource.
func RegisterAutoscalingGateHandlers(
	dynamicInformer dynamicinformer.DynamicSharedInformerFactory,
	gate *autoscalinggate.Gate,
) error {
	enable := func(_ any) { gate.Enable() }
	handlers := cache.ResourceEventHandlerFuncs{AddFunc: enable}

	// Only the first add has an effect
	if _, err := dynamicInformer.ForResource(workload.PodAutoscalerGVR).Informer().AddEventHandler(handlers); err != nil {
		return fmt.Errorf("cannot add gate handler to DatadogPodAutoscaler informer: %w", err)
	}
	if _, err := dynamicInformer.ForResource(podAutoscalerClusterProfileGVR).Informer().AddEventHandler(handlers); err != nil {
		return fmt.Errorf("cannot add gate handler to DatadogPodAutoscalerClusterProfile informer: %w", err)
	}
	return nil
}

// StartWorkloadAutoscalingOnGate waits for the autoscaling gate to be enabled
// before starting the workload autoscaling stack.
func StartWorkloadAutoscalingOnGate(
	ctx context.Context,
	gate *autoscalinggate.Gate,
	clusterID, clusterName string,
	isLeader func() bool,
	apiCl *apiserver.APIClient,
	rcClient workload.RcClient,
	wlm workloadmeta.Component,
	taggerComp tagger.Component,
	senderManager sender.SenderManager,
	webhook *autoscaling.Webhook,
) {
	if !gate.WaitForEnable(ctx) || ctx.Err() != nil {
		return
	}

	if !gate.WaitForPodCollectionSynced(ctx) || ctx.Err() != nil {
		return
	}

	log.Info("Workload autoscaling gate synced, starting autoscaling stack")

	patcher, err := StartWorkloadAutoscaling(ctx, clusterID, clusterName, isLeader, apiCl, rcClient, wlm, taggerComp, senderManager)
	if err != nil {
		log.Errorf("Failed to start workload autoscaling stack: %v", err)
		return
	}

	if webhook != nil {
		webhook.SetPatcher(patcher)
	}
}
