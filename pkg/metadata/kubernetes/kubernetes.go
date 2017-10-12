// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package kubernetes

import (
	"context"
	"fmt"
	"net/http"

	log "github.com/cihub/seelog"
	"github.com/ericchiang/k8s"
	"github.com/ericchiang/k8s/api/v1"
	appsv1beta1 "github.com/ericchiang/k8s/apis/apps/v1beta1"
	batchv1 "github.com/ericchiang/k8s/apis/batch/v1"
	"github.com/ericchiang/k8s/apis/extensions/v1beta1"
	metav1 "github.com/ericchiang/k8s/apis/meta/v1"

	payload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/metadata"
)

var client *k8s.Client

const (
	createdByKey = "kubernetes.io/created-by"

	// Kube creator types, from created-by annotation.
	kindDaemonSet             = "DaemonSet"
	kindReplicaSet            = "ReplicaSet"
	kindReplicationController = "ReplicationController"
	kindDeployment            = "Deployment"
	kindJob                   = "Job"
)

// GetPayload returns a payload.KubernetesMetadata payload with metadata about
// the state of a Kubernetes cluster. We will use this metadata for tagging
// metrics and other services in the backend.
func GetPayload() (metadata.Payload, error) {
	ctx := context.Background()
	if client == nil {
		var err error
		client, err = k8s.NewInClusterClient()
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve config: %s", err)
		}
	}

	dr, err := client.AppsV1Beta1().ListDeployments(ctx, "")
	apiErr, ok := err.(*k8s.APIError)
	// Older K8s version don't have this API and will return a 404.
	if ok && apiErr.Code == http.StatusNotFound {
		dr = &appsv1beta1.DeploymentList{}
	} else if err != nil {
		// Allow Deployments API to fail, it's not available in all version and we
		// can also parse this data from the replica-set name on the backend.
		log.Warnf("Failed to retrieve Kubernetes deployments: %s", err)
		dr = &appsv1beta1.DeploymentList{}
	}
	rr, err := client.ExtensionsV1Beta1().ListReplicaSets(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get replicasets: %s", err)
	}
	sr, err := client.CoreV1().ListServices(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get services: %s", err)
	}
	dsr, err := client.ExtensionsV1Beta1().ListDaemonSets(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get daemonsets: %s", err)
	}
	jr, err := client.BatchV1().ListJobs(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get jobs: %s", err)
	}
	pr, err := client.CoreV1().ListPods(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get pods: %s", err)
	}

	deployments := parseDeployments(dr.Items)
	replicaSets := parseReplicaSets(rr.Items)
	services := parseServices(sr.Items)
	jobs := parseJobs(jr.Items)
	daemonSets := parseDaemonSets(dsr.Items)
	pods, containers := parsePods(pr.Items, services)
	return &payload.KubeMetadataPayload{
		Deployments: deployments,
		ReplicaSets: replicaSets,
		DaemonSets:  daemonSets,
		Services:    services,
		Jobs:        jobs,
		Pods:        pods,
		Containers:  containers,
	}, nil
}

func parseDeployments(apiDeployments []*appsv1beta1.Deployment) []*payload.KubeMetadataPayload_Deployment {
	dss := make([]*payload.KubeMetadataPayload_Deployment, 0, len(apiDeployments))
	for _, d := range apiDeployments {
		dss = append(dss, &payload.KubeMetadataPayload_Deployment{
			Uid:       d.Metadata.GetUid(),
			Name:      d.Metadata.GetName(),
			Namespace: d.Metadata.GetNamespace(),
		})
	}
	return dss
}

func parseReplicaSets(apiRs []*v1beta1.ReplicaSet) []*payload.KubeMetadataPayload_ReplicaSet {
	rss := make([]*payload.KubeMetadataPayload_ReplicaSet, 0, len(apiRs))
	for _, ar := range apiRs {
		// Assumes only a single deployment per ReplicaSet
		var deployment string
		for _, o := range ar.Metadata.OwnerReferences {
			if o.GetKind() == kindDeployment {
				deployment = o.GetName()
			}
		}
		rss = append(rss, &payload.KubeMetadataPayload_ReplicaSet{
			Uid:        ar.Metadata.GetUid(),
			Name:       ar.Metadata.GetName(),
			Namespace:  ar.Metadata.GetNamespace(),
			Deployment: deployment,
		})
	}
	return rss
}

