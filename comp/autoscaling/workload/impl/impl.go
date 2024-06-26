// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package workloadimpl

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	autoscalingComp "github.com/DataDog/datadog-agent/comp/autoscaling/workload/def"
	"github.com/DataDog/datadog-agent/comp/autoscaling/workload/impl/model"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Requires defines the dependencies for the autoscaling component
type Requires struct {
	Lc  compdef.Lifecycle
	Log log.Component
	Cfg config.Component
	Wlm workloadmeta.Component

	// TODO(Components): Componentize the API client and the RC Client for the Cluster-Agent
	APICl    *apiserver.APIClient
	RcClient optional.Option[*rcclient.Client]
}

// Provides defines the output of the autoscaling component
type Provides struct {
	Comp optional.Option[autoscalingComp.Component]
}

// NewComponent creates a new autoscaling component
func NewComponent(reqs Requires) (Provides, error) {
	provides := Provides{
		Comp: optional.NewNoneOption[autoscalingComp.Component](),
	}
	if !reqs.Cfg.GetBool("autoscaling.workload.enabled") {
		return provides, nil
	}

	rcClient, ok := reqs.RcClient.Get()
	if !ok {
		return provides, fmt.Errorf("remote configuration client not found")
	}

	if !reqs.Cfg.GetBool("admission_controller.enabled") {
		reqs.Log.Error("Admission controller is disabled, vertical autoscaling requires the admission controller to be enabled. Vertical scaling will be disabled.")
	}

	le, err := leaderelection.GetLeaderEngine()
	if err != nil {
		return provides, fmt.Errorf("Unable to start workload autoscaling as LeaderElection failed with: %v", err)
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: reqs.APICl.Cl.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "datadog-workload-autoscaler"})

	store := autoscaling.NewStore[model.PodAutoscalerInternal]()
	podPatcher := newPodPatcher(store, le.IsLeader, reqs.APICl.DynamicCl, eventRecorder)
	podWatcher := newPodWatcher(reqs.Wlm, podPatcher)

	_, err = newConfigRetriever(store, le.IsLeader, rcClient)
	if err != nil {
		return provides, fmt.Errorf("Unable to start workload autoscaling config retriever: %w", err)
	}

	controller, err := newController(eventRecorder, reqs.APICl.RESTMapper, reqs.APICl.ScaleCl, reqs.APICl.DynamicInformerCl, reqs.APICl.DynamicInformerFactory, le.IsLeader, store, podWatcher)
	if err != nil {
		return provides, fmt.Errorf("Unable to start workload autoscaling controller: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	reqs.Lc.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			// Start informers & controllers (informers can be started multiple times)
			reqs.APICl.DynamicInformerFactory.Start(ctx.Done())
			reqs.APICl.InformerFactory.Start(ctx.Done())

			// TODO: Wait POD Watcher sync before running the controller
			go podWatcher.Run(ctx)
			go controller.Run(ctx)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			cancel()
			return nil
		},
	})
	provides.Comp = optional.NewOption[autoscalingComp.Component](podPatcher)
	return provides, nil
}
