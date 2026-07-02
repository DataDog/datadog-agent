// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const senderID = "cluster_spot_autoscaling"

// StartSpotScheduling creates and starts the spot scheduler, returning a PodHandler for use in the admission webhook.
func StartSpotScheduling(ctx context.Context, clusterID string, wlm workloadmeta.Component, apiCl *apiserver.APIClient, isLeaderFunc func() bool, senderManager sender.SenderManager, taggerComp tagger.Component) (PodHandler, error) {
	if apiCl == nil {
		return nil, errors.New("impossible to start spot scheduling without valid APIClient")
	}

	localSender, err := senderManager.GetSender(senderID)
	if err != nil {
		return nil, fmt.Errorf("unable to get sender for spot scheduling: %w", err)
	}
	localSender.DisableDefaultHostname(true)
	autoscaling.StartLocalTelemetry(ctx, localSender, "cluster.spot", []string{"orch_cluster_id:" + clusterID})

	globalTagsFunc := func() []string {
		tags, err := taggerComp.GlobalTags(types.LowCardinality)
		if err != nil {
			log.Warnf("Unable to get global tags from tagger for spot scheduling metrics: %v", err)
			return nil
		}
		return tags
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: apiCl.Cl.CoreV1().Events("")})
	eventRecorder := newSpotEventRecorder(eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: cluster.EventSourceComponent}))

	cfg := ReadConfig(pkgconfigsetup.Datadog())
	tel := newTelemetry(localSender, isLeaderFunc, globalTagsFunc)
	s := newScheduler(cfg, wlm,
		newKubePodEvictor(apiCl.Cl),
		newKubeWorkloadPatcher(apiCl.DynamicInformerCl),
		apiCl.DynamicInformerCl,
		newWLMPodLister(wlm),
		isLeaderFunc,
		tel,
		eventRecorder)
	s.Start(ctx)

	return s, nil
}
