// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

// Package k8s defines handlers for processing kubernetes resources
package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	jsoniter "github.com/json-iterator/go"
	"github.com/twmb/murmur3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1Client "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	extendedResourcesBlacklist = map[corev1.ResourceName]struct{}{
		corev1.ResourceCPU:    {},
		corev1.ResourceMemory: {},
		corev1.ResourcePods:   {},
	}
)

// ClusterProcessor is a processor for Kubernetes clusters. There is no
// concept of cluster per se in Kubernetes. The model is created by aggregating
// data from the Kubernetes Node resource and pulling API Server information.
// This is why that processor is custom and not following the generic logic like
// other resources.
type ClusterProcessor struct {
	processors.Processor
	nodeHandlers processors.Handlers
}

// NewClusterProcessor creates a new processor for the Kubernetes cluster
// resource.
func NewClusterProcessor() *ClusterProcessor {
	return &ClusterProcessor{
		nodeHandlers: new(NodeHandlers),
	}
}

// Process is used to process a list of node resources forming a cluster.
func (p *ClusterProcessor) Process(ctx processors.ProcessorContext, list interface{}) (processResult processors.ProcessResult, processed int, err error) {
	processed = -1

	defer processors.RecoverOnPanic()

	// Cluster information is an aggregation of node list data.
	var (
		kubeletVersions              = make(map[string]int32)
		cpuAllocatable               uint64
		cpuCapacity                  uint64
		memoryAllocatable            uint64
		memoryCapacity               uint64
		podAllocatable               uint32
		podCapacity                  uint32
		extendedResourcesCapacity    = make(map[string]int64)
		extendedResourcesAllocatable = make(map[string]int64)
	)
	pctx := ctx.(*processors.K8sProcessorContext)
	resourceList := p.nodeHandlers.ResourceList(ctx, list)
	nodeCount := int32(len(resourceList))
	nodesInfo := make([]*model.ClusterNodeInfo, 0, len(resourceList))

	for _, resource := range resourceList {
		r := resource.(*corev1.Node)

		// Kubelet versions.
		kubeletVersions[r.Status.NodeInfo.KubeletVersion]++

		// CPU allocatable and capacity.
		cpuAllocatable += uint64(r.Status.Allocatable.Cpu().MilliValue())
		cpuCapacity += uint64(r.Status.Capacity.Cpu().MilliValue())

		// Memory allocatable and capacity.
		memoryAllocatable += uint64(r.Status.Allocatable.Memory().Value())
		memoryCapacity += uint64(r.Status.Capacity.Memory().Value())

		// Pod allocatable and capacity.
		podAllocatable += uint32(r.Status.Allocatable.Pods().Value())
		podCapacity += uint32(r.Status.Capacity.Pods().Value())

		// Unaggregated node information summary.
		nodesInfo = append(nodesInfo, k8sTransformers.ExtractClusterNodeInfo(r))

		// Extended resources capacity and allocatable.
		for name, quantity := range r.Status.Capacity {
			if _, found := extendedResourcesBlacklist[name]; !found {
				extendedResourcesCapacity[name.String()] += quantity.Value()
			}
		}
		for name, quantity := range r.Status.Allocatable {
			if _, found := extendedResourcesBlacklist[name]; !found {
				extendedResourcesAllocatable[name.String()] += quantity.Value()
			}
		}
	}

	clusterModel := &model.Cluster{
		CpuAllocatable:               cpuAllocatable,
		CpuCapacity:                  cpuCapacity,
		KubeletVersions:              kubeletVersions,
		MemoryAllocatable:            memoryAllocatable,
		MemoryCapacity:               memoryCapacity,
		NodeCount:                    nodeCount,
		PodAllocatable:               podAllocatable,
		PodCapacity:                  podCapacity,
		ExtendedResourcesCapacity:    extendedResourcesCapacity,
		ExtendedResourcesAllocatable: extendedResourcesAllocatable,
		NodesInfo:                    nodesInfo,
	}

	kubeSystemCreationTimestamp, err := getKubeSystemCreationTimeStamp(pctx.APIClient.Cl.CoreV1())
	if err != nil {
		return processResult, 0, fmt.Errorf("error getting server kube system creation timestamp: %s", err.Error())
	}

	apiVersion, err := pctx.APIClient.Cl.Discovery().ServerVersion()
	if err != nil {
		return processResult, 0, fmt.Errorf("error getting server apiVersion: %s", err.Error())
	}

	clusterModel.ApiServerVersions = map[string]int32{apiVersion.String(): 1}

	if !kubeSystemCreationTimestamp.IsZero() {
		clusterModel.CreationTimestamp = kubeSystemCreationTimestamp.Unix()
	}

	if err := fillClusterResourceVersion(clusterModel); err != nil {
		return processResult, 0, fmt.Errorf("failed to compute resource version: %s", err.Error())
	}

	if orchestrator.SkipKubernetesResource(types.UID(pctx.ClusterID), clusterModel.ResourceVersion, orchestrator.K8sCluster) {
		stats := orchestrator.CheckStats{
			CacheHits: 1,
			CacheMiss: 0,
			NodeType:  orchestrator.K8sCluster,
		}
		orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sCluster), stats, orchestrator.NoExpiration)
		return processResult, 0, nil
	}

	yaml, err := json.Marshal(clusterModel)
	if err != nil {
		log.Warnc(processors.NewMarshallingError(err).Error(), orchestrator.ExtraLogContext...)
		return processResult, 0, nil
	}

	metadataMessages := []model.MessageBody{
		&model.CollectorCluster{
			ClusterName:  pctx.Cfg.KubeClusterName,
			ClusterId:    pctx.ClusterID,
			GroupId:      pctx.MsgGroupID,
			Cluster:      clusterModel,
			Tags:         util.ImmutableTagsJoin(pctx.Cfg.ExtraTags, pctx.GetCollectorTags()),
			AgentVersion: ctx.GetAgentVersion(),
		},
	}
	manifestMessages := []model.MessageBody{
		&model.CollectorManifest{
			ClusterName: pctx.Cfg.KubeClusterName,
			ClusterId:   pctx.ClusterID,
			GroupId:     pctx.MsgGroupID,
			HostName:    pctx.HostName,
			Manifests: []*model.Manifest{
				{
					Content:         yaml,
					ContentType:     "json",
					ResourceVersion: clusterModel.ResourceVersion,
					Type:            int32(orchestrator.K8sCluster),
					Uid:             pctx.ClusterID,
					Version:         "v1",
					// when manifest get buffered, they share a common CollectorManifest - collector-specific tags
					// should be added to the Manifest only
					Tags: pctx.GetCollectorTags(),
				},
			},
			Tags:         pctx.Cfg.ExtraTags,
			AgentVersion: ctx.GetAgentVersion(),
		},
	}
	processResult = processors.ProcessResult{
		MetadataMessages: metadataMessages,
		ManifestMessages: manifestMessages,
	}

	return processResult, 1, nil
}

