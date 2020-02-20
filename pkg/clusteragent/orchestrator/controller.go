// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package orchestrator

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	processcfg "github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/util/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type OrchestratorControllerContext struct {
	UnassignedPodInformerFactory informers.SharedInformerFactory
	Client                       kubernetes.Interface
	StopCh                       chan struct{}
}

type OrchestratorController struct {
	unassignedPodLister     corelisters.PodLister
	unassignedPodListerSync cache.InformerSynced
}

func StartOrchestratorController(ctx OrchestratorControllerContext) error {
	if !config.Datadog.GetBool("orchestrator_explorer.enabled") {
		log.Info("orchestrator explorer is disabled")
		return nil
	}
	orchestratorControler := newOrchestratorController(
		ctx.UnassignedPodInformerFactory.Core().V1().Pods(),
	)

	go orchestratorControler.Run(ctx.StopCh)

	ctx.UnassignedPodInformerFactory.Start(ctx.StopCh)

	return nil
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
	podList, err := o.unassignedPodLister.List(labels.Everything())

	if err != nil {
		log.Errorf("Unable to list pods: %s", err)
		return
	}
	log.Infof("Number of unassigned pods: %d", len(podList))
	for _, p := range podList {
		log.Infof("unassigned pod: %s %s %s", p.Name, p.Spec.NodeName, p.Status.Phase)
	}

	cfg := processcfg.NewDefaultAgentConfig(true)
	// FIXME: generate proper groupid
	groupId := 1
	msg, err := orchestrator.ProcessPodlist(podList, groupId, cfg)
	if err != nil {
		log.Errorf("Unable to process pod list")
		return
	}

	for _, m := range msg {
		log.Infof("message %v", m)
	}
}
