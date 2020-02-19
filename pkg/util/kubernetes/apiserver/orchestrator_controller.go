// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type OrchestratorController struct {
	unassignedPodLister     corelisters.PodLister
	unassignedPodListerSync cache.InformerSynced
}

func newOrchestratorController(podInformer coreinformers.PodInformer) *OrchestratorController {
	return &OrchestratorController{
		unassignedPodLister:     podInformer.Lister(),
		unassignedPodListerSync: podInformer.Informer().HasSynced,
	}
}

func (o *OrchestratorController) Run(stopCh <-chan struct{}) {
	log.Infof("Starting orchestrator controller")
	defer log.Infof("Stopping orchestrator controller")

	if !cache.WaitForCacheSync(stopCh, o.unassignedPodListerSync) {
		return
	}

	go wait.Until(o.processPods, 10*time.Second, stopCh)

	<-stopCh
}

func (o *OrchestratorController) processPods() {
	log.Info("processing pods...")
	pods, err := o.unassignedPodLister.List(labels.Everything())
	if err != nil {
		log.Errorf("Unable to list pods: %s", err)
	}
	log.Infof("Number of unassigned pods: %d", len(pods))
	for _, p := range pods {
		log.Infof("unassigned pod: %s %s %s", p.Name, p.Spec.NodeName, p.Status.Phase)
	}
}
