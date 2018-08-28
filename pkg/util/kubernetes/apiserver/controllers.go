// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hpa"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

var controllerCatalog = map[string]func(controllerContext) error{
	"metadata":    startMetadataController,
	"autoscalers": startMetadataController,
}

type controllerContext struct {
	InformerFactory informers.SharedInformerFactory
	Client          kubernetes.Interface
	LeaderElector   LeaderElectorInterface
	StopCh          chan struct{}
}

// StartControllers runs the Kubernetes controllers for the Datadog Cluster Agent. This is
// only called once, when we have confirmed we could correctly connect to the API server.
func StartControllers(le LeaderElectorInterface, stopCh chan struct{}) error {
	timeoutSeconds := time.Duration(config.Datadog.GetInt64("kubernetes_informers_restclient_timeout"))
	resyncPeriodSeconds := time.Duration(config.Datadog.GetInt64("kubernetes_informers_resync_period"))
	client, err := getKubeClient(timeoutSeconds * time.Second)
	if err != nil {
		log.Infof("Could not get apiserver client: %v", err)
		return err
	}

	informerFactory := informers.NewSharedInformerFactory(client, resyncPeriodSeconds*time.Second)
	informerFactory.Start(stopCh)

	ctx := controllerContext{
		InformerFactory: informerFactory,
		Client:          client,
		LeaderElector:   le,
		StopCh:          stopCh,
	}

	enabledControllers := sets.NewString("metadata") // always enabled
	if config.Datadog.GetBool("external_metrics_provider.enabled") {
		enabledControllers.Insert("autoscalers")
	}

	for name, controllerFn := range controllerCatalog {
		if !enabledControllers.Has(name) {
			log.Infof("%q is disabled", name)
			continue
		}
		err := controllerFn(ctx)
		if err != nil {
			log.Errorf("Error starting %q", name)
		}
	}

	return nil
}

func startMetadataController(ctx controllerContext) error {
	metaController := NewMetadataController(
		ctx.InformerFactory.Core().V1().Nodes(),
		ctx.InformerFactory.Core().V1().Endpoints(),
	)
	go metaController.Run(ctx.StopCh)

	return nil
}

func startAutoscalersController(ctx controllerContext) error {
	dogCl, err := hpa.NewDatadogClient()
	if err != nil {
		return err
	}
	autoscalersController, err := NewAutoscalersController(
		ctx.Client,
		ctx.LeaderElector,
		dogCl,
		ctx.InformerFactory.Autoscaling().V2beta1().HorizontalPodAutoscalers(),
	)
	if err != nil {
		return err
	}
	go autoscalersController.Run(ctx.StopCh)

	return nil
}
