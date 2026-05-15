// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package provider

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/metadata/metadatainformer"
	"k8s.io/client-go/tools/cache"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/autoscalinggate"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/profile"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var namespaceGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}

// RegisterAutoscalingGateHandlers installs informer event handlers that enable
// the gate on the first observation of:
//   - a DatadogPodAutoscaler resource
//   - a supported workload with the autoscaling profile label
//   - a namespace with the autoscaling profile label
//
// DatadogPodAutoscalerClusterProfile is not a trigger because the autoscaling
// stack creates some OOTB profiles itself, which would make this always
// trigger once the stack has run.
func RegisterAutoscalingGateHandlers(
	ctx context.Context,
	dynamicInformer dynamicinformer.DynamicSharedInformerFactory,
	metadataClient metadata.Interface,
	workloadResources []profile.GroupVersionKindResource,
	gate *autoscalinggate.Gate,
) error {
	enable := func(_ any) { gate.Enable() }
	handlers := cache.ResourceEventHandlerFuncs{AddFunc: enable}

	// DPA trigger
	if _, err := dynamicInformer.ForResource(workload.PodAutoscalerGVR).Informer().AddEventHandler(handlers); err != nil {
		return fmt.Errorf("cannot add gate handler to DatadogPodAutoscaler informer: %w", err)
	}

	// Workload and namespace triggers
	labelFilteredFactory := metadatainformer.NewFilteredSharedInformerFactory(
		metadataClient,
		0,
		metav1.NamespaceAll,
		func(opts *metav1.ListOptions) {
			opts.LabelSelector = model.ProfileLabelKey
		},
	)

	for _, resource := range workloadResources {
		if _, err := labelFilteredFactory.ForResource(resource.GroupVersionResource).Informer().AddEventHandler(handlers); err != nil {
			return fmt.Errorf("cannot add gate handler to %s informer: %w", resource.GroupVersionResource, err)
		}
	}

	if _, err := labelFilteredFactory.ForResource(namespaceGVR).Informer().AddEventHandler(handlers); err != nil {
		return fmt.Errorf("cannot add gate handler to namespaces informer: %w", err)
	}

	labelFilteredFactory.Start(ctx.Done())

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
