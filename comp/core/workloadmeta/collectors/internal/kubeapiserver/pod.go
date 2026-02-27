// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"

	jsoniter "github.com/json-iterator/go"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/framer"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/config"
	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/gpu"
	kubeutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// This reflector uses a MinimalPod struct that contains only the fields we
// actually use from the Kubernetes pod. This is for memory optimization.
//
// The reflector uses the MinimalPod struct with the Kubernetes REST client
// instead of the typed client.
//
// The typed client always unmarshalls the whole pod object. This includes many
// fields that we don't use. During startup, in large clusters, this reflector
// can sync lots of pods causing a large memory spike.
//
// This approach of using a minimal pod allows us unmarshal directly into
// MinimalPod, avoiding allocations of unused fields.
//
// We can only use this approach when protobuf is disabled. When protobuf is
// enabled, we fall back to the typed client approach.

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// MinimalPod contains only the fields we actually use from a Pod. This is used
// to reduce memory allocations during JSON unmarshalling by avoiding allocation
// of unused fields.
type MinimalPod struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MinimalPodSpec   `json:"spec,omitempty"`
	Status MinimalPodStatus `json:"status,omitempty"`
}

// MinimalPodSpec contains only the pod spec fields we need
type MinimalPodSpec struct {
	Containers        []MinimalContainer `json:"containers"`
	Volumes           []MinimalVolume    `json:"volumes,omitempty"`
	RuntimeClassName  *string            `json:"runtimeClassName,omitempty"`
	PriorityClassName string             `json:"priorityClassName,omitempty"`
}

// MinimalContainer contains only the container fields we need
type MinimalContainer struct {
	Name      string                      `json:"name"`
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// MinimalVolume contains only the volume fields we need
type MinimalVolume struct {
	PersistentVolumeClaim *corev1.PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty"`
}

// MinimalPodStatus contains only the pod status fields we need
type MinimalPodStatus struct {
	Phase      corev1.PodPhase       `json:"phase,omitempty"`
	Conditions []corev1.PodCondition `json:"conditions,omitempty"`
	PodIP      string                `json:"podIP,omitempty"`
	QOSClass   corev1.PodQOSClass    `json:"qosClass,omitempty"`
}

// DeepCopyObject deep copies (required to implement kubernetes runtime.Object
// interface)
// Note: we can't use the deepcopy library like we're using in some parts of
// workloadmeta because it doesn't copy unexported fields. This means it doesn't
// work with corev1.ResourceRequirements.
func (p *MinimalPod) DeepCopyObject() runtime.Object {
	if p == nil {
		return nil
	}

	out := &MinimalPod{}

	// TypeMeta
	out.TypeMeta = p.TypeMeta

	// ObjectMeta
	p.ObjectMeta.DeepCopyInto(&out.ObjectMeta)

	// Spec
	if p.Spec.Containers != nil {
		out.Spec.Containers = make([]MinimalContainer, len(p.Spec.Containers))
		for i := range p.Spec.Containers {
			out.Spec.Containers[i].Name = p.Spec.Containers[i].Name

			resIn := &p.Spec.Containers[i].Resources
			resOut := &out.Spec.Containers[i].Resources

			if resIn.Limits != nil {
				resOut.Limits = make(corev1.ResourceList, len(resIn.Limits))
				for k, v := range resIn.Limits {
					resOut.Limits[k] = v.DeepCopy()
				}
			}
			if resIn.Requests != nil {
				resOut.Requests = make(corev1.ResourceList, len(resIn.Requests))
				for k, v := range resIn.Requests {
					resOut.Requests[k] = v.DeepCopy()
				}
			}
			if resIn.Claims != nil {
				resOut.Claims = make([]corev1.ResourceClaim, len(resIn.Claims))
				for j := range resIn.Claims {
					resIn.Claims[j].DeepCopyInto(&resOut.Claims[j])
				}
			}
		}
	}

	if p.Spec.Volumes != nil {
		out.Spec.Volumes = make([]MinimalVolume, len(p.Spec.Volumes))
		for i := range p.Spec.Volumes {
			if p.Spec.Volumes[i].PersistentVolumeClaim != nil {
				out.Spec.Volumes[i].PersistentVolumeClaim = p.Spec.Volumes[i].PersistentVolumeClaim.DeepCopy()
			}
		}
	}

	out.Spec.PriorityClassName = p.Spec.PriorityClassName

	if p.Spec.RuntimeClassName != nil {
		out.Spec.RuntimeClassName = new(string)
		*out.Spec.RuntimeClassName = *p.Spec.RuntimeClassName
	}

	// Status
	out.Status.Phase = p.Status.Phase

	if p.Status.Conditions != nil {
		out.Status.Conditions = make([]corev1.PodCondition, len(p.Status.Conditions))
		for i := range p.Status.Conditions {
			p.Status.Conditions[i].DeepCopyInto(&out.Status.Conditions[i])
		}
	}

	out.Status.PodIP = p.Status.PodIP
	out.Status.QOSClass = p.Status.QOSClass

	return out
}

// MinimalPodList is a list of MinimalPods
type MinimalPodList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MinimalPod `json:"items"`
}

