package testdata

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	appsV1 "k8s.io/api/apps/v1"
	batchV1 "k8s.io/api/batch/v1"
	batchV1B1 "k8s.io/api/batch/v1beta1"
	coreV1 "k8s.io/api/core/v1"
	extensionsV1B1 "k8s.io/api/extensions/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"time"
)

func NewMockBenchmarkAPICollectorClient() apiserver.APICollectorClient {
	volumeLocation := coreV1.HostPathFileOrCreate
	return MockBenchmarkAPICollectorClient{
		creationTime:   v1.Time{Time: time.Now().Add(-1 * time.Hour)},
		replicas:       1,
		parralelism:    2,
		backoffLimit:   5,
		volumeLocation: volumeLocation,
		gcePersistentDisk: coreV1.GCEPersistentDiskVolumeSource{
			PDName: "name-of-the-gce-persistent-disk",
		},
		awsElasticBlockStore: coreV1.AWSElasticBlockStoreVolumeSource{
			VolumeID: "id-of-the-aws-block-store",
		},
		hostPath: coreV1.HostPathVolumeSource{
			Path: "some/path/to/the/volume",
			Type: &volumeLocation,
		},
	}
}

//
type MockBenchmarkAPICollectorClient struct {
	creationTime         v1.Time
	replicas             int32
	parralelism          int32
	backoffLimit         int32
	volumeLocation       coreV1.HostPathType
	gcePersistentDisk    coreV1.GCEPersistentDiskVolumeSource
	awsElasticBlockStore coreV1.AWSElasticBlockStoreVolumeSource
	hostPath             coreV1.HostPathVolumeSource
	apiserver.APICollectorClient
}

func (m MockBenchmarkAPICollectorClient) GetConfigMaps() ([]coreV1.ConfigMap, error) {
	configMaps := make([]coreV1.ConfigMap, 0)
	for i := 1; i <= 3; i++ {

		configMap := coreV1.ConfigMap{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-configmap-%d", i),
				CreationTimestamp: m.creationTime,
				Namespace:         "test-namespace",
				UID:               types.UID(fmt.Sprintf("test-configmap-%d", i)),
				GenerateName:      "",
			},
		}

		if i == 1 {
			configMap.Data = map[string]string{
				"key1": "value1",
				"key2": "longersecretvalue2",
			}
		}

		if i != 3 {
			configMap.Labels = map[string]string{
				"test": "label",
			}
		}

		configMaps = append(configMaps, configMap)
	}

	return configMaps, nil
}

func (m MockBenchmarkAPICollectorClient) GetCronJobs() ([]batchV1B1.CronJob, error) {
	cronJobs := make([]batchV1B1.CronJob, 0)
	for i := 1; i <= 2; i++ {

		var jobLinks []coreV1.ObjectReference
		if i == 1 {
			jobLinks = []coreV1.ObjectReference{
				{Name: "job-1", Namespace: "test-namespace-1"},
				{Name: "job-2", Namespace: "test-namespace-2"},
			}
		} else {
			jobLinks = []coreV1.ObjectReference{}
		}

		cronJobs = append(cronJobs, batchV1B1.CronJob{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-cronjob-%d", i),
				CreationTimestamp: m.creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-cronjob-%d", i)),
				GenerateName: "",
			},
			Spec: batchV1B1.CronJobSpec{
				Schedule:          "0 0 * * *",
				ConcurrencyPolicy: batchV1B1.AllowConcurrent,
			},
			Status: batchV1B1.CronJobStatus{
				Active: jobLinks,
			},
		})
	}

	return cronJobs, nil
}

