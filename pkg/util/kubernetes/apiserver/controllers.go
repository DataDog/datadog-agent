// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	dd_client "github.com/DataDog/datadog-operator/pkg/generated/clientset/versioned"
	dd_informers "github.com/DataDog/datadog-operator/pkg/generated/informers/externalversions"
	wpa_client "github.com/DataDog/watermarkpodautoscaler/pkg/client/clientset/versioned"
	wpa_informers "github.com/DataDog/watermarkpodautoscaler/pkg/client/informers/externalversions"

	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

type startFunc func(ControllerContext, chan error)

type controllerFuncs struct {
	enabled func() bool
	start   startFunc
}

var controllerCatalog = map[controllerName]controllerFuncs{
	metadataController: {
		func() bool { return config.Datadog.GetBool("kubernetes_collect_metadata_tags") },
		startMetadataController,
	},
	autoscalersController: {
		func() bool {
			return config.Datadog.GetBool("external_metrics_provider.enabled") && !config.Datadog.GetBool("external_metrics_provider.use_datadogmetric_crd")
		},
		startAutoscalersController,
	},
	servicesController: {
		func() bool { return config.Datadog.GetBool("cluster_checks.enabled") },
		startServicesInformer,
	},
	endpointsController: {
		func() bool { return config.Datadog.GetBool("cluster_checks.enabled") },
		startEndpointsInformer,
	},
}

type ControllerContext struct {
	InformerFactory    informers.SharedInformerFactory
	WPAClient          wpa_client.Interface
	WPAInformerFactory wpa_informers.SharedInformerFactory
	DDClient           dd_client.Interface
	DDInformerFactory  dd_informers.SharedInformerFactory
	Client             kubernetes.Interface
	IsLeaderFunc       func() bool
	EventRecorder      record.EventRecorder
	StopCh             chan struct{}
}

// StartControllers runs the enabled Kubernetes controllers for the Datadog Cluster Agent. This is
// only called once, when we have confirmed we could correctly connect to the API server.
func StartControllers(ctx ControllerContext) errors.Aggregate {
	var wg sync.WaitGroup
	errChan := make(chan error, len(controllerCatalog))
	for name, cntrlFuncs := range controllerCatalog {
		if !cntrlFuncs.enabled() {
			log.Infof("%q is disabled", name)
			continue
		}

		// controllers should be started in parallel as their start functions are
		// blocking until the informers are sync'ed or the sync period timed-out.
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

	// NewAggregate will filter out nil errors
	return errors.NewAggregate(errs)
}

// startMetadataController starts the informers needed for metadata collection.
// The synchronization of the informers is handled by the controller.
func startMetadataController(ctx ControllerContext, c chan error) {
	metaController := NewMetadataController(
		ctx.InformerFactory.Core().V1().Nodes(),
		ctx.InformerFactory.Core().V1().Endpoints(),
	)
	go metaController.Run(ctx.StopCh)
}

// startAutoscalersController starts the informers needed for autoscaling.
// The synchronization of the informers is handled by the controller.
func startAutoscalersController(ctx ControllerContext, c chan error) {
	dogCl, err := autoscalers.NewDatadogClient()
	if err != nil {
		c <- err
		return
	}
	autoscalersController, err := NewAutoscalersController(
		ctx.Client,
		ctx.EventRecorder,
		ctx.IsLeaderFunc,
		dogCl,
	)
	if err != nil {
		c <- err
		return
	}
	if ctx.WPAInformerFactory != nil {
		go autoscalersController.RunWPA(ctx.StopCh, ctx.WPAClient, ctx.WPAInformerFactory)
	}
	// mutate the Autoscaler controller to embed an informer against the HPAs
	autoscalersController.EnableHPA(ctx.InformerFactory.Autoscaling().V2beta1().HorizontalPodAutoscalers())
	go autoscalersController.RunHPA(ctx.StopCh)

	autoscalersController.RunControllerLoop(ctx.StopCh)
}

// startServicesInformer starts the service informer.
// The synchronization of the service informer is handled in this function.
func startServicesInformer(ctx ControllerContext, c chan error) {
	// Just start the shared informer, the autodiscovery
	// components will access it when needed.
	go ctx.InformerFactory.Core().V1().Services().Informer().Run(ctx.StopCh)

	// Wait for the cache to sync
	c <- SyncInformers(map[InformerName]cache.SharedInformer{
		servicesInformer: ctx.InformerFactory.Core().V1().Services().Informer(),
	})
}

// startEndpointsInformer starts the endpoints informer.
// The synchronization of the endpoints informer is handled in this function.
func startEndpointsInformer(ctx ControllerContext, c chan error) {
	// Just start the shared informer, the autodiscovery
	// components will access it when needed.
	go ctx.InformerFactory.Core().V1().Endpoints().Informer().Run(ctx.StopCh)

	// Wait for the cache to sync
	c <- SyncInformers(map[InformerName]cache.SharedInformer{
		endpointsInformer: ctx.InformerFactory.Core().V1().Endpoints().Informer(),
	})
}