// DeepCopyObject deep copies (required to implement kubernetes runtime.Object interface)
func (p *MinimalPodList) DeepCopyObject() runtime.Object {
	if p == nil {
		return nil
	}

	out := &MinimalPodList{}
	out.TypeMeta = p.TypeMeta
	p.ListMeta.DeepCopyInto(&out.ListMeta)

	if p.Items != nil {
		out.Items = make([]MinimalPod, len(p.Items))
		for i := range p.Items {
			out.Items[i] = *p.Items[i].DeepCopyObject().(*MinimalPod)
		}
	}

	return out
}

type minimalPodDecoder struct {
	reader  io.ReadCloser
	decoder *jsoniter.Decoder
}

func newMinimalPodDecoder(body io.ReadCloser) *minimalPodDecoder {
	return &minimalPodDecoder{
		reader:  body,
		decoder: json.NewDecoder(body),
	}
}

// Decode implements watch.Decoder interface
func (d *minimalPodDecoder) Decode() (watch.EventType, runtime.Object, error) {
	var event metav1.WatchEvent
	if err := d.decoder.Decode(&event); err != nil {
		return "", nil, err
	}

	eventType := watch.EventType(event.Type)

	switch eventType {
	case watch.Added, watch.Modified, watch.Deleted, watch.Error, watch.Bookmark:
	default:
		return "", nil, fmt.Errorf("got invalid watch event type: %v", event.Type)
	}

	if eventType == watch.Error {
		var status metav1.Status
		if err := json.Unmarshal(event.Object.Raw, &status); err != nil {
			return "", nil, fmt.Errorf("unable to decode watch event status: %v", err)
		}
		return eventType, &status, nil
	}

	var pod MinimalPod
	if err := json.Unmarshal(event.Object.Raw, &pod); err != nil {
		return "", nil, fmt.Errorf("unable to decode pod in watch event: %v", err)
	}

	return eventType, &pod, nil
}

// Close closes the underlying reader
func (d *minimalPodDecoder) Close() {
	d.reader.Close()
}

type minimalPodParser struct {
	annotationsFilter []*regexp.Regexp
}

// Parse parses a MinimalPod object into a workloadmeta Pod
// This is basically a copy of the full pod parser defined in the
// kubernetesresourceparsers package
func (p minimalPodParser) Parse(obj interface{}) workloadmeta.Entity {
	pod := obj.(*MinimalPod)
	owners := make([]workloadmeta.KubernetesPodOwner, 0, len(pod.OwnerReferences))
	for _, o := range pod.OwnerReferences {
		owners = append(owners, workloadmeta.KubernetesPodOwner{
			Kind: o.Kind,
			Name: o.Name,
			ID:   string(o.UID),
		})
	}

	var ready bool
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			if condition.Status == corev1.ConditionTrue {
				ready = true
			}
			break
		}
	}

	var pvcNames []string
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			pvcNames = append(pvcNames, volume.PersistentVolumeClaim.ClaimName)
		}
	}

	var rtcName string
	if pod.Spec.RuntimeClassName != nil {
		rtcName = *pod.Spec.RuntimeClassName
	}

	var gpuVendorList []string
	uniqueGPUVendor := make(map[string]struct{})
	for _, container := range pod.Spec.Containers {
		for resourceName := range container.Resources.Limits {
			gpuName, found := gpu.ExtractSimpleGPUName(gpu.ResourceGPU(resourceName))
			if found {
				uniqueGPUVendor[gpuName] = struct{}{}
			}
		}
	}
	for gpuVendor := range uniqueGPUVendor {
		gpuVendorList = append(gpuVendorList, gpuVendor)
	}

	containersList := make([]workloadmeta.OrchestratorContainer, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		c := workloadmeta.OrchestratorContainer{
			Name: container.Name,
		}
		if cpuReq, found := container.Resources.Requests[corev1.ResourceCPU]; found {
			c.Resources.CPURequest = kubeutil.FormatCPURequests(cpuReq)
		}
		if memoryReq, found := container.Resources.Requests[corev1.ResourceMemory]; found {
			c.Resources.MemoryRequest = kubeutil.FormatMemoryRequests(memoryReq)
		}
		if cpuLimit, found := container.Resources.Limits[corev1.ResourceCPU]; found {
			c.Resources.CPULimit = kubeutil.FormatCPURequests(cpuLimit)
		}
		if memoryLimit, found := container.Resources.Limits[corev1.ResourceMemory]; found {
			c.Resources.MemoryLimit = kubeutil.FormatMemoryRequests(memoryLimit)
		}
		containersList = append(containersList, c)
	}

	return &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   string(pod.UID),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Annotations: kubernetesresourceparsers.FilterMapStringKey(pod.Annotations, p.annotationsFilter),
			Labels:      pod.Labels,
		},
		Phase:                      string(pod.Status.Phase),
		Owners:                     owners,
		PersistentVolumeClaimNames: pvcNames,
		Ready:                      ready,
		IP:                         pod.Status.PodIP,
		PriorityClass:              pod.Spec.PriorityClassName,
		QOSClass:                   string(pod.Status.QOSClass),
		RuntimeClass:               rtcName,
		GPUVendorList:              gpuVendorList,
		Containers:                 containersList,
		CreationTimestamp:           pod.CreationTimestamp.Time,
	}
}

