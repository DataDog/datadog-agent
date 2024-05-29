// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package controllers is responsible for running the Kubernetes controllers
// needed by the Datadog Cluster Agent
package controllers

import (
	"errors"
	"sync"

	k8serrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const autoscalerNowHandleMsgEvent = "Autoscaler is now handled by the Cluster-Agent"

var errIsEmpty = errors.New("entity is empty") //nolint:revive

type startFunc func(ControllerContext, chan error)

type controllerFuncs struct {
	enabled func() bool
	start   startFunc
}

var controllerCatalog = map[controllerName]controllerFuncs{
	metadataControllerName: {
		func() bool { return config.Datadog().GetBool("kubernetes_collect_metadata_tags") },
		startMetadataController,
	},
	autoscalersControllerName: {
		func() bool {
			return config.Datadog().GetBool("external_metrics_provider.enabled") && !config.Datadog().GetBool("external_metrics_provider.use_datadogmetric_crd")
		},
		startAutoscalersController,
	},
	servicesControllerName: {
		func() bool { return config.Datadog().GetBool("cluster_checks.enabled") },
		registerServicesInformer,
	},
	endpointsControllerName: {
		func() bool { return config.Datadog().GetBool("cluster_checks.enabled") },
		registerEndpointsInformer,
	},
}

// ControllerContext holds all the attributes needed by the controllers
type ControllerContext struct {
	informers              map[apiserver.InformerName]cache.SharedInformer
	InformerFactory        informers.SharedInformerFactory
	DynamicClient          dynamic.Interface
	DynamicInformerFactory dynamicinformer.DynamicSharedInformerFactory
	Client                 kubernetes.Interface
	IsLeaderFunc           func() bool
	EventRecorder          record.EventRecorder
	WorkloadMeta           workloadmeta.Component
	StopCh                 chan struct{}
}

// StartControllers runs the enabled Kubernetes controllers for the Datadog Cluster Agent. This is
// only called once, when we have confirmed we could correctly connect to the API server.
func StartControllers(ctx ControllerContext) k8serrors.Aggregate {
	ctx.informers = make(map[apiserver.InformerName]cache.SharedInformer)

	var wg sync.WaitGroup
	errChan := make(chan error, len(controllerCatalog))
	for name, cntrlFuncs := range controllerCatalog {
		if !cntrlFuncs.enabled() {
			log.Infof("%q is disabled", name)
			continue
		}

		// controllers should be started in parallel as their start functions are
		// blocking until the informers are synced or the sync period timed-out.
		// for error propagation we rely on a buffered channel to gather errors
		// from the spawned goroutines.
		wg.Add(1)
		go func(f startFunc) {
			defer wg.Done()
			f(ctx, errChan)
		}(cntrlFuncs.start)
	}

	wg.Wait()
	close(errChan)
	errs := []error{}
	for err := range errChan {
		errs = append(errs, err)
	}

	// we must start the informer factory after starting the controllers because the informer
	// factory uses lazy initialization (delays the creation of an informer until the first
	// time it's needed).
	// TODO: If any of the controllers here are initialized asynchronously, relying on the
	// informer factory to run informers for these controllers will not initialize them properly.
	// FIXME: We may want to initialize each of these controllers separately via their respective
	// `<informer>.Run()`
	ctx.InformerFactory.Start(ctx.StopCh)

	// Wait for the cache to sync
	if err := apiserver.SyncInformers(ctx.informers, 0); err != nil {
		errs = append(errs, err)
	}

	// NewAggregate will filter out nil errors
	return k8serrors.NewAggregate(errs)
}

// startMetadataController starts the informers needed for metadata collection.
// The synchronization of the informers is handled by the controller.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func startMetadataController(ctx ControllerContext, c chan error) {
	metaController := newMetadataController(
		ctx.InformerFactory.Core().V1().Endpoints(),
		ctx.WorkloadMeta,
	)
	go metaController.run(ctx.StopCh)
}

// startAutoscalersController starts the informers needed for autoscaling.
// The synchronization of the informers is handled by the controller.
func startAutoscalersController(ctx ControllerContext, c chan error) {
	var err error
	apiserver.DogCl, err = autoscalers.NewDatadogClient()
	if err != nil {
		c <- err
		return
	}
	autoscalersController, err := newAutoscalersController(
		ctx.Client,
		ctx.EventRecorder,
		ctx.IsLeaderFunc,
		apiserver.DogCl,
	)
	if err != nil {
		c <- err
		return
	}

	if config.Datadog().GetBool("external_metrics_provider.wpa_controller") {
		go autoscalersController.runWPA(ctx.StopCh, ctx.DynamicClient, ctx.DynamicInformerFactory)
	}

	autoscalersController.enableHPA(ctx.Client, ctx.InformerFactory)
	go autoscalersController.runHPA(ctx.StopCh)

	autoscalersController.runControllerLoop(ctx.StopCh)
}

// registerServicesInformer registers the services informer.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func registerServicesInformer(ctx ControllerContext, c chan error) {
	ctx.informers[servicesInformer] = ctx.InformerFactory.Core().V1().Services().Informer()
}

// registerEndpointsInformer registers the endpoints informer.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func registerEndpointsInformer(ctx ControllerContext, c chan error) {
	ctx.informers[endpointsInformer] = ctx.InformerFactory.Core().V1().Endpoints().Informer()
}
