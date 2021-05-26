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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

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
			log.Warnf("Could not marshal daemon sets to JSON: %s", err)
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

	log.Debugf("Collected & enriched %d out of %d daemon sets in %s", len(daemonSetMsgs), len(daemonSetList), time.Since(start))
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
			log.Warnf("Could not marshal cron job to JSON: %s", err)
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

	log.Debugf("Collected & enriched %d out of %d cron jobs in %s", len(cronJobMsgs), len(cronJobList), time.Since(start))
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
			log.Warnf("Could not marshal deployment to JSON: %s", err)
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

	log.Debugf("Collected & enriched %d out of %d deployments in %s", len(deployMsgs), len(deploymentList), time.Since(start))
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
			log.Warnf("Could not marshal job to JSON: %s", err)
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

	log.Debugf("Collected & enriched %d out of %d jobs in %s", len(jobMsgs), len(jobList), time.Since(start))
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
			log.Warnf("Could not marshal replica set to JSON: %s", err)
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

	log.Debugf("Collected & enriched %d out of %d replica sets in %s", len(rsMsgs), len(rsList), time.Since(start))
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
			log.Warnf("Could not marshal service to JSON: %s", err)
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

	log.Debugf("Collected & enriched %d out of %d services in %s", len(serviceMsgs), len(serviceList), time.Since(start))
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
			log.Warnf("Could not marshal node to JSON: %s", err)
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

	log.Debugf("Collected & enriched %d out of %d nodes in %s", len(nodeMsgs), len(nodesList), time.Since(start))
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