func fillClusterResourceVersion(c *model.Cluster) error {
	marshaller := jsoniter.ConfigCompatibleWithStandardLibrary
	jsonClustermodel, err := marshaller.Marshal(c)
	if err != nil {
		return fmt.Errorf("could not marshal model to JSON: %s", err)
	}

	version := murmur3.Sum64(jsonClustermodel)
	c.ResourceVersion = fmt.Sprint(version)

	return nil
}

// getKubeSystemCreationTimeStamp returns the timestamp of the kube-system namespace from the cluster
// We use it as the cluster timestamp as it is the first namespace which have been created by the cluster.
func getKubeSystemCreationTimeStamp(coreClient corev1Client.CoreV1Interface) (metav1.Time, error) {
	x, found := orchestrator.KubernetesResourceCache.Get(orchestrator.ClusterAgeCacheKey)
	if found {
		return x.(metav1.Time), nil
	}
	svc, err := coreClient.Namespaces().Get(context.TODO(), "kube-system", metav1.GetOptions{})
	if err != nil {
		return metav1.Time{}, err
	}
	ts := svc.GetCreationTimestamp()
	orchestrator.KubernetesResourceCache.Set(orchestrator.ClusterAgeCacheKey, svc.GetCreationTimestamp(), orchestrator.NoExpiration)
	return ts, nil
}
