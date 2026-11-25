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
	"slices"
	"time"

	jsoniter "github.com/json-iterator/go"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/framer"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/config"
	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/gpu"
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
	NodeName          string             `json:"nodeName,omitempty"`
	InitContainers    []MinimalContainer `json:"initContainers,omitempty"`
	Containers        []MinimalContainer `json:"containers"`
	Volumes           []MinimalVolume    `json:"volumes,omitempty"`
	RuntimeClassName  *string            `json:"runtimeClassName,omitempty"`
	PriorityClassName string             `json:"priorityClassName,omitempty"`
}

// MinimalContainer contains only the container fields we need
type MinimalContainer struct {
	Name          string                         `json:"name"`
	Image         string                         `json:"image,omitempty"`
	Resources     corev1.ResourceRequirements    `json:"resources,omitempty"`
	ResizePolicy  []corev1.ContainerResizePolicy `json:"resizePolicy,omitempty"`
	RestartPolicy *corev1.ContainerRestartPolicy `json:"restartPolicy,omitempty"`
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
	out.Spec.InitContainers = deepCopyMinimalContainers(p.Spec.InitContainers)
	out.Spec.Containers = deepCopyMinimalContainers(p.Spec.Containers)

	if p.Spec.Volumes != nil {
		out.Spec.Volumes = make([]MinimalVolume, len(p.Spec.Volumes))
		for i := range p.Spec.Volumes {
			if p.Spec.Volumes[i].PersistentVolumeClaim != nil {
				out.Spec.Volumes[i].PersistentVolumeClaim = p.Spec.Volumes[i].PersistentVolumeClaim.DeepCopy()
			}
		}
	}

	out.Spec.NodeName = p.Spec.NodeName
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

func deepCopyMinimalContainers(in []MinimalContainer) []MinimalContainer {
	if in == nil {
		return nil
	}
	out := make([]MinimalContainer, len(in))
	for i := range in {
		out[i].Name = in[i].Name
		out[i].Image = in[i].Image
		in[i].Resources.DeepCopyInto(&out[i].Resources)
		out[i].ResizePolicy = slices.Clone(in[i].ResizePolicy)
		if in[i].RestartPolicy != nil {
			rp := *in[i].RestartPolicy
			out[i].RestartPolicy = &rp
		}
	}
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
		gv, _ := schema.ParseGroupVersion(o.APIVersion)
		owners = append(owners, workloadmeta.KubernetesPodOwner{
			Kind:  o.Kind,
			Name:  o.Name,
			ID:    string(o.UID),
			Group: gv.Group,
		})
	}

	var ready bool
	var readyTimestamp *time.Time
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			if condition.Status == corev1.ConditionTrue {
				ready = true
				// Leave readyTimestamp nil when the transition time is unknown (omitted/zero),
				// so the warmup gate treats the readiness time as unknown rather than year 1.
				if !condition.LastTransitionTime.IsZero() {
					t := condition.LastTransitionTime.Time
					readyTimestamp = &t
				}
			}
			break
		}
	}

	// DeletionTimestamp is set once the pod is terminating; cluster-agent workload autoscaling
	// uses it to exclude terminating pods from its replica calculation (matching the HPA).
	var deletionTimestamp *time.Time
	if pod.DeletionTimestamp != nil {
		t := pod.DeletionTimestamp.Time
		deletionTimestamp = &t
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

	containersList := parseMinimalContainers(pod.Spec.Containers)
	var initContainersList []workloadmeta.OrchestratorContainer
	if len(pod.Spec.InitContainers) > 0 {
		initContainersList = parseMinimalContainers(pod.Spec.InitContainers)
	}

	conditions := kubernetesresourceparsers.ResizeConditionsFromPodConditions(pod.Status.Conditions)

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
		InitContainers:             initContainersList,
		Containers:                 containersList,
		Conditions:                 conditions,
		CreationTimestamp:          pod.CreationTimestamp.Time,
		ReadyTimestamp:             readyTimestamp,
		DeletionTimestamp:          deletionTimestamp,
		NodeName:                   pod.Spec.NodeName,
	}
}

func parseMinimalContainers(containers []MinimalContainer) []workloadmeta.OrchestratorContainer {
	list := make([]workloadmeta.OrchestratorContainer, 0, len(containers))
	for i := range containers {
		c := &containers[i]
		list = append(list, kubernetesresourceparsers.OrchestratorContainerFromSpec(c.Name, c.Image, c.Resources, c.ResizePolicy, c.RestartPolicy))
	}
	return list
}

func newPodStore(wlm workloadmeta.Component, config config.Reader, client kubernetes.Interface) (*cache.Reflector, *reflectorStore) {
	// The REST client approach doesn't work with protobuf, so fallback to typed
	// client.
	if config.GetBool("kubernetes_apiserver_use_protobuf") {
		return newPodStoreWithTypedClient(wlm, config, client)
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

func newPodStoreWithTypedClient(wlm workloadmeta.Component, config config.Reader, client kubernetes.Interface) (*cache.Reflector, *reflectorStore) {
	podListerWatcher := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, options)
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
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
