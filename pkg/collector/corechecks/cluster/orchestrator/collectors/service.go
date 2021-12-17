// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package collectors

import (
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/orchestrator"

	"k8s.io/apimachinery/pkg/labels"
	corev1Informers "k8s.io/client-go/informers/core/v1"
	corev1Listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// K8sServiceCollector is a collector for Kubernetes Services.
type K8sServiceCollector struct {
	informer corev1Informers.ServiceInformer
	lister   corev1Listers.ServiceLister
	meta     *CollectorMetadata
}

func newK8sServiceCollector() *K8sServiceCollector {
	return &K8sServiceCollector{
		meta: &CollectorMetadata{
			IsStable: true,
			Name:     "services",
			NodeType: orchestrator.K8sService,
		},
	}
}

// Informer returns the shared informer.
func (sc *K8sServiceCollector) Informer() cache.SharedInformer {
	return sc.informer.Informer()
}

// Init is used to initialize the collector.
func (sc *K8sServiceCollector) Init(rcfg *CollectorRunConfig) {
	sc.informer = rcfg.APIClient.InformerFactory.Core().V1().Services()
	sc.lister = sc.informer.Lister()
}

// Metadata is used to access information about the collector.
func (sc *K8sServiceCollector) Metadata() *CollectorMetadata {
	return sc.meta
}

// Run triggers the collection process.
func (sc *K8sServiceCollector) Run(rcfg *CollectorRunConfig) (*CollectorRunResult, error) {
	list, err := sc.lister.List(labels.Everything())
	if err != nil {
		return nil, NewListingError(err)
	}

	groupID := atomic.AddInt32(rcfg.MsgGroupRef, 1)
	messages, processed := processServiceList(list, groupID, rcfg.Config, rcfg.ClusterID)
	if err != nil {
		return nil, NewProcessingError(err)
	}

	return &CollectorRunResult{Messages: messages, ResourcesListed: len(list), ResourcesProcessed: processed}, nil
}