func newPodStore(ctx context.Context, wlm workloadmeta.Component, config config.Reader, client kubernetes.Interface) (*cache.Reflector, *reflectorStore) {
	// The REST client approach doesn't work with protobuf, so fallback to typed
	// client.
	if config.GetBool("kubernetes_apiserver_use_protobuf") {
		return newPodStoreWithTypedClient(ctx, wlm, config, client)
	}
	return newPodStoreWithRestClient(wlm, config, client)
}

func newPodStoreWithRestClient(wlm workloadmeta.Component, config config.Reader, client kubernetes.Interface) (*cache.Reflector, *reflectorStore) {
	restClient := client.CoreV1().RESTClient()

	listFunc := func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
		var podList MinimalPodList
		err := restClient.Get().
			Namespace(metav1.NamespaceAll).
			Resource("pods").
			VersionedParams(&options, metav1.ParameterCodec).
			Do(ctx).
			Into(&podList)
		return &podList, err
	}

	watchFunc := func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
		options.Watch = true

		resp, err := restClient.Get().
			Namespace(metav1.NamespaceAll).
			Resource("pods").
			VersionedParams(&options, metav1.ParameterCodec).
			Stream(ctx)
		if err != nil {
			return nil, err
		}

		framedReader := framer.NewJSONFramedReader(resp) // Needed to decode individual objects
		decoder := newMinimalPodDecoder(framedReader)
		errorReporter := errors.NewClientErrorReporter(http.StatusInternalServerError, "GET", "PodWatchDecoding")

		return watch.NewStreamWatcher(decoder, errorReporter), nil
	}

	podListerWatcher := &cache.ListWatch{
		ListWithContextFunc:  listFunc,
		WatchFuncWithContext: watchFunc,
	}

	podStore := newPodReflectorStoreWithMinimalPodParser(wlm, config)
	podReflector := cache.NewNamedReflector(
		componentName,
		podListerWatcher,
		&MinimalPod{},
		podStore,
		noResync,
	)
	log.Debug("pod reflector enabled")
	return podReflector, podStore
}

func newPodStoreWithTypedClient(ctx context.Context, wlm workloadmeta.Component, config config.Reader, client kubernetes.Interface) (*cache.Reflector, *reflectorStore) {
	podListerWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Pods(metav1.NamespaceAll).Watch(ctx, options)
		},
	}

	podStore := newPodReflectorStoreWithFullPodParser(wlm, config)
	podReflector := cache.NewNamedReflector(
		componentName,
		podListerWatcher,
		&corev1.Pod{},
		podStore,
		noResync,
	)
	log.Debug("pod reflector enabled")
	return podReflector, podStore
}

func newPodReflectorStoreWithMinimalPodParser(wlmetaStore workloadmeta.Component, config config.Reader) *reflectorStore {
	annotationsExclude := config.GetStringSlice("cluster_agent.kubernetes_resources_collection.pod_annotations_exclude")
	filters, err := kubernetesresourceparsers.ParseFilters(annotationsExclude)
	if err != nil {
		log.Errorf("unable to parse all pod_annotations_exclude: %v", err)
	}

	return &reflectorStore{
		wlmetaStore: wlmetaStore,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      minimalPodParser{annotationsFilter: filters},
	}
}

func newPodReflectorStoreWithFullPodParser(wlmetaStore workloadmeta.Component, config config.Reader) *reflectorStore {
	annotationsExclude := config.GetStringSlice("cluster_agent.kubernetes_resources_collection.pod_annotations_exclude")
	parser, err := kubernetesresourceparsers.NewPodParser(annotationsExclude)
	if err != nil {
		_ = log.Errorf("unable to parse all pod_annotations_exclude: %v, err:", err)
		parser, _ = kubernetesresourceparsers.NewPodParser(nil)
	}

	return &reflectorStore{
		wlmetaStore: wlmetaStore,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      parser,
	}
}
