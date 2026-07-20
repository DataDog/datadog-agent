// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	corev1Informers "k8s.io/client-go/informers/core/v1"
	corev1Listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s/helm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
)

// helmChartCollectorName is the collector/resource name for Helm charts.
const helmChartCollectorName = "helmcharts"

// NewHelmChartCollectorVersions builds the group of collector versions.
func NewHelmChartCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewHelmChartCollector(),
	)
}

// HelmChartCollector collects charts packaged with Helm releases.
type HelmChartCollector struct {
	informer  corev1Informers.ConfigMapInformer
	lister    corev1Listers.ConfigMapLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewHelmChartCollector creates a new collector for Helm charts.
func NewHelmChartCollector() *HelmChartCollector {
	return &HelmChartCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsStable:                             true,
			IsManifestProducer:                   true,
			IsMetadataProducer:                   false,
			SupportsManifestBuffering:            false,
			Group:                                helm.HelmReleaseGroup,
			Name:                                 helmChartCollectorName,
			Kind:                                 helm.HelmChartKind,
			NodeType:                             orchestrator.K8sCR,
			Version:                              helm.HelmReleaseVersion,
			SupportsTerminatedResourceCollection: false,
		},
		processor: processors.NewProcessor(new(k8sProcessors.HelmReleaseHandlers)),
	}
}

// Informer returns the shared informer.
func (c *HelmChartCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *HelmChartCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.HelmConfigMapInformerFactory.Core().V1().ConfigMaps()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *HelmChartCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *HelmChartCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	configMaps, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	releases := helm.ReleasesFromConfigMaps(configMaps)
	charts := helm.AggregateCharts(releases)
	list := make([]runtime.Object, 0, len(charts))
	for _, chart := range charts {
		if u := helm.ChartToUnstructured(chart); u != nil {
			list = append(list, u)
		}
	}

	return c.Process(rcfg, list)
}

// Process is used to process the list of resources and return the result.
func (c *HelmChartCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
	ctx := collectors.NewK8sProcessorContext(rcfg, c.metadata)

	processResult, listed, processed := c.processor.Process(ctx, list)

	if processed == -1 {
		return nil, collectors.ErrProcessingPanic
	}

	result := &collectors.CollectorRunResult{
		Result:             processResult,
		ResourcesListed:    listed,
		ResourcesProcessed: processed,
	}

	return result, nil
}
