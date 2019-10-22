// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/watermarkpodautoscaler/pkg/client/informers/externalversions"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

type controllerFuncs struct {
	enabled func() bool
	start   func(ControllerContext) error
}

var controllerCatalog = map[string]controllerFuncs{
	"metadata": {
		func() bool { return config.Datadog.GetBool("kubernetes_collect_metadata_tags") },
		startMetadataController,
	},
	"autoscalers": {
		func() bool { return config.Datadog.GetBool("external_metrics_provider.enabled") },
		startAutoscalersController,
	},
	"services": {
		func() bool { return config.Datadog.GetBool("cluster_checks.enabled") },
		startServicesInformer,
	},
}

type ControllerContext struct {
	InformerFactory    informers.SharedInformerFactory
	WPAInformerFactory externalversions.SharedInformerFactory
	Client             kubernetes.Interface
	LeaderElector      LeaderElectorInterface
	StopCh             chan struct{}
}

// StartControllers runs the enabled Kubernetes controllers for the Datadog Cluster Agent. This is
// only called once, when we have confirmed we could correctly connect to the API server.
func StartControllers(ctx ControllerContext) error {
	for name, cntrlFuncs := range controllerCatalog {
		if !cntrlFuncs.enabled() {
			log.Infof("%q is disabled", name)
			continue
		}
		err := cntrlFuncs.start(ctx)
		if err != nil {
			log.Errorf("Error starting %q: %s", name, err.Error())
		}
	}

	// we must start the informer factory after starting the controllers because the informer
	// factory uses lazy initialization (delays the creation of an informer until the first
	// time it's needed).
	ctx.InformerFactory.Start(ctx.StopCh)
	ctx.WPAInformerFactory.Start(ctx.StopCh)

	return nil
}

func startMetadataController(ctx ControllerContext) error {
	metaController := NewMetadataController(
		ctx.InformerFactory.Core().V1().Nodes(),
		ctx.InformerFactory.Core().V1().Endpoints(),
	)
	go metaController.Run(ctx.StopCh)

	return nil
}

func startAutoscalersController(ctx ControllerContext) error {
	dogCl, err := autoscalers.NewDatadogClient()
	if err != nil {
		return err
	}
	autoscalersController, err := NewAutoscalersController(
		ctx.Client,
		ctx.LeaderElector,
		dogCl,
	)
	if err != nil {
		return err
	}
	if config.Datadog.GetBool("external_metrics_provider.wpa_controller") {
		ctx.WPAInformerFactory, err = getWPAInformerFactory()
		if err != nil {
			log.Errorf("Error getting WPA Informer Factory: %s", err.Error())
			return err
		}
		autoscalersController.wpaEnabled = true
		// mutate the Autoscaler controller to embed an informer against the WPAs
		ExtendToWPAController(autoscalersController, ctx.WPAInformerFactory.Datadoghq().V1alpha1().WatermarkPodAutoscalers())
		if err != nil {
			return err
		}
		go autoscalersController.RunWPA(ctx.StopCh)
	}
	// mutate the Autoscaler controller to embed an informer against the HPAs
	ExtendToHPAController(autoscalersController, ctx.InformerFactory.Autoscaling().V2beta1().HorizontalPodAutoscalers())
	go autoscalersController.RunHPA(ctx.StopCh)

	autoscalersController.RunControllerLoop(ctx.StopCh)
	return nil
}

func startServicesInformer(ctx ControllerContext) error {
	// Just start the shared informer, the autodiscovery
	// components will access it when needed.
	go ctx.InformerFactory.Core().V1().Services().Informer().Run(ctx.StopCh)

	return nil
}
