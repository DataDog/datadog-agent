// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build kubelet && orchestrator

// Package pod is used for the orchestrator pod check
package pod

import (
	"context"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	maximumWaitForAPIServer = 10 * time.Second
	defaultResyncInterval   = 300 * time.Second
	defaultExtraSyncTimeout = 60 * time.Second
)

var startTerminatedPodsCollectionOnce sync.Once

// TerminatedPodCollector collects terminated pods manifest and metadata
type TerminatedPodCollector struct {
	hostName   string
	clusterID  string
	sender     sender.Sender
	processor  *processors.Processor
	config     *oconfig.OrchestratorConfig
	systemInfo *model.SystemInfo
	stopChan   chan struct{}
}

// NewTerminatedPodCollector creates a new TerminatedPodCollector
func NewTerminatedPodCollector(hostName, clusterID string, sender sender.Sender, processor *processors.Processor, config *oconfig.OrchestratorConfig, systemInfo *model.SystemInfo) *TerminatedPodCollector {
	return &TerminatedPodCollector{
		hostName:   hostName,
		clusterID:  clusterID,
		sender:     sender,
		processor:  processor,
		config:     config,
		systemInfo: systemInfo,
		stopChan:   make(chan struct{}),
	}
}

// Run starts the terminated pod collection
// It will only start once and will not start if the feature is disabled
func (t *TerminatedPodCollector) Run() {
	if !pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.terminated_resources.enabled") {
		return
	}

	startTerminatedPodsCollectionOnce.Do(func() {
		log.Infof("Starting terminated pods collection")
		if err := t.setupInformer(); err != nil {
			log.Errorf("failed to setup pod informer: %s", err)
			return
		}
	})
}

// Stop stops the terminated pod collection
func (t *TerminatedPodCollector) Stop() {
	log.Infof("Terminated pods collection stopped")
	close(t.stopChan)
}

// setupInformer sets up the pod informer and starts the collection
// It watches for pod deletions and processes them
func (t *TerminatedPodCollector) setupInformer() error {
	sharedInformerFactory, err := getSharedInformerFactory()
	if err != nil {
		return err
	}

	podInformer := sharedInformerFactory.Core().V1().Pods()

	if _, err = podInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			DeleteFunc: t.deletionHandler,
		}); err != nil {
		return err
	}

	go podInformer.Informer().Run(t.stopChan)

	return apiserver.SyncInformers(
		map[apiserver.InformerName]cache.SharedInformer{
			"terminated-pods": podInformer.Informer(),
		},
		getExtraSyncTimeout(),
	)
}

// deletionHandler processes the pod deletion event and sends the metadata and manifest
func (t *TerminatedPodCollector) deletionHandler(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		log.Warn("deletionHandler received an object that is not a Pod")
		return
	}

	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              t.config,
			NodeType:         orchestrator.K8sPod,
			ClusterID:        t.clusterID,
			ManifestProducer: true,
		},
		HostName:           t.hostName,
		ApiGroupVersionTag: "kube_api_version:v1",
		SystemInfo:         t.systemInfo,
	}

	processResult, processed := t.processor.Process(ctx, []*v1.Pod{pod})
	if processed == -1 {
		log.Warn("unable to process pods: a panic occurred")
		return
	}

	t.sender.OrchestratorMetadata(processResult.MetadataMessages, t.clusterID, orchestrator.K8sPod)
	t.sender.OrchestratorManifest(processResult.ManifestMessages, t.clusterID)
}

// getSharedInformerFactory returns a shared informer factory for pods
func getSharedInformerFactory() (informers.SharedInformerFactory, error) {
	kubeUtil, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}

	nodeName, err := kubeUtil.GetNodename(context.Background())
	if err != nil {
		return nil, err
	}

	apiCtx, apiCancel := context.WithTimeout(context.Background(), maximumWaitForAPIServer)
	defer apiCancel()

	apiClient, err := apiserver.WaitForAPIClient(apiCtx)
	if err != nil {
		return nil, err
	}

	tweakListOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", nodeName).String()
	}
	log.Infof("Creating pod informer for node %s", nodeName)

	return apiClient.GetInformerWithOptions(pointer.Ptr(defaultResyncInterval), informers.WithTweakListOptions(tweakListOptions)), nil
}

// getExtraSyncTimeout returns the extra sync timeout for the informer
func getExtraSyncTimeout() time.Duration {
	extraTimeout := pkgconfigsetup.Datadog().GetDuration("orchestrator_explorer.terminated_resources.extra_sync_timeout")
	if extraTimeout > 0 {
		return extraTimeout
	}
	return defaultExtraSyncTimeout
}
