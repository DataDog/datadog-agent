// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package collectors

import (
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/processors"
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
	proc     *processors.Processor
}

func newK8sServiceCollector() *K8sServiceCollector {
	return &K8sServiceCollector{
		meta: &CollectorMetadata{
			IsStable: true,
			Name:     "services",
			NodeType: orchestrator.K8sService,
		},
		proc: processors.NewProcessor(new(processors.K8sServiceHandlers)),
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
func (sc *K8sServiceCollector) Run(rcfg *CollectorRunConfig) (res *CollectorRunResult, err error) {
	list, err := sc.lister.List(labels.Everything())
	if err != nil {
		return nil, newListingError(err)
	}

	ctx := &processors.ProcessorContext{
		Cfg:        rcfg.Config,
		ClusterID:  rcfg.ClusterID,
		MsgGroupID: atomic.AddInt32(rcfg.MsgGroupRef, 1),
		NodeType:   sc.meta.NodeType,
	}

	messages, processed := sc.proc.Process(ctx, list)

	// This would happen when recovering from a processor panic. In the nominal
	// case we would a positive integer set at the very end of processing.  If
	// this is not the case then it means code execution stopped sooner. Panic
	// recovery will log more information about the error so we can figure the
	// root cause.
	if processed == -1 {
		return nil, processingPanicErr
	}

	result := &CollectorRunResult{
		Messages:           messages,
		ResourcesListed:    len(list),
		ResourcesProcessed: processed,
	}

	return result, nil
}