func parseServices(apiServices []*v1.Service) []*payload.KubeMetadataPayload_Service {
	services := make([]*payload.KubeMetadataPayload_Service, 0, len(apiServices))
	for _, s := range apiServices {
		services = append(services, &payload.KubeMetadataPayload_Service{
			Uid:       s.Metadata.GetUid(),
			Name:      s.Metadata.GetName(),
			Namespace: s.Metadata.GetNamespace(),
			Selector:  s.Spec.GetSelector(),
			Type:      s.Spec.GetType(),
		})
	}
	return services
}

func parseJobs(apiJobs []*batchv1.Job) []*payload.KubeMetadataPayload_Job {
	jobs := make([]*payload.KubeMetadataPayload_Job, 0, len(apiJobs))
	for _, j := range apiJobs {
		jobs = append(jobs, &payload.KubeMetadataPayload_Job{
			Uid:       j.Metadata.GetUid(),
			Name:      j.Metadata.GetName(),
			Namespace: j.Metadata.GetNamespace(),
		})
	}
	return jobs
}

func parseDaemonSets(apiDs []*v1beta1.DaemonSet) []*payload.KubeMetadataPayload_DaemonSet {
	daemonSets := make([]*payload.KubeMetadataPayload_DaemonSet, 0, len(apiDs))
	for _, ds := range apiDs {
		daemonSets = append(daemonSets, &payload.KubeMetadataPayload_DaemonSet{
			Uid:       ds.Metadata.GetUid(),
			Name:      ds.Metadata.GetName(),
			Namespace: ds.Metadata.GetNamespace(),
		})
	}
	return daemonSets
}

func parsePods(
	apiPods []*v1.Pod,
	services []*payload.KubeMetadataPayload_Service,
) ([]*payload.KubeMetadataPayload_Pod, []*payload.KubeMetadataPayload_Container) {
	pods := make([]*payload.KubeMetadataPayload_Pod, 0, len(apiPods))
	containers := make([]*payload.KubeMetadataPayload_Container, 0)
	for _, ap := range apiPods {
		cids := make([]string, 0, len(ap.Status.ContainerStatuses))
		for _, c := range ap.Status.ContainerStatuses {
			containers = append(containers, &payload.KubeMetadataPayload_Container{
				Name:    c.GetName(),
				Id:      c.GetContainerID(),
				Image:   c.GetImage(),
				ImageId: c.GetImageID(),
			})
			cids = append(cids, c.GetContainerID())
		}

		pm := ap.GetMetadata()
		pod := &payload.KubeMetadataPayload_Pod{
			Uid:          pm.GetUid(),
			Name:         pm.GetName(),
			Namespace:    pm.GetNamespace(),
			HostIp:       ap.Status.GetHostIP(),
			PodIp:        ap.Status.GetPodIP(),
			Labels:       ap.Metadata.GetLabels(),
			ServiceUids:  findPodServices(pm.GetNamespace(), pm.GetLabels(), services),
			ContainerIds: cids,
		}
		setPodCreator(pod, ap.Metadata.GetOwnerReferences())
		pods = append(pods, pod)
	}
	return pods, containers
}

func findPodServices(
	namespace string,
	labels map[string]string,
	services []*payload.KubeMetadataPayload_Service,
) []string {
	uids := make([]string, 0)
	for _, s := range services {
		if s.Namespace != namespace {
			continue
		}

		match := true
		for k, search := range s.Selector {
			if v, ok := labels[k]; !ok || v != search {
				match = false
				break
			}
		}
		if match {
			uids = append(uids, s.Uid)
		}
	}
	return uids
}

func setPodCreator(pod *payload.KubeMetadataPayload_Pod, ownerRefs []*metav1.OwnerReference) {
	for _, o := range ownerRefs {
		switch o.GetKind() {
		case kindDaemonSet:
			pod.DaemonSet = o.GetName()
		case kindReplicaSet:
			pod.ReplicaSet = o.GetName()
		case kindReplicationController:
			pod.ReplicationController = o.GetName()
		case kindJob:
			pod.Job = o.GetName()
		}
	}
}