func (m MockBenchmarkAPICollectorClient) GetDaemonSets() ([]appsV1.DaemonSet, error) {
	daemonSets := make([]appsV1.DaemonSet, 0)
	for i := 1; i <= 3; i++ {
		daemonSet := appsV1.DaemonSet{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-daemonset-%d", i),
				CreationTimestamp: m.creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-daemonset-%d", i)),
				GenerateName: "",
			},
			Spec: appsV1.DaemonSetSpec{
				UpdateStrategy: appsV1.DaemonSetUpdateStrategy{
					Type: appsV1.RollingUpdateDaemonSetStrategyType,
				},
			},
		}

		if i == 3 {
			daemonSet.TypeMeta.Kind = "some-specified-kind"
			daemonSet.ObjectMeta.GenerateName = "some-specified-generation"
		}

		daemonSets = append(daemonSets, daemonSet)
	}

	return daemonSets, nil
}

func (m MockBenchmarkAPICollectorClient) GetDeployments() ([]appsV1.Deployment, error) {
	deployments := make([]appsV1.Deployment, 0)
	for i := 1; i <= 3; i++ {
		deployment := appsV1.Deployment{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-deployment-%d", i),
				CreationTimestamp: m.creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-deployment-%d", i)),
				GenerateName: "",
			},
			Spec: appsV1.DeploymentSpec{
				Strategy: appsV1.DeploymentStrategy{
					Type: appsV1.RollingUpdateDeploymentStrategyType,
				},
				Replicas: &m.replicas,
			},
		}

		if i == 3 {
			deployment.TypeMeta.Kind = "some-specified-kind"
			deployment.ObjectMeta.GenerateName = "some-specified-generation"
		}

		deployments = append(deployments, deployment)
	}

	return deployments, nil
}

func (m MockBenchmarkAPICollectorClient) GetIngresses() ([]extensionsV1B1.Ingress, error) {
	ingresses := make([]extensionsV1B1.Ingress, 0)
	for i := 1; i <= 3; i++ {
		ingress := extensionsV1B1.Ingress{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-ingress-%d", i),
				CreationTimestamp: m.creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-ingress-%d", i)),
				GenerateName: "",
			},
			Status: extensionsV1B1.IngressStatus{
				LoadBalancer: coreV1.LoadBalancerStatus{
					Ingress: []coreV1.LoadBalancerIngress{
						{IP: "34.100.200.15"},
						{Hostname: "64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com"},
					},
				},
			},
		}

		if i == 2 {
			ingress.Spec.Backend = &extensionsV1B1.IngressBackend{ServiceName: "test-service"}
		}

		if i == 3 {
			ingress.TypeMeta.Kind = "some-specified-kind"
			ingress.ObjectMeta.GenerateName = "some-specified-generation"
			ingress.Spec.Rules = []extensionsV1B1.IngressRule{
				{
					Host: "host-1",
					IngressRuleValue: extensionsV1B1.IngressRuleValue{
						HTTP: &extensionsV1B1.HTTPIngressRuleValue{
							Paths: []extensionsV1B1.HTTPIngressPath{
								{Path: "host-1-path-1", Backend: extensionsV1B1.IngressBackend{ServiceName: "test-service-1"}},
								{Path: "host-1-path-2", Backend: extensionsV1B1.IngressBackend{ServiceName: "test-service-2"}},
							},
						},
					},
				},
				{
					Host: "host-2",
					IngressRuleValue: extensionsV1B1.IngressRuleValue{
						HTTP: &extensionsV1B1.HTTPIngressRuleValue{
							Paths: []extensionsV1B1.HTTPIngressPath{
								{Path: "host-2-path-1", Backend: extensionsV1B1.IngressBackend{ServiceName: "test-service-3"}},
							},
						},
					},
				},
			}
		}

		ingresses = append(ingresses, ingress)
	}

	return ingresses, nil
}

func (m MockBenchmarkAPICollectorClient) GetJobs() ([]batchV1.Job, error) {
	jobs := make([]batchV1.Job, 0)
	for i := 1; i <= 3; i++ {
		job := batchV1.Job{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-job-%d", i),
				CreationTimestamp: m.creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-job-%d", i)),
				GenerateName: "",
			},
			Spec: batchV1.JobSpec{
				Parallelism:  &m.parralelism,
				BackoffLimit: &m.backoffLimit,
			},
		}

		if i == 3 {
			job.TypeMeta.Kind = "some-specified-kind"
			job.ObjectMeta.GenerateName = "some-specified-generation"
		}

		jobs = append(jobs, job)
	}

	return jobs, nil
}

