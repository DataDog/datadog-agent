// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"context"
	"fmt"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"

	jsoniter "github.com/json-iterator/go"
	"github.com/twmb/murmur3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1Client "k8s.io/client-go/kubernetes/typed/core/v1"
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
func (p *ClusterProcessor) Process(ctx *processors.ProcessorContext, list interface{}) (processResult processors.ProcessResult, processed int, err error) {
	processed = -1

	defer processors.RecoverOnPanic()

	// Cluster information is an aggregation of node list data.
	var (
		kubeletVersions   = make(map[string]int32)
		cpuAllocatable    uint64
		cpuCapacity       uint64
		memoryAllocatable uint64
		memoryCapacity    uint64
		podAllocatable    uint32
		podCapacity       uint32
	)

	resourceList := p.nodeHandlers.ResourceList(ctx, list)
	nodeCount := int32(len(resourceList))

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
	}

	clusterModel := &model.Cluster{
		CpuAllocatable:    cpuAllocatable,
		CpuCapacity:       cpuCapacity,
		KubeletVersions:   kubeletVersions,
		MemoryAllocatable: memoryAllocatable,
		MemoryCapacity:    memoryCapacity,
		NodeCount:         nodeCount,
		PodAllocatable:    podAllocatable,
		PodCapacity:       podCapacity,
	}

	kubeSystemCreationTimestamp, err := getKubeSystemCreationTimeStamp(ctx.APIClient.Cl.CoreV1())
	if err != nil {
		return processResult, 0, fmt.Errorf("error getting server kube system creation timestamp: %s", err.Error())
	}

	apiVersion, err := ctx.APIClient.Cl.Discovery().ServerVersion()
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

	if orchestrator.SkipKubernetesResource(types.UID(ctx.ClusterID), clusterModel.ResourceVersion, orchestrator.K8sCluster) {
		stats := orchestrator.CheckStats{
			CacheHits: 1,
			CacheMiss: 0,
			NodeType:  orchestrator.K8sCluster,
		}
		orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sCluster), stats, orchestrator.NoExpiration)
		return processResult, 0, nil
	}

	messages := []model.MessageBody{
		&model.CollectorCluster{
			ClusterName: ctx.Cfg.KubeClusterName,
			ClusterId:   ctx.ClusterID,
			GroupId:     ctx.MsgGroupID,
			Cluster:     clusterModel,
			Tags:        ctx.Cfg.ExtraTags,
		},
	}
	processResult = processors.ProcessResult{
		MetadataMessages: messages,
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
