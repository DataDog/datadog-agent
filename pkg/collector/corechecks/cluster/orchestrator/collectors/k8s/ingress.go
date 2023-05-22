// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	netv1Informers "k8s.io/client-go/informers/networking/v1"
	netv1Listers "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
)

// NewIngressCollectorVersions builds the group of collector versions.
func NewIngressCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewIngressCollector(),
	)
}

// IngressCollector is a collector for Kubernetes Ingresss.
type IngressCollector struct {
	informer    netv1Informers.IngressInformer
	lister      netv1Listers.IngressLister
	metadata    *collectors.CollectorMetadata
	processor   *processors.Processor
	retryLister func(ctx context.Context, opts metav1.ListOptions) (*netv1.IngressList, error)
}

// NewIngressCollector creates a new collector for the Kubernetes Ingress
// resource.
func NewIngressCollector() *IngressCollector {
	return &IngressCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:          true,
			IsStable:                  true,
			IsMetadataProducer:        true,
			IsManifestProducer:        true,
			SupportsManifestBuffering: true,
			Name:                      "ingresses",
			NodeType:                  orchestrator.K8sIngress,
			Version:                   "networking.k8s.io/v1",
		},
		processor: processors.NewProcessor(new(k8sProcessors.IngressHandlers)),
	}
}

// Informer returns the shared informer.
func (c *IngressCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *IngressCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.APIClient.InformerFactory.Networking().V1().Ingresses()
	c.lister = c.informer.Lister()
	c.retryLister = rcfg.APIClient.Cl.NetworkingV1().Ingresses("").List
}

// IsAvailable returns whether the collector is available.
// Returns false if the networking.k8s.io/v1 API version is not available (kubernetes < 1.19).
func (c *IngressCollector) IsAvailable() bool {
	var retrier retry.Retrier
	if err := retrier.SetupRetrier(&retry.Config{
		Name:          "NetworkV1Discovery",
		AttemptMethod: c.list,
		Strategy:      retry.RetryCount,
		RetryCount:    3,               // try 3 times
		RetryDelay:    1 * time.Second, // with 1 sec interval
	}); err != nil {
		log.Errorf("Couldn't setup api retrier: %v", err)
		return false
	}

	return try(&retrier) == nil
}

// Metadata is used to access information about the collector.
func (c *IngressCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *IngressCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	ctx := collectors.NewProcessorContext(rcfg, c.metadata)

	processResult, processed := c.processor.Process(ctx, list)

	if processed == -1 {
		return nil, collectors.ErrProcessingPanic
	}

	result := &collectors.CollectorRunResult{
		Result:             processResult,
		ResourcesListed:    len(list),
		ResourcesProcessed: processed,
	}

	return result, nil
}

func (c *IngressCollector) list() error {
	_, err := c.retryLister(context.TODO(), metav1.ListOptions{})
	return err
}

func try(r *retry.Retrier) error {
	for {
		_ = r.TriggerRetry()
		switch r.RetryStatus() {
		case retry.OK:
			log.Debug("Queried networking.k8s.io/v1 successfully")
			return nil
		case retry.PermaFail:
			err := r.LastError()
			log.Infof("Couldn't query networking.k8s.io/v1 successfully: %s", err.Error())
			return err
		}
	}
}