func (m MockBenchmarkAPICollectorClient) GetNodes() ([]coreV1.Node, error) {
	nodes := make([]coreV1.Node, 0)
	for i := 1; i <= 3; i++ {
		node := coreV1.Node{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-node-%d", i),
				CreationTimestamp: m.creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-node-%d", i)),
				GenerateName: "",
			},
			Status: coreV1.NodeStatus{
				Phase: coreV1.NodeRunning,
				NodeInfo: coreV1.NodeSystemInfo{
					MachineID:     fmt.Sprintf("test-machine-id-%d", i),
					KernelVersion: "4.19.0",
					Architecture:  "x86_64",
				},
				DaemonEndpoints: coreV1.NodeDaemonEndpoints{KubeletEndpoint: coreV1.DaemonEndpoint{Port: 5000}},
			},
		}

		if i == 1 {
			node.Status.Addresses = []coreV1.NodeAddress{
				{Type: coreV1.NodeInternalIP, Address: "10.20.01.01"},
			}
		}

		if i == 2 {
			node.TypeMeta.Kind = "some-specified-kind"
			node.ObjectMeta.GenerateName = "some-specified-generation"

			node.Status.Addresses = []coreV1.NodeAddress{
				{Type: coreV1.NodeInternalIP, Address: "10.20.01.01"},
				{Type: coreV1.NodeExternalIP, Address: "10.20.01.02"},
			}
		}

		if i == 3 {
			node.TypeMeta.Kind = "some-specified-kind"
			node.ObjectMeta.GenerateName = "some-specified-generation"
			node.Spec.ProviderID = "aws:///us-east-1b/i-024b28584ed2e6321"
			node.Status.Addresses = []coreV1.NodeAddress{
				{Type: coreV1.NodeInternalIP, Address: "10.20.01.01"},
				{Type: coreV1.NodeExternalIP, Address: "10.20.01.02"},
				{Type: coreV1.NodeInternalDNS, Address: "cluster.internal.dns.test-node-3"},
				{Type: coreV1.NodeExternalDNS, Address: "my-organization.test-node-3"},
			}
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (m MockBenchmarkAPICollectorClient) GetPersistentVolumes() ([]coreV1.PersistentVolume, error) {
	persistentVolumes := make([]coreV1.PersistentVolume, 0)
	for i := 1; i <= 3; i++ {
		persistentVolume := coreV1.PersistentVolume{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-persistent-volume-%d", i),
				CreationTimestamp: m.creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-persistent-volume-%d", i)),
				GenerateName: "",
			},
			Spec: coreV1.PersistentVolumeSpec{
				StorageClassName: "Storage-Class-Name",
			},
			Status: coreV1.PersistentVolumeStatus{
				Phase:   coreV1.VolumeAvailable,
				Message: "Volume is available for use",
			},
		}

		if i == 1 {
			persistentVolume.Spec.PersistentVolumeSource = coreV1.PersistentVolumeSource{
				AWSElasticBlockStore: &m.awsElasticBlockStore,
			}
		}

		if i == 2 {
			persistentVolume.Spec.PersistentVolumeSource = coreV1.PersistentVolumeSource{
				GCEPersistentDisk: &m.gcePersistentDisk,
			}
		}

		if i == 3 {
			persistentVolume.Spec.PersistentVolumeSource = coreV1.PersistentVolumeSource{
				HostPath: &m.hostPath,
			}
			persistentVolume.TypeMeta.Kind = "some-specified-kind"
			persistentVolume.ObjectMeta.GenerateName = "some-specified-generation"
		}

		persistentVolumes = append(persistentVolumes, persistentVolume)
	}

	return persistentVolumes, nil
}

func (m MockBenchmarkAPICollectorClient) GetPods() ([]coreV1.Pod, error) {
	pods := make([]coreV1.Pod, 0)
	for i := 1; i <= 3; i++ {
		pod := coreV1.Pod{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-pod-%d", i),
				CreationTimestamp: m.creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-pod-%d", i)),
				GenerateName: "",
			},
			Status: coreV1.PodStatus{
				Phase:     coreV1.PodRunning,
				PodIP:     "10.0.0.1",
				StartTime: &m.creationTime,
			},
			Spec: coreV1.PodSpec{
				RestartPolicy: coreV1.RestartPolicyAlways,
				NodeName:      "test-node",
			},
		}

		if i == 2 {
			pod.Spec.HostNetwork = true
			pod.Status.PodIP = "10.0.0.2"
			pod.Spec.ServiceAccountName = "some-service-account-name"
			pod.Status.Message = "some longer readable message for the phase"
			pod.Status.Reason = "some-short-reason"
			pod.Status.NominatedNodeName = "some-nominated-node-name"
			pod.Status.QOSClass = "some-qos-class"
			pod.TypeMeta.Kind = "some-specified-kind"
			pod.ObjectMeta.GenerateName = "some-specified-generation"
		}

		if i == 3 {
			pod.OwnerReferences = []v1.OwnerReference{
				{Kind: "DaemonSet", Name: "daemonset-w"},
				{Kind: "Deployment", Name: "deployment-x"},
				{Kind: "ReplicaSet", Name: "replicaset-y"},
				{Kind: "StatefulSet", Name: "statefulset-z"},
			}
		}

		if i == 4 {

		}

		if i == 5 {

		}

		if i == 6 {

		}

		if i == 7 {

		}

		if i == 8 {

		}

		if i == 9 {

		}

		pods = append(pods, pod)
	}

	return pods, nil
}

