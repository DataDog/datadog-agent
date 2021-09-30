// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver,orchestrator

package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	jsoniter "github.com/json-iterator/go"
	"github.com/twmb/murmur3"
	v1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

func processStatefulSetList(statefulSetList []*v1.StatefulSet, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	statefulSetMsgs := make([]*model.StatefulSet, 0, len(statefulSetList))

	for _, statefulSet := range statefulSetList {
		if orchestrator.SkipKubernetesResource(statefulSet.UID, statefulSet.ResourceVersion, orchestrator.K8sStatefulSet) {
			continue
		}

		// extract statefulSet info
		statefulSetModel := extractStatefulSet(statefulSet)

		// k8s objects only have json "omitempty" annotations
		// and marshalling is more performant than YAML
		jsonStatefulSet, err := jsoniter.Marshal(statefulSet)
		if err != nil {
			log.Warnf("Could not marshal StatefulSet to JSON: %s", err)
			continue
		}
		statefulSetModel.Yaml = jsonStatefulSet

		statefulSetMsgs = append(statefulSetMsgs, statefulSetModel)
	}

	groupSize := orchestrator.GroupSize(len(statefulSetMsgs), cfg.MaxPerMessage)

	chunked := chunkStatefulSets(statefulSetMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorStatefulSet{
			ClusterName:  cfg.KubeClusterName,
			StatefulSets: chunked[i],
			GroupId:      groupID,
			GroupSize:    int32(groupSize),
			ClusterId:    clusterID,
			Tags:         cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d StatefulSets in %s", len(statefulSetMsgs), len(statefulSetList), time.Since(start))
	return messages, nil
}

func processDaemonSetList(daemonSetList []*v1.DaemonSet, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	daemonSetMsgs := make([]*model.DaemonSet, 0, len(daemonSetList))

	for _, daemonSet := range daemonSetList {
		if orchestrator.SkipKubernetesResource(daemonSet.UID, daemonSet.ResourceVersion, orchestrator.K8sDaemonSet) {
			continue
		}

		// extract daemonSet info
		daemonSetModel := extractDaemonSet(daemonSet)

		// k8s objects only have json "omitempty" annotations
		// and marshalling is more performant than YAML
		jsonDaemonSet, err := jsoniter.Marshal(daemonSet)
		if err != nil {
			log.Warnf("Could not marshal DaemonSet to JSON: %s", err)
			continue
		}
		daemonSetModel.Yaml = jsonDaemonSet

		daemonSetMsgs = append(daemonSetMsgs, daemonSetModel)
	}

	groupSize := orchestrator.GroupSize(len(daemonSetMsgs), cfg.MaxPerMessage)

	chunked := chunkDaemonSets(daemonSetMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorDaemonSet{
			ClusterName: cfg.KubeClusterName,
			DaemonSets:  chunked[i],
			GroupId:     groupID,
			GroupSize:   int32(groupSize),
			ClusterId:   clusterID,
			Tags:        cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d DaemonSets in %s", len(daemonSetMsgs), len(daemonSetList), time.Since(start))
	return messages, nil
}

func processCronJobList(cronJobList []*batchv1beta1.CronJob, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	cronJobMsgs := make([]*model.CronJob, 0, len(cronJobList))

	for _, cronJob := range cronJobList {
		if orchestrator.SkipKubernetesResource(cronJob.UID, cronJob.ResourceVersion, orchestrator.K8sCronJob) {
			continue
		}
		redact.RemoveLastAppliedConfigurationAnnotation(cronJob.Annotations)

		// extract cronJob info
		cronJobModel := extractCronJob(cronJob)
		// scrub & generate YAML
		if cfg.IsScrubbingEnabled {
			for c := 0; c < len(cronJob.Spec.JobTemplate.Spec.Template.Spec.InitContainers); c++ {
				redact.ScrubContainer(&cronJob.Spec.JobTemplate.Spec.Template.Spec.InitContainers[c], cfg.Scrubber)
			}
			for c := 0; c < len(cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers); c++ {
				redact.ScrubContainer(&cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[c], cfg.Scrubber)
			}
		}
		// k8s objects only have json "omitempty" annotations
		// and marshalling is more performant than YAML
		jsonCronJob, err := jsoniter.Marshal(cronJob)
		if err != nil {
			log.Warnf("Could not marshal CronJob to JSON: %s", err)
			continue
		}
		cronJobModel.Yaml = jsonCronJob

		cronJobMsgs = append(cronJobMsgs, cronJobModel)
	}

	groupSize := orchestrator.GroupSize(len(cronJobMsgs), cfg.MaxPerMessage)

	chunked := chunkCronJobs(cronJobMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorCronJob{
			ClusterName: cfg.KubeClusterName,
			CronJobs:    chunked[i],
			GroupId:     groupID,
			GroupSize:   int32(groupSize),
			ClusterId:   clusterID,
			Tags:        cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d CronJobs in %s", len(cronJobMsgs), len(cronJobList), time.Since(start))
	return messages, nil
}

// chunkCronJobs formats and chunks cronJobs into a slice of chunks using a specific number of chunks.
func chunkCronJobs(cronJobs []*model.CronJob, chunkCount, chunkSize int) [][]*model.CronJob {
	chunks := make([][]*model.CronJob, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(cronJobs), chunkCount, chunkSize, counter)
		chunks = append(chunks, cronJobs[chunkStart:chunkEnd])
	}

	return chunks
}

func processDeploymentList(deploymentList []*v1.Deployment, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	deployMsgs := make([]*model.Deployment, 0, len(deploymentList))

	for d := 0; d < len(deploymentList); d++ {
		depl := deploymentList[d]
		if orchestrator.SkipKubernetesResource(depl.UID, depl.ResourceVersion, orchestrator.K8sDeployment) {
			continue
		}
		redact.RemoveLastAppliedConfigurationAnnotation(depl.Annotations)

		// extract deployment info
		deployModel := extractDeployment(depl)
		// scrub & generate YAML
		if cfg.IsScrubbingEnabled {
			for c := 0; c < len(depl.Spec.Template.Spec.InitContainers); c++ {
				redact.ScrubContainer(&depl.Spec.Template.Spec.InitContainers[c], cfg.Scrubber)
			}
			for c := 0; c < len(deploymentList[d].Spec.Template.Spec.Containers); c++ {
				redact.ScrubContainer(&depl.Spec.Template.Spec.Containers[c], cfg.Scrubber)
			}
		}
		// k8s objects only have json "omitempty" annotations
		// and marshalling is more performant than YAML
		jsonDeploy, err := jsoniter.Marshal(depl)
		if err != nil {
			log.Warnf("Could not marshal Deployment to JSON: %s", err)
			continue
		}
		deployModel.Yaml = jsonDeploy

		deployMsgs = append(deployMsgs, deployModel)
	}

	groupSize := orchestrator.GroupSize(len(deployMsgs), cfg.MaxPerMessage)

	chunked := chunkDeployments(deployMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorDeployment{
			ClusterName: cfg.KubeClusterName,
			Deployments: chunked[i],
			GroupId:     groupID,
			GroupSize:   int32(groupSize),
			ClusterId:   clusterID,
			Tags:        cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d Deployments in %s", len(deployMsgs), len(deploymentList), time.Since(start))
	return messages, nil
}

// chunkDeployments formats and chunks the deployments into a slice of chunks using a specific number of chunks.
func chunkDeployments(deploys []*model.Deployment, chunkCount, chunkSize int) [][]*model.Deployment {
	chunks := make([][]*model.Deployment, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(deploys), chunkCount, chunkSize, counter)
		chunks = append(chunks, deploys[chunkStart:chunkEnd])
	}

	return chunks
}

func processJobList(jobList []*batchv1.Job, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	jobMsgs := make([]*model.Job, 0, len(jobList))

	for _, job := range jobList {
		if orchestrator.SkipKubernetesResource(job.UID, job.ResourceVersion, orchestrator.K8sJob) {
			continue
		}
		redact.RemoveLastAppliedConfigurationAnnotation(job.Annotations)

		// extract job info
		jobModel := extractJob(job)
		// scrub & generate YAML
		if cfg.IsScrubbingEnabled {
			for c := 0; c < len(job.Spec.Template.Spec.InitContainers); c++ {
				redact.ScrubContainer(&job.Spec.Template.Spec.InitContainers[c], cfg.Scrubber)
			}
			for c := 0; c < len(job.Spec.Template.Spec.Containers); c++ {
				redact.ScrubContainer(&job.Spec.Template.Spec.Containers[c], cfg.Scrubber)
			}
		}
		// k8s objects only have json "omitempty" annotations
		// and marshalling is more performant than YAML
		jsonJob, err := jsoniter.Marshal(job)
		if err != nil {
			log.Warnf("Could not marshal Job to JSON: %s", err)
			continue
		}
		jobModel.Yaml = jsonJob

		jobMsgs = append(jobMsgs, jobModel)
	}

	groupSize := orchestrator.GroupSize(len(jobMsgs), cfg.MaxPerMessage)

	chunked := chunkJobs(jobMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorJob{
			ClusterName: cfg.KubeClusterName,
			Jobs:        chunked[i],
			GroupId:     groupID,
			GroupSize:   int32(groupSize),
			ClusterId:   clusterID,
			Tags:        cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d Jobs in %s", len(jobMsgs), len(jobList), time.Since(start))
	return messages, nil
}

// chunkJobs formats and chunks jobs into a slice of chunks using a specific number of chunks.
func chunkJobs(jobs []*model.Job, chunkCount, chunkSize int) [][]*model.Job {
	chunks := make([][]*model.Job, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(jobs), chunkCount, chunkSize, counter)
		chunks = append(chunks, jobs[chunkStart:chunkEnd])
	}

	return chunks
}

func processReplicaSetList(rsList []*v1.ReplicaSet, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	rsMsgs := make([]*model.ReplicaSet, 0, len(rsList))

	for rs := 0; rs < len(rsList); rs++ {
		r := rsList[rs]
		if orchestrator.SkipKubernetesResource(r.UID, r.ResourceVersion, orchestrator.K8sReplicaSet) {
			continue
		}
		redact.RemoveLastAppliedConfigurationAnnotation(r.Annotations)

		// extract replica set info
		rsModel := extractReplicaSet(r)

		// scrub & generate YAML
		if cfg.IsScrubbingEnabled {
			for c := 0; c < len(r.Spec.Template.Spec.InitContainers); c++ {
				redact.ScrubContainer(&r.Spec.Template.Spec.InitContainers[c], cfg.Scrubber)
			}
			for c := 0; c < len(r.Spec.Template.Spec.Containers); c++ {
				redact.ScrubContainer(&r.Spec.Template.Spec.Containers[c], cfg.Scrubber)
			}
		}

		// k8s objects only have json "omitempty" annotations
		// and marshalling is more performant than YAML
		jsonRS, err := jsoniter.Marshal(r)
		if err != nil {
			log.Warnf("Could not marshal ReplicaSet to JSON: %s", err)
			continue
		}
		rsModel.Yaml = jsonRS

		rsMsgs = append(rsMsgs, rsModel)
	}

	groupSize := orchestrator.GroupSize(len(rsMsgs), cfg.MaxPerMessage)

	chunked := chunkReplicaSets(rsMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorReplicaSet{
			ClusterName: cfg.KubeClusterName,
			ReplicaSets: chunked[i],
			GroupId:     groupID,
			GroupSize:   int32(groupSize),
			ClusterId:   clusterID,
			Tags:        cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d ReplicaSets in %s", len(rsMsgs), len(rsList), time.Since(start))
	return messages, nil
}

// chunkReplicaSets formats and chunks the replica sets into a slice of chunks using a specific number of chunks.
func chunkReplicaSets(replicaSets []*model.ReplicaSet, chunkCount, chunkSize int) [][]*model.ReplicaSet {
	chunks := make([][]*model.ReplicaSet, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(replicaSets), chunkCount, chunkSize, counter)
		chunks = append(chunks, replicaSets[chunkStart:chunkEnd])
	}

	return chunks
}

// processServiceList process a service list into process messages.
func processServiceList(serviceList []*corev1.Service, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	serviceMsgs := make([]*model.Service, 0, len(serviceList))

	for s := 0; s < len(serviceList); s++ {
		svc := serviceList[s]
		if orchestrator.SkipKubernetesResource(svc.UID, svc.ResourceVersion, orchestrator.K8sService) {
			continue
		}

		serviceModel := extractService(svc)

		// k8s objects only have json "omitempty" annotations
		// + marshalling is more performant than YAML
		jsonSvc, err := jsoniter.Marshal(svc)
		if err != nil {
			log.Warnf("Could not marshal Service to JSON: %s", err)
			continue
		}
		serviceModel.Yaml = jsonSvc

		serviceMsgs = append(serviceMsgs, serviceModel)
	}

	groupSize := orchestrator.GroupSize(len(serviceMsgs), cfg.MaxPerMessage)

	chunks := chunkServices(serviceMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)

	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorService{
			ClusterName: cfg.KubeClusterName,
			ClusterId:   clusterID,
			GroupId:     groupID,
			GroupSize:   int32(groupSize),
			Services:    chunks[i],
			Tags:        cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d Services in %s", len(serviceMsgs), len(serviceList), time.Since(start))
	return messages, nil
}

// chunkServices chunks the given list of services, honoring the given chunk count and size.
// The last chunk may be smaller than the others.
func chunkServices(services []*model.Service, chunkCount, chunkSize int) [][]*model.Service {
	chunks := make([][]*model.Service, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(services), chunkCount, chunkSize, counter)
		chunks = append(chunks, services[chunkStart:chunkEnd])
	}

	return chunks
}

func fillClusterResourceVersion(c *model.Cluster) error {
	// Marshal the cluster message to JSON.
	marshaller := jsoniter.ConfigCompatibleWithStandardLibrary
	jsonClusterModel, err := marshaller.Marshal(c)
	if err != nil {
		return fmt.Errorf("could not marshal cluster model to JSON: %s", err)
	}

	version := murmur3.Sum64(jsonClusterModel)
	c.ResourceVersion = fmt.Sprint(version)

	return nil
}

// getKubeSystemCreationTimeStamp returns the timestamp of the kube-system namespace from the cluster
// We use it as the cluster timestamp as it is the first namespace which have been created by the cluster.
func getKubeSystemCreationTimeStamp(coreClient corev1client.CoreV1Interface) (metav1.Time, error) {
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

// processNodesList process a nodes list into nodes process messages and cluster process message.
// error can only be returned if cluster resource are being collected as it needs information from the apiserver.
func processNodesList(nodesList []*corev1.Node, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, model.Cluster, error) {
	start := time.Now()
	nodeMsgs := make([]*model.Node, 0, len(nodesList))
	kubeletVersions := map[string]int32{}
	podCap := uint32(0)
	podAllocatable := uint32(0)
	memoryAllocatable := uint64(0)
	memoryCap := uint64(0)
	cpuAllocatable := uint64(0)
	cpuCap := uint64(0)

	for s := 0; s < len(nodesList); s++ {
		node := nodesList[s]
		kubeletVersions[node.Status.NodeInfo.KubeletVersion]++
		podCap += uint32(node.Status.Capacity.Pods().Value())
		podAllocatable += uint32(node.Status.Allocatable.Pods().Value())
		memoryAllocatable += uint64(node.Status.Allocatable.Memory().Value())
		memoryCap += uint64(node.Status.Capacity.Memory().Value())
		cpuAllocatable += uint64(node.Status.Allocatable.Cpu().MilliValue())
		cpuCap += uint64(node.Status.Capacity.Cpu().MilliValue())

		if orchestrator.SkipKubernetesResource(node.UID, node.ResourceVersion, orchestrator.K8sNode) {
			continue
		}

		nodeModel := extractNode(node)
		// k8s objects only have json "omitempty" annotations
		// + marshalling is more performant than YAML
		jsonNode, err := jsoniter.Marshal(node)
		if err != nil {
			log.Warnf("Could not marshal Node to JSON: %s", err)
			continue
		}
		nodeModel.Yaml = jsonNode

		// additional tags
		nodeStatusTags := convertNodeStatusToTags(nodeModel.Status.Status)
		nodeModel.Tags = append(nodeModel.Tags, nodeStatusTags...)

		for _, role := range nodeModel.Roles {
			nodeModel.Tags = append(nodeModel.Tags, fmt.Sprintf("%s:%s", kubernetes.KubeNodeRoleTagName, strings.ToLower(role)))
		}

		nodeMsgs = append(nodeMsgs, nodeModel)
	}

	groupSize := orchestrator.GroupSize(len(nodeMsgs), cfg.MaxPerMessage)

	chunks := chunkNodes(nodeMsgs, groupSize, cfg.MaxPerMessage)
	nodeMessages := make([]model.MessageBody, 0, groupSize)

	for i := 0; i < groupSize; i++ {
		nodeMessages = append(nodeMessages, &model.CollectorNode{
			ClusterName: cfg.KubeClusterName,
			ClusterId:   clusterID,
			GroupId:     groupID,
			GroupSize:   int32(groupSize),
			Nodes:       chunks[i],
			Tags:        cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d Nodes in %s", len(nodeMsgs), len(nodesList), time.Since(start))
	return nodeMessages, model.Cluster{
		KubeletVersions:   kubeletVersions,
		PodCapacity:       podCap,
		PodAllocatable:    podAllocatable,
		MemoryAllocatable: memoryAllocatable,
		MemoryCapacity:    memoryCap,
		CpuAllocatable:    cpuAllocatable,
		CpuCapacity:       cpuCap,
		NodeCount:         int32(len(nodesList)),
	}, nil
}

func extractClusterMessage(cfg *config.OrchestratorConfig, clusterID string, client *apiserver.APIClient, groupID int32, cluster model.Cluster) (*model.CollectorCluster, error) {
	kubeSystemCreationTimestamp, err := getKubeSystemCreationTimeStamp(client.Cl.CoreV1())
	if err != nil {
		log.Warnf("Error getting server kube system creation timestamp: %s", err.Error())
		return nil, err
	}

	apiVersion, err := client.Cl.Discovery().ServerVersion()
	if err != nil {
		log.Warnf("Error getting server apiVersion: %s", err.Error())
		return nil, err
	}

	cluster.ApiServerVersions = map[string]int32{apiVersion.String(): 1}

	if !kubeSystemCreationTimestamp.IsZero() {
		cluster.CreationTimestamp = kubeSystemCreationTimestamp.Unix()
	}

	if err := fillClusterResourceVersion(&cluster); err != nil {
		log.Warnf("Failed to compute cluster resource version: %s", err.Error())
		return nil, err
	}

	if orchestrator.SkipKubernetesResource(types.UID(clusterID), cluster.ResourceVersion, orchestrator.K8sCluster) {
		stats := orchestrator.CheckStats{
			CacheHits: 1,
			CacheMiss: 0,
			NodeType:  orchestrator.K8sCluster,
		}
		orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sCluster), stats, orchestrator.NoExpiration)
		return nil, nil
	}

	clusterMessage := &model.CollectorCluster{
		ClusterName: cfg.KubeClusterName,
		ClusterId:   clusterID,
		GroupId:     groupID,
		Cluster:     &cluster,
		Tags:        cfg.ExtraTags,
	}
	return clusterMessage, nil
}

// chunkNodes chunks the given list of nodes, honoring the given chunk count and size.
// The last chunk may be smaller than the others.
func chunkNodes(nodes []*model.Node, chunkCount, chunkSize int) [][]*model.Node {
	chunks := make([][]*model.Node, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(nodes), chunkCount, chunkSize, counter)
		chunks = append(chunks, nodes[chunkStart:chunkEnd])
	}

	return chunks
}

// chunkDaemonSets chunks the given list of daemonSets, honoring the given chunk count and size.
// The last chunk may be smaller than the others.
func chunkDaemonSets(daemonSets []*model.DaemonSet, chunkCount, chunkSize int) [][]*model.DaemonSet {
	chunks := make([][]*model.DaemonSet, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(daemonSets), chunkCount, chunkSize, counter)
		chunks = append(chunks, daemonSets[chunkStart:chunkEnd])
	}

	return chunks
}

// chunkStatefulSets chunks the given list of statefulsets, honoring the given chunk count and size.
// The last chunk may be smaller than the others.
func chunkStatefulSets(statefulSets []*model.StatefulSet, chunkCount, chunkSize int) [][]*model.StatefulSet {
	chunks := make([][]*model.StatefulSet, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(statefulSets), chunkCount, chunkSize, counter)
		chunks = append(chunks, statefulSets[chunkStart:chunkEnd])
	}

	return chunks
}

// processPersistentVolumeList process a PV list into process messages.
func processPersistentVolumeList(pvList []*corev1.PersistentVolume, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	pvMsgs := make([]*model.PersistentVolume, 0, len(pvList))

	for s := 0; s < len(pvList); s++ {
		pv := pvList[s]
		if orchestrator.SkipKubernetesResource(pv.UID, pv.ResourceVersion, orchestrator.K8sPersistentVolume) {
			continue
		}

		pvModel := extractPersistentVolume(pv)

		// k8s objects only have json "omitempty" annotations
		// + marshalling is more performant than YAML
		jsonPv, err := jsoniter.Marshal(pv)
		if err != nil {
			log.Warnf("Could not marshal PersistentVolume to JSON: %s", err)
			continue
		}
		pvModel.Yaml = jsonPv

		addAdditionalPVTags(pvModel)

		pvMsgs = append(pvMsgs, pvModel)
	}

	groupSize := orchestrator.GroupSize(len(pvMsgs), cfg.MaxPerMessage)

	chunks := chunkPersistentVolumes(pvMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)

	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorPersistentVolume{
			ClusterName:       cfg.KubeClusterName,
			ClusterId:         clusterID,
			GroupId:           groupID,
			GroupSize:         int32(groupSize),
			PersistentVolumes: chunks[i],
			Tags:              cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d Persistent volumes in %s", len(pvMsgs), len(pvList), time.Since(start))
	return messages, nil
}

func addAdditionalPVTags(pvModel *model.PersistentVolume) {
	// additional tags
	pvPhaseTag := "pv_phase" + pvModel.Status.Phase
	pvTypeTag := "pv_type" + pvModel.Spec.PersistentVolumeType
	pvModel.Tags = append(pvModel.Tags, pvPhaseTag)
	pvModel.Tags = append(pvModel.Tags, pvTypeTag)
}

// chunkPersistentVolumes chunks the given list of pv, honoring the given chunk count and size.
// The last chunk may be smaller than the others.
func chunkPersistentVolumes(persistentVolumes []*model.PersistentVolume, chunkCount, chunkSize int) [][]*model.PersistentVolume {
	chunks := make([][]*model.PersistentVolume, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(persistentVolumes), chunkCount, chunkSize, counter)
		chunks = append(chunks, persistentVolumes[chunkStart:chunkEnd])
	}

	return chunks
}

// processPersistentVolumeClaimList process a PVC list into process messages.
func processPersistentVolumeClaimList(pvcList []*corev1.PersistentVolumeClaim, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	pvcMsgs := make([]*model.PersistentVolumeClaim, 0, len(pvcList))

	for s := 0; s < len(pvcList); s++ {
		pvc := pvcList[s]
		if orchestrator.SkipKubernetesResource(pvc.UID, pvc.ResourceVersion, orchestrator.K8sPersistentVolumeClaim) {
			continue
		}

		pvModel := extractPersistentVolumeClaim(pvc)

		// k8s objects only have json "omitempty" annotations
		// + marshalling is more performant than YAML
		jsonPvc, err := jsoniter.Marshal(pvc)
		if err != nil {
			log.Warnf("Could not marshal PersistentVolumeClaim to JSON: %s", err)
			continue
		}
		pvModel.Yaml = jsonPvc

		pvcMsgs = append(pvcMsgs, pvModel)
	}

	groupSize := orchestrator.GroupSize(len(pvcMsgs), cfg.MaxPerMessage)

	chunks := chunkPersistentVolumeClaims(pvcMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)

	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorPersistentVolumeClaim{
			ClusterName:            cfg.KubeClusterName,
			ClusterId:              clusterID,
			GroupId:                groupID,
			GroupSize:              int32(groupSize),
			PersistentVolumeClaims: chunks[i],
			Tags:                   cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d Persistent volume claims in %s", len(pvcMsgs), len(pvcList), time.Since(start))
	return messages, nil
}

// chunkPersistentVolumeClaims chunks the given list of pvc, honoring the given chunk count and size.
// The last chunk may be smaller than the others.
func chunkPersistentVolumeClaims(pvcs []*model.PersistentVolumeClaim, chunkCount, chunkSize int) [][]*model.PersistentVolumeClaim {
	chunks := make([][]*model.PersistentVolumeClaim, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(pvcs), chunkCount, chunkSize, counter)
		chunks = append(chunks, pvcs[chunkStart:chunkEnd])
	}

	return chunks
}

func processRoleList(roleList []*rbacv1.Role, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	roleMsgs := make([]*model.Role, 0, len(roleList))

	for _, role := range roleList {
		if orchestrator.SkipKubernetesResource(role.UID, role.ResourceVersion, orchestrator.K8sRole) {
			continue
		}

		roleModel := extractRole(role)

		// k8s objects only have json "omitempty" annotations
		// + marshalling is more performant than YAML
		jsonRole, err := jsoniter.Marshal(role)
		if err != nil {
			log.Warnf("Could not marshal Role to JSON: %s", err)
			continue
		}
		roleModel.Yaml = jsonRole

		roleMsgs = append(roleMsgs, roleModel)
	}

	groupSize := orchestrator.GroupSize(len(roleMsgs), cfg.MaxPerMessage)

	chunks := chunkRoles(roleMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)

	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorRole{
			ClusterName: cfg.KubeClusterName,
			ClusterId:   clusterID,
			GroupId:     groupID,
			GroupSize:   int32(groupSize),
			Roles:       chunks[i],
			Tags:        cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d Roles in %s", len(roleMsgs), len(roleList), time.Since(start))
	return messages, nil
}

// chunkRoles chunks the given list of roles, honoring the given chunk count and size.
// The last chunk may be smaller than the others.
func chunkRoles(roles []*model.Role, chunkCount, chunkSize int) [][]*model.Role {
	chunks := make([][]*model.Role, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(roles), chunkCount, chunkSize, counter)
		chunks = append(chunks, roles[chunkStart:chunkEnd])
	}

	return chunks
}

func processRoleBindingList(roleBindingList []*rbacv1.RoleBinding, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	roleBindingMsgs := make([]*model.RoleBinding, 0, len(roleBindingList))

	for _, roleBinding := range roleBindingList {
		if orchestrator.SkipKubernetesResource(roleBinding.UID, roleBinding.ResourceVersion, orchestrator.K8sRoleBinding) {
			continue
		}

		roleBindingModel := extractRoleBinding(roleBinding)

		// k8s objects only have json "omitempty" annotations
		// + marshalling is more performant than YAML
		jsonRole, err := jsoniter.Marshal(roleBinding)
		if err != nil {
			log.Warnf("Could not marshal RoleBinding to JSON: %s", err)
			continue
		}
		roleBindingModel.Yaml = jsonRole

		roleBindingMsgs = append(roleBindingMsgs, roleBindingModel)
	}

	groupSize := orchestrator.GroupSize(len(roleBindingMsgs), cfg.MaxPerMessage)

	chunks := chunkRoleBindings(roleBindingMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)

	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorRoleBinding{
			ClusterName:  cfg.KubeClusterName,
			ClusterId:    clusterID,
			GroupId:      groupID,
			GroupSize:    int32(groupSize),
			RoleBindings: chunks[i],
			Tags:         cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d RoleBindings in %s", len(roleBindingMsgs), len(roleBindingList), time.Since(start))
	return messages, nil
}

// chunkRoleBindings chunks the given list of role bindings, honoring the given
// chunk count and size.  The last chunk may be smaller than the others.
func chunkRoleBindings(roleBindings []*model.RoleBinding, chunkCount, chunkSize int) [][]*model.RoleBinding {
	chunks := make([][]*model.RoleBinding, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(roleBindings), chunkCount, chunkSize, counter)
		chunks = append(chunks, roleBindings[chunkStart:chunkEnd])
	}

	return chunks
}

func processClusterRoleList(clusterRoleList []*rbacv1.ClusterRole, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	clusterRoleMsgs := make([]*model.ClusterRole, 0, len(clusterRoleList))

	for _, clusterRole := range clusterRoleList {
		if orchestrator.SkipKubernetesResource(clusterRole.UID, clusterRole.ResourceVersion, orchestrator.K8sClusterRole) {
			continue
		}

		clusterRoleModel := extractClusterRole(clusterRole)

		// k8s objects only have json "omitempty" annotations
		// + marshalling is more performant than YAML
		jsonRole, err := jsoniter.Marshal(clusterRole)
		if err != nil {
			log.Warnf("Could not marshal ClusterRole to JSON: %s", err)
			continue
		}
		clusterRoleModel.Yaml = jsonRole

		clusterRoleMsgs = append(clusterRoleMsgs, clusterRoleModel)
	}

	groupSize := orchestrator.GroupSize(len(clusterRoleMsgs), cfg.MaxPerMessage)

	chunks := chunkClusterRoles(clusterRoleMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)

	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorClusterRole{
			ClusterName:  cfg.KubeClusterName,
			ClusterId:    clusterID,
			GroupId:      groupID,
			GroupSize:    int32(groupSize),
			ClusterRoles: chunks[i],
			Tags:         cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d ClusterRoles in %s", len(clusterRoleMsgs), len(clusterRoleList), time.Since(start))
	return messages, nil
}

// chunkclusterRoles chunks the given list of cluster roles, honoring the given
// chunk count and size.  The last chunk may be smaller than the others.
func chunkClusterRoles(clusterRoles []*model.ClusterRole, chunkCount, chunkSize int) [][]*model.ClusterRole {
	chunks := make([][]*model.ClusterRole, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(clusterRoles), chunkCount, chunkSize, counter)
		chunks = append(chunks, clusterRoles[chunkStart:chunkEnd])
	}

	return chunks
}

func processClusterRoleBindingList(clusterRoleBindingList []*rbacv1.ClusterRoleBinding, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	clusterRoleBindingMsgs := make([]*model.ClusterRoleBinding, 0, len(clusterRoleBindingList))

	for _, clusterRoleBinding := range clusterRoleBindingList {
		if orchestrator.SkipKubernetesResource(clusterRoleBinding.UID, clusterRoleBinding.ResourceVersion, orchestrator.K8sClusterRoleBinding) {
			continue
		}

		clusterRoleBindingModel := extractClusterRoleBinding(clusterRoleBinding)

		// k8s objects only have json "omitempty" annotations
		// + marshalling is more performant than YAML
		jsonRole, err := jsoniter.Marshal(clusterRoleBinding)
		if err != nil {
			log.Warnf("Could not marshal ClusterRoleBinding to JSON: %s", err)
			continue
		}
		clusterRoleBindingModel.Yaml = jsonRole

		clusterRoleBindingMsgs = append(clusterRoleBindingMsgs, clusterRoleBindingModel)
	}

	groupSize := orchestrator.GroupSize(len(clusterRoleBindingMsgs), cfg.MaxPerMessage)

	chunks := chunkClusterRoleBindings(clusterRoleBindingMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)

	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorClusterRoleBinding{
			ClusterName:         cfg.KubeClusterName,
			ClusterId:           clusterID,
			GroupId:             groupID,
			GroupSize:           int32(groupSize),
			ClusterRoleBindings: chunks[i],
			Tags:                cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d ClusterRoleBindings in %s", len(clusterRoleBindingMsgs), len(clusterRoleBindingList), time.Since(start))
	return messages, nil
}

// chunkClusterRoleBindings chunks the given list of cluster role bindings, honoring the given
// chunk count and size.  The last chunk may be smaller than the others.
func chunkClusterRoleBindings(clusterRoleBindings []*model.ClusterRoleBinding, chunkCount, chunkSize int) [][]*model.ClusterRoleBinding {
	chunks := make([][]*model.ClusterRoleBinding, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(clusterRoleBindings), chunkCount, chunkSize, counter)
		chunks = append(chunks, clusterRoleBindings[chunkStart:chunkEnd])
	}

	return chunks
}

func processServiceAccountList(serviceAccountList []*corev1.ServiceAccount, groupID int32, cfg *config.OrchestratorConfig, clusterID string) ([]model.MessageBody, error) {
	start := time.Now()
	serviceAccountMsgs := make([]*model.ServiceAccount, 0, len(serviceAccountList))

	for _, serviceAcount := range serviceAccountList {
		if orchestrator.SkipKubernetesResource(serviceAcount.UID, serviceAcount.ResourceVersion, orchestrator.K8sServiceAccount) {
			continue
		}

		clusterRoleBindingModel := extractServiceAccount(serviceAcount)

		// k8s objects only have json "omitempty" annotations
		// + marshalling is more performant than YAML
		jsonRole, err := jsoniter.Marshal(serviceAcount)
		if err != nil {
			log.Warnf("Could not marshal ServiceAccount to JSON: %s", err)
			continue
		}
		clusterRoleBindingModel.Yaml = jsonRole

		serviceAccountMsgs = append(serviceAccountMsgs, clusterRoleBindingModel)
	}

	groupSize := orchestrator.GroupSize(len(serviceAccountMsgs), cfg.MaxPerMessage)

	chunks := chunkServiceAccounts(serviceAccountMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)

	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorServiceAccount{
			ClusterName:     cfg.KubeClusterName,
			ClusterId:       clusterID,
			GroupId:         groupID,
			GroupSize:       int32(groupSize),
			ServiceAccounts: chunks[i],
			Tags:            cfg.ExtraTags,
		})
	}

	log.Debugf("Collected & enriched %d out of %d ServiceAccounts in %s", len(serviceAccountMsgs), len(serviceAccountList), time.Since(start))
	return messages, nil
}

// chunkServiceAccounts chunks the given list of cluster service accounts, honoring the given
// chunk count and size.  The last chunk may be smaller than the others.
func chunkServiceAccounts(serviceAccounts []*model.ServiceAccount, chunkCount, chunkSize int) [][]*model.ServiceAccount {
	chunks := make([][]*model.ServiceAccount, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(serviceAccounts), chunkCount, chunkSize, counter)
		chunks = append(chunks, serviceAccounts[chunkStart:chunkEnd])
	}

	return chunks
}
