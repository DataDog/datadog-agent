// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver,orchestrator

package orchestrator

import (
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	processcfg "github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
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
	Hostname                     string
	ClusterName                  string
}

type OrchestratorController struct {
	unassignedPodLister     corelisters.PodLister
	unassignedPodListerSync cache.InformerSynced
	groupID                 int32
	hostName                string
	clusterName             string
	apiClient               api.Client
}

func StartOrchestratorController(ctx OrchestratorControllerContext) error {
	if !config.Datadog.GetBool("orchestrator_explorer.enabled") {
		log.Info("orchestrator explorer is disabled")
		return nil
	}
	if ctx.ClusterName == "" {
		log.Info("orchestrator explorer enabled but no cluster name set: disabling")
		return nil
	}
	orchestratorControler := newOrchestratorController(
		ctx.UnassignedPodInformerFactory.Core().V1().Pods(),
		ctx.Hostname,
		ctx.ClusterName,
	)

	go orchestratorControler.Run(ctx.StopCh)

	ctx.UnassignedPodInformerFactory.Start(ctx.StopCh)

	return nil
}

func newOrchestratorController(podInformer coreinformers.PodInformer, hostName string, clusterName string) *OrchestratorController {
	return &OrchestratorController{
		unassignedPodLister:     podInformer.Lister(),
		unassignedPodListerSync: podInformer.Informer().HasSynced,
		groupID:                 rand.Int31(),
		hostName:                hostName,
		clusterName:             clusterName,
		apiClient: api.NewClient(
			http.Client{Timeout: 20 * time.Second, Transport: processcfg.NewDefaultTransport()},
			30*time.Second),
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
	msg, err := orchestrator.ProcessPodlist(podList, atomic.AddInt32(&o.groupID, 1), cfg, "hostname", "clustername")
	if err != nil {
		log.Errorf("Unable to process pod list")
		return
	}

	extraHeaders := map[string]string{
		"X-Dd-Hostname":       o.hostName,
		"X-Dd-ContainerCount": "0",
	}
	for _, m := range msg {
		log.Infof("message %v", m)
		statuses := o.apiClient.PostMessage(cfg.OrchestratorEndpoints, "/api/v1/orchestrator", m, extraHeaders)
		if len(statuses) > 0 {
			log.Infof("%v", statuses)
		}
	}
}