func (m MockBenchmarkAPICollectorClient) GetReplicaSets() ([]appsV1.ReplicaSet, error) {
	replicaSets := make([]appsV1.ReplicaSet, 0)
	for i := 1; i <= 3; i++ {
		replicaSet := appsV1.ReplicaSet{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-replicaset-%d", i),
				CreationTimestamp: m.creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-replicaset-%d", i)),
				GenerateName: "",
			},
			Spec: appsV1.ReplicaSetSpec{
				Replicas: &m.replicas,
			},
		}

		if i > 1 {
			replicaSet.TypeMeta.Kind = "some-specified-kind"
			replicaSet.ObjectMeta.GenerateName = "some-specified-generation"
		}

		if i == 3 {
			replicaSet.OwnerReferences = []v1.OwnerReference{
				{Kind: "Deployment", Name: "test-deployment-3"},
			}
		}

		replicaSets = append(replicaSets, replicaSet)
	}

	return replicaSets, nil
}

func (m MockBenchmarkAPICollectorClient) GetServices() ([]coreV1.Service, error) {
	services := make([]coreV1.Service, 0)
	for i := 1; i <= 5; i++ {

		service := coreV1.Service{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-service-%d", i),
				CreationTimestamp: m.creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-service-%d", i)),
				GenerateName: "",
			},
			Spec: coreV1.ServiceSpec{
				Ports: []coreV1.ServicePort{
					{Name: fmt.Sprintf("test-service-port-%d", i), Port: int32(80 + i), TargetPort: intstr.FromInt(8080 + i)},
				},
				Type: coreV1.ServiceTypeClusterIP,
			},
		}

		if i == 2 {
			service.Spec.Type = coreV1.ServiceTypeNodePort
			service.Spec.Ports = []coreV1.ServicePort{
				{
					Name:       fmt.Sprintf("test-service-node-port-%d", i),
					Port:       int32(80 + i),
					TargetPort: intstr.FromInt(8080 + i),
					NodePort:   int32(10200 + i),
				},
			}
			service.Spec.ClusterIP = "10.100.200.20"
		}
		if i == 3 {
			service.Spec.Type = coreV1.ServiceTypeClusterIP
			service.Spec.ExternalIPs = []string{"34.100.200.12", "34.100.200.13"}
			service.Spec.ClusterIP = "10.100.200.21"
		}

		if i == 4 {
			service.Spec.Type = coreV1.ServiceTypeClusterIP
			service.Spec.ClusterIP = "10.100.200.22"
		}

		if i == 5 {
			service.Spec.Type = coreV1.ServiceTypeLoadBalancer
			service.Spec.Ports = []coreV1.ServicePort{
				{
					Name:       fmt.Sprintf("test-service-port-%d", i),
					Port:       int32(80 + i),
					TargetPort: intstr.FromInt(8080 + i),
				},
				{
					Name:       fmt.Sprintf("test-service-node-port-%d", i),
					Port:       int32(80 + i),
					TargetPort: intstr.FromInt(8080 + i),
					NodePort:   int32(10200 + i),
				},
			}
			service.Status.LoadBalancer = coreV1.LoadBalancerStatus{
				Ingress: []coreV1.LoadBalancerIngress{
					{IP: "34.100.200.15"},
					{Hostname: "64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com"},
				},
			}
			service.Spec.LoadBalancerIP = "10.100.200.23"
		}

		services = append(services, service)
	}

	return services, nil
}

