package kubernetes

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	batchv1 "k8s.io/client-go/pkg/apis/batch/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/rest"

	payload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/metadata"
)

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
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve config: %s", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset: %s", err)
	}
	dr, err := clientset.Deployments("").List(v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployments: %s", err)
	}
	rr, err := clientset.ReplicaSets("").List(v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get replicasets: %s", err)
	}
	sr, err := clientset.Services("").List(v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get services: %s", err)
	}
	dsr, err := clientset.DaemonSets("").List(v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get daemonsets: %s", err)
	}
	jr, err := clientset.BatchV1Client.Jobs("").List(v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get jobs: %s", err)
	}
	pr, err := clientset.Pods("").List(v1.ListOptions{})
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

func parseDeployments(apiDeployments []v1beta1.Deployment) []*payload.KubeMetadataPayload_Deployment {
	dss := make([]*payload.KubeMetadataPayload_Deployment, len(apiDeployments))
	for _, d := range apiDeployments {
		dss = append(dss, &payload.KubeMetadataPayload_Deployment{
			Uid:       string(d.UID),
			Name:      d.Name,
			Namespace: d.Namespace,
		})
	}
	return dss
}

func parseReplicaSets(apiRs []v1beta1.ReplicaSet) []*payload.KubeMetadataPayload_ReplicaSet {
	rss := make([]*payload.KubeMetadataPayload_ReplicaSet, 0, len(apiRs))
	for _, ar := range apiRs {
		// Assumes only a single deployment per ReplicaSet
		var deployment string
		for _, o := range ar.OwnerReferences {
			if o.Kind == kindDeployment {
				deployment = o.Name
			}
		}
		rss = append(rss, &payload.KubeMetadataPayload_ReplicaSet{
			Uid:        string(ar.UID),
			Name:       ar.Name,
			Namespace:  ar.Namespace,
			Deployment: deployment,
		})
	}
	return rss
}

func parseServices(apiServices []v1.Service) []*payload.KubeMetadataPayload_Service {
	services := make([]*payload.KubeMetadataPayload_Service, 0, len(apiServices))
	for _, s := range apiServices {
		services = append(services, &payload.KubeMetadataPayload_Service{
			Uid:       string(s.UID),
			Name:      s.Name,
			Namespace: s.Namespace,
			Selector:  s.Spec.Selector,
			Type:      string(s.Spec.Type),
		})
	}
	return services
}

func parseJobs(apiJobs []batchv1.Job) []*payload.KubeMetadataPayload_Job {
	jobs := make([]*payload.KubeMetadataPayload_Job, 0, len(apiJobs))
	for _, j := range apiJobs {
		jobs = append(jobs, &payload.KubeMetadataPayload_Job{
			Uid:       string(j.UID),
			Name:      j.Name,
			Namespace: j.Namespace,
		})
	}
	return jobs
}

func parseDaemonSets(apiDs []v1beta1.DaemonSet) []*payload.KubeMetadataPayload_DaemonSet {
	daemonSets := make([]*payload.KubeMetadataPayload_DaemonSet, 0, len(apiDs))
	for _, ds := range apiDs {
		daemonSets = append(daemonSets, &payload.KubeMetadataPayload_DaemonSet{
			Uid:       string(ds.UID),
			Name:      ds.Name,
			Namespace: ds.Namespace,
		})
	}
	return daemonSets
}

func parsePods(
	apiPods []v1.Pod,
	services []*payload.KubeMetadataPayload_Service,
) ([]*payload.KubeMetadataPayload_Pod, []*payload.KubeMetadataPayload_Container) {
	pods := make([]*payload.KubeMetadataPayload_Pod, 0, len(apiPods))
	containers := make([]*payload.KubeMetadataPayload_Container, 0)
	for _, ap := range apiPods {
		cids := make([]string, 0, len(ap.Status.ContainerStatuses))
		for _, c := range ap.Status.ContainerStatuses {
			containers = append(containers, &payload.KubeMetadataPayload_Container{
				Name:    c.Name,
				Id:      c.ContainerID,
				Image:   c.Image,
				ImageId: c.ImageID,
			})
			cids = append(cids, c.ContainerID)
		}

		pod := &payload.KubeMetadataPayload_Pod{
			Uid:          string(ap.UID),
			Name:         ap.Name,
			Namespace:    ap.Namespace,
			HostIp:       ap.Status.HostIP,
			PodIp:        ap.Status.PodIP,
			Labels:       ap.Labels,
			ServiceUids:  findPodServices(ap.Namespace, ap.Labels, services),
			ContainerIds: cids,
		}
		setPodCreator(pod, ap.OwnerReferences)
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

func setPodCreator(pod *payload.KubeMetadataPayload_Pod, ownerRefs []v1.OwnerReference) {
	for _, o := range ownerRefs {
		switch o.Kind {
		case kindDaemonSet:
			pod.DaemonSet = o.Name
		case kindReplicaSet:
			pod.ReplicaSet = o.Name
		case kindReplicationController:
			pod.ReplicationController = o.Name
		case kindJob:
			pod.Job = o.Name
		}
	}
}
