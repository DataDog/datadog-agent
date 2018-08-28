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

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

type controllerFuncs struct {
	enabled func() bool
	start   func(controllerContext) error
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
}

type controllerContext struct {
	informerFactory informers.SharedInformerFactory
	client          kubernetes.Interface
	leaderElector   LeaderElectorInterface
	stopCh          chan struct{}
}

// StartControllers runs the enabled Kubernetes controllers for the Datadog Cluster Agent. This is
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
		informerFactory: informerFactory,
		client:          client,
		leaderElector:   le,
		stopCh:          stopCh,
	}

	for name, cntrlFuncs := range controllerCatalog {
		if !cntrlFuncs.enabled() {
			log.Infof("%q is disabled", name)
			continue
		}
		err := cntrlFuncs.start(ctx)
		if err != nil {
			log.Errorf("Error starting %q", name)
		}
	}

	return nil
}

func startMetadataController(ctx controllerContext) error {
	metaController := NewMetadataController(
		ctx.informerFactory.Core().V1().Nodes(),
		ctx.informerFactory.Core().V1().Endpoints(),
	)
	go metaController.Run(ctx.stopCh)

	return nil
}

func startAutoscalersController(ctx controllerContext) error {
	dogCl, err := hpa.NewDatadogClient()
	if err != nil {
		return err
	}
	autoscalersController, err := NewAutoscalersController(
		ctx.client,
		ctx.leaderElector,
		dogCl,
		ctx.informerFactory.Autoscaling().V2beta1().HorizontalPodAutoscalers(),
	)
	if err != nil {
		return err
	}
	go autoscalersController.Run(ctx.stopCh)

	return nil
}