func (m MockBenchmarkAPICollectorClient) GetEndpoints() ([]coreV1.Endpoints, error) {
	endpoints := make([]coreV1.Endpoints, 0)
	// endpoints for test case 1
	endpoints = append(endpoints, coreV1.Endpoints{
		TypeMeta: v1.TypeMeta{
			Kind: "",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:              "test-service-1",
			CreationTimestamp: m.creationTime,
			Namespace:         "test-namespace",
			Labels: map[string]string{
				"test": "label",
			},
			UID:          types.UID("test-service-1"),
			GenerateName: "",
		},
		Subsets: []coreV1.EndpointSubset{
			{
				Addresses: []coreV1.EndpointAddress{
					{IP: "10.100.200.1", TargetRef: &coreV1.ObjectReference{Kind: "Pod", Name: "some-pod-name"}},
				},
				Ports: []coreV1.EndpointPort{
					{Name: "", Port: int32(81)},
				},
			},
		},
	})

	// endpoints for test case 5
	endpoints = append(endpoints, coreV1.Endpoints{
		TypeMeta: v1.TypeMeta{
			Kind: "",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:              "test-service-5",
			CreationTimestamp: m.creationTime,
			Namespace:         "test-namespace",
			Labels: map[string]string{
				"test": "label",
			},
			UID:          types.UID("test-service-5"),
			GenerateName: "",
		},
		Subsets: []coreV1.EndpointSubset{
			{
				Addresses: []coreV1.EndpointAddress{
					{IP: "10.100.200.2", TargetRef: &coreV1.ObjectReference{Kind: "Pod", Name: "some-pod-name"}},
				},
				Ports: []coreV1.EndpointPort{
					{Name: "Endpoint Port", Port: int32(85)},
					{Name: "Endpoint NodePort", Port: int32(10205)},
				},
			},
		},
	})

	return endpoints, nil
}

func (m MockBenchmarkAPICollectorClient) GetStatefulSets() ([]appsV1.StatefulSet, error) {
	statefulSets := make([]appsV1.StatefulSet, 0)
	for i := 1; i <= 3; i++ {
		statefulSet := appsV1.StatefulSet{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-statefulset-%d", i),
				CreationTimestamp: m.creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-statefulset-%d", i)),
				GenerateName: "",
			},
			Spec: appsV1.StatefulSetSpec{
				UpdateStrategy: appsV1.StatefulSetUpdateStrategy{
					Type: appsV1.RollingUpdateStatefulSetStrategyType,
				},
				Replicas:            &m.replicas,
				PodManagementPolicy: appsV1.OrderedReadyPodManagement,
				ServiceName:         "statefulset-service-name",
			},
		}

		if i == 3 {
			statefulSet.TypeMeta.Kind = "some-specified-kind"
			statefulSet.ObjectMeta.GenerateName = "some-specified-generation"
		}

		statefulSets = append(statefulSets, statefulSet)
	}

	return statefulSets, nil
}
