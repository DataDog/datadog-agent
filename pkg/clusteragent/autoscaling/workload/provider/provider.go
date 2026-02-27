// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package provider

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"

	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/external"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/local"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// StartWorkloadAutoscaling starts the workload autoscaling controller
func StartWorkloadAutoscaling(
	ctx context.Context,
	clusterID string,
	clusterName string,
	isLeaderFunc func() bool,
	apiCl *apiserver.APIClient,
	rcClient workload.RcClient,
	wlm workloadmeta.Component,
	taggerComp tagger.Component,
	senderManager sender.SenderManager,
) (workload.PodPatcher, error) {
	if apiCl == nil {
		return nil, errors.New("Impossible to start workload autoscaling without valid APIClient")
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: apiCl.Cl.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "datadog-workload-autoscaler"})

	store := autoscaling.NewStore[model.PodAutoscalerInternal]()
	workload.InitDumper(store)

	podPatcher := workload.NewPodPatcher(store, isLeaderFunc, apiCl.DynamicCl, eventRecorder)
	podWatcher := workload.NewPodWatcher(wlm, podPatcher)

	clock := clock.RealClock{}
	_, err := workload.NewConfigRetriever(ctx, clock, store, isLeaderFunc, rcClient)
	if err != nil {
		return nil, fmt.Errorf("Unable to start workload autoscaling config retriever: %w", err)
	}

	// We could consider the sender to be optional, but it's actually required for backend information
	sender, err := senderManager.GetSender("workload_autoscaling")
	if err != nil {
		return nil, fmt.Errorf("Unable to start local telemetry for autoscaling: %w", err)
	}
	sender.DisableDefaultHostname(true)

	globalTagsFunc := func() []string {
		tags, err := taggerComp.GlobalTags(types.LowCardinality)
		if err != nil {
			log.Warnf("Unable to get global tags from tagger for workload autoscaling metrics: %v", err)
			return nil
		}
		return tags
	}

	maxDatadogPodAutoscalerObjects := pkgconfigsetup.Datadog().GetInt("autoscaling.workload.limit")
	limitHeap := autoscaling.NewHashHeap(maxDatadogPodAutoscalerObjects, store, (*model.PodAutoscalerInternal).CreationTimestamp)

	controller, err := workload.NewController(clock, clusterID, eventRecorder, apiCl.RESTMapper, apiCl.ScaleCl, apiCl.DynamicInformerCl, apiCl.DynamicInformerFactory, isLeaderFunc, store, podWatcher, sender, limitHeap, globalTagsFunc)
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
		localRecommender := local.NewRecommender(clock, podWatcher, store)
		go localRecommender.Run(ctx)
	}

	externalTLSConfig := buildExternalRecommenderTLSConfig(pkgconfigsetup.Datadog())
	externalRecommender, err := external.NewRecommender(ctx, clock, podWatcher, store, clusterName, externalTLSConfig)
	if err != nil {
		return nil, fmt.Errorf("Unable to start workload autoscaling external recommender: %w", err)
	}
	go externalRecommender.Run(ctx)

	return podPatcher, nil
}

func buildExternalRecommenderTLSConfig(cfg config.Component) *external.TLSFilesConfig {
	caFile := cfg.GetString("autoscaling.workload.external_recommender.tls.ca_file")
	certFile := cfg.GetString("autoscaling.workload.external_recommender.tls.cert_file")
	keyFile := cfg.GetString("autoscaling.workload.external_recommender.tls.key_file")

	if caFile == "" && certFile == "" && keyFile == "" {
		return nil
	}

	return &external.TLSFilesConfig{
		CAFile:   caFile,
		CertFile: certFile,
		KeyFile:  keyFile,
	}
}
