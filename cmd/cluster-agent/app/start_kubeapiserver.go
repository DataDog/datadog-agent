// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package app

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() {
	startControllersFunc = startControllersKubeAPIServer
}

func startControllersKubeAPIServer() error {
	apiCl, err := apiserver.GetAPIClient() // make sure we can connect to the apiserver
	if err != nil {
		log.Errorf("Could not connect to the apiserver: %v", err)
	} else {
		le, err := leaderelection.GetLeaderEngine()
		if err != nil {
			return err
		}

		// Create event recorder
		eventBroadcaster := record.NewBroadcaster()
		eventBroadcaster.StartLogging(log.Infof)
		eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: apiCl.Cl.CoreV1().Events("")})
		eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "datadog-cluster-agent"})

		stopCh := make(chan struct{})
		ctx := apiserver.ControllerContext{
			InformerFactory:    apiCl.InformerFactory,
			WPAClient:          apiCl.WPAClient,
			WPAInformerFactory: apiCl.WPAInformerFactory,
			Client:             apiCl.Cl,
			LeaderElector:      le,
			EventRecorder:      eventRecorder,
			StopCh:             stopCh,
		}

		if err := apiserver.StartControllers(ctx); err != nil {
			log.Errorf("Could not start controllers: %v", err)
		}
	}
	return nil
}
