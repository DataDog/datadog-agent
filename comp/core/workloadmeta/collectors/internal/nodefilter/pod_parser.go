// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build nodefilter

package nodefilter

import (
	stdErrors "errors"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/internal/third_party/golang/expansion"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	pkgcontainersimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
	"github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// dockerImageIDPrefix is stripped from ImageID, mirroring the kubelet
	// API's own convention for reporting pullable image references.
	dockerImageIDPrefix = "docker-pullable://"

	// mirrorPodAnnotation is set by the kubelet, on the mirror pod it creates
	// in the API server for each static pod, to the static pod's UID.
	mirrorPodAnnotation = "kubernetes.io/config.mirror"
)

// parsePod builds the workloadmeta events for a single pod: one
// KubernetesPod entity, plus one Container entity per (init/ephemeral)
// container that the runtime has already created. Only the fields consumed
// by the tagger (comp/core/tagger/collectors) are populated. Ephemeral
// containers are only parsed when collectEphemeralContainers is set, mirroring
// the kubelet collector's own include_ephemeral_containers gate.
func parsePod(pod *corev1.Pod, collectEphemeralContainers bool) []workloadmeta.CollectorEvent {
	podID := workloadmeta.EntityID{
		Kind: workloadmeta.KindKubernetesPod,
		ID:   podUID(pod),
	}

	initContainers, initContainerEvents := parsePodContainers(pod.Spec.InitContainers, pod.Status.InitContainerStatuses, pod.Namespace, &podID)
	podContainers, containerEvents := parsePodContainers(pod.Spec.Containers, pod.Status.ContainerStatuses, pod.Namespace, &podID)

	events := make([]workloadmeta.CollectorEvent, 0, len(initContainerEvents)+len(containerEvents)+1)
	events = append(events, initContainerEvents...)
	events = append(events, containerEvents...)

	var ephemeralContainers []workloadmeta.OrchestratorContainer
	if collectEphemeralContainers {
		var ephemeralContainerEvents []workloadmeta.CollectorEvent
		ephemeralContainers, ephemeralContainerEvents = parsePodContainers(
			ephemeralContainerSpecs(pod.Spec.EphemeralContainers),
			pod.Status.EphemeralContainerStatuses,
			pod.Namespace,
			&podID,
		)
		events = append(events, ephemeralContainerEvents...)
	}

	mirrorsStaticPod := staticPodUID(pod) != ""
	owners := make([]workloadmeta.KubernetesPodOwner, 0, len(pod.OwnerReferences))
	for _, o := range pod.OwnerReferences {
		gv, _ := schema.ParseGroupVersion(o.APIVersion)
		// The kubelet makes a mirror pod owned by its node, so that the mirror
		// pod is garbage-collected with the node. The static pod it stands for,
		// which the kubelet collector reports, has no such owner: leave it out,
		// or the pod would get kube_ownerref_* tags for the node that the
		// kubelet collector's doesn't.
		if mirrorsStaticPod && gv.Group == "" && o.Kind == "Node" {
			continue
		}
		owners = append(owners, workloadmeta.KubernetesPodOwner{
			Kind:       o.Kind,
			Name:       o.Name,
			ID:         string(o.UID),
			Group:      gv.Group,
			Controller: o.Controller,
		})
	}

	var pvcNames []string
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			pvcNames = append(pvcNames, volume.PersistentVolumeClaim.ClaimName)
		}
	}

	var runtimeClass string
	if pod.Spec.RuntimeClassName != nil {
		runtimeClass = *pod.Spec.RuntimeClassName
	}

	var ready bool
	var readyTimestamp *time.Time
	for _, condition := range pod.Status.Conditions {
		if condition.Type != corev1.PodReady {
			continue
		}
		if condition.Status == corev1.ConditionTrue {
			ready = true
			// Leave readyTimestamp nil when the transition time is unknown
			// (omitted/zero), like the kubeapiserver collector's pod parser,
			// so consumers treat the readiness time as unknown rather than
			// year 1.
			if !condition.LastTransitionTime.IsZero() {
				t := condition.LastTransitionTime.Time
				readyTimestamp = &t
			}
		}
		break
	}

	var deletionTimestamp *time.Time
	if pod.DeletionTimestamp != nil {
		t := pod.DeletionTimestamp.Time
		deletionTimestamp = &t
	}

	entity := &workloadmeta.KubernetesPod{
		EntityID: podID,
		EntityMeta: workloadmeta.EntityMeta{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Annotations: pod.Annotations,
			Labels:      pod.Labels,
		},
		Owners:                     owners,
		PersistentVolumeClaimNames: pvcNames,
		InitContainers:             initContainers,
		Containers:                 podContainers,
		EphemeralContainers:        ephemeralContainers,
		Ready:                      ready,
		Phase:                      string(pod.Status.Phase),
		IP:                         pod.Status.PodIP,
		PriorityClass:              pod.Spec.PriorityClassName,
		QOSClass:                   string(pod.Status.QOSClass),
		GPUVendorList:              gpuVendorsFromContainers(pod.Spec.InitContainers, pod.Spec.Containers),
		RuntimeClass:               runtimeClass,
		NodeName:                   pod.Spec.NodeName,
		HostNetwork:                pod.Spec.HostNetwork,
		HostIP:                     pod.Status.HostIP,
		CreationTimestamp:          pod.CreationTimestamp.Time,
		DeletionTimestamp:          deletionTimestamp,
		ReadyTimestamp:             readyTimestamp,
	}

	events = append(events, workloadmeta.CollectorEvent{
		Source: workloadmeta.SourceNodeOrchestrator,
		Type:   workloadmeta.EventTypeSet,
		Entity: entity,
	})

	return events
}

// podUID returns the UID the kubelet knows pod by, which the kubelet
// collector stores pods under. It's the pod's own UID, except for a static
// pod: the API server only holds its mirror pod, under another UID, while
// the kubelet, its /pods endpoint, the downward API and the log paths under
// /var/log/pods all use the static pod's UID, a hash of its manifest.
func podUID(pod *corev1.Pod) string {
	if uid := staticPodUID(pod); uid != "" {
		return uid
	}
	return string(pod.UID)
}

// staticPodUID returns the UID of the static pod that pod mirrors, or "" when
// pod isn't a mirror pod.
func staticPodUID(pod *corev1.Pod) string {
	return pod.Annotations[mirrorPodAnnotation]
}

// parsePodContainers builds the OrchestratorContainer references for a pod's
// entity plus the corresponding Container entity events, matching container
// statuses (from the pod's status, which carries the real container ID) with
// their container spec (from the pod's spec, which carries env vars and
// resource requirements). Each Container entity carries its pod's namespace
// as the io.kubernetes.pod.namespace label, like the kubelet collector's and
// the container runtimes' own.
func parsePodContainers(
	containerSpecs []corev1.Container,
	containerStatuses []corev1.ContainerStatus,
	namespace string,
	parent *workloadmeta.EntityID,
) ([]workloadmeta.OrchestratorContainer, []workloadmeta.CollectorEvent) {
	podContainers := make([]workloadmeta.OrchestratorContainer, 0, len(containerStatuses))
	events := make([]workloadmeta.CollectorEvent, 0, len(containerStatuses))

	for _, status := range containerStatuses {
		if status.ContainerID == "" {
			// The runtime has not created this container yet; it will be
			// picked up on a later update once it has an ID.
			continue
		}

		runtime, containerID := containers.SplitEntityName(status.ContainerID)

		imageID := strings.TrimPrefix(status.ImageID, dockerImageIDPrefix)
		image, err := workloadmeta.NewContainerImage(imageID, status.Image)
		if err != nil {
			if stdErrors.Is(err, pkgcontainersimage.ErrImageIsSha256) {
				image, err = workloadmeta.NewContainerImage(imageID, imageID)
			}
			if err != nil {
				log.Debugf("cannot parse image name %q / %q for container %q: %s", status.Image, imageID, status.Name, err)
			}
		}

		var env map[string]string
		var ports []workloadmeta.ContainerPort
		var resources workloadmeta.ContainerResources
		var resizePolicy workloadmeta.ContainerResizePolicy

		if spec := findContainerSpec(status.Name, containerSpecs); spec != nil {
			env = extractEnvFromSpec(spec.Env)
			ports = extractPorts(spec.Ports)
			resources = extractResources(spec)
			resizePolicy = extractResizePolicy(spec)

			// Prefer the image from the spec over the status: the status's
			// image can still be the previous one right after a container is
			// resized/recreated, while the spec always reflects the current
			// desired image. Keep the status's image if the spec's can't be
			// parsed (e.g. it is empty), rather than overwriting a good image
			// with a half-populated one.
			specImage, specErr := workloadmeta.NewContainerImage(imageID, spec.Image)
			if specErr != nil {
				log.Debugf("cannot parse image name %q for container %q: %s", spec.Image, status.Name, specErr)
			} else {
				image = specImage
			}
		} else {
			log.Debugf("cannot find spec for container %q", status.Name)
		}

		podContainers = append(podContainers, workloadmeta.OrchestratorContainer{
			ID:           containerID,
			Name:         status.Name,
			Image:        image,
			Resources:    resources,
			ResizePolicy: resizePolicy,
		})

		containerState := workloadmeta.ContainerState{}
		switch {
		case status.State.Running != nil:
			containerState.Running = true
			containerState.Status = workloadmeta.ContainerStatusRunning
			containerState.CreatedAt = status.State.Running.StartedAt.Time
			containerState.StartedAt = status.State.Running.StartedAt.Time
		case status.State.Terminated != nil:
			containerState.Running = false
			containerState.Status = workloadmeta.ContainerStatusStopped
			containerState.CreatedAt = status.State.Terminated.StartedAt.Time
			containerState.StartedAt = status.State.Terminated.StartedAt.Time
			containerState.FinishedAt = status.State.Terminated.FinishedAt.Time
		}

		if status.Ready {
			containerState.Health = workloadmeta.ContainerHealthHealthy
		} else {
			containerState.Health = workloadmeta.ContainerHealthUnhealthy
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNodeOrchestrator,
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   containerID,
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: status.Name,
					Labels: map[string]string{
						kubernetes.CriContainerNamespaceLabel: namespace,
					},
				},
				Image:        image,
				EnvVars:      env,
				Ports:        ports,
				Runtime:      workloadmeta.ContainerRuntime(runtime),
				State:        containerState,
				Resources:    resources,
				ResizePolicy: resizePolicy,
				Owner:        parent,
			},
		})
	}

	return podContainers, events
}

// ephemeralContainerSpecs converts ephemeral container specs to the plain
// corev1.Container shape parsePodContainers expects. EphemeralContainerCommon
// is a field-for-field copy of Container meant for exactly this conversion
// (see corev1's own var _ = Container(EphemeralContainerCommon{}) assertion).
func ephemeralContainerSpecs(ephemeralContainers []corev1.EphemeralContainer) []corev1.Container {
	specs := make([]corev1.Container, 0, len(ephemeralContainers))
	for _, ec := range ephemeralContainers {
		specs = append(specs, corev1.Container(ec.EphemeralContainerCommon))
	}
	return specs
}

func findContainerSpec(name string, specs []corev1.Container) *corev1.Container {
	for i := range specs {
		if specs[i].Name == name {
			return &specs[i]
		}
	}
	return nil
}

// extractEnvFromSpec resolves the static env vars declared on a container
// spec, skipping anything sourced from ValueFrom (configmap/secret/field
// refs) since those require calls this collector does not make, and
// filtering out variables not in the configured allow-list.
func extractEnvFromSpec(envSpec []corev1.EnvVar) map[string]string {
	envSpec = slices.DeleteFunc(slices.Clone(envSpec), func(v corev1.EnvVar) bool {
		return v.ValueFrom != nil
	})

	env := make(map[string]string)
	mappingFunc := expansion.MappingFuncFor(env)

	for _, e := range envSpec {
		if !containers.EnvVarFilterFromConfig().IsIncluded(e.Name) {
			continue
		}

		value := e.Value
		ok := true
		if value != "" {
			value, ok = expansion.Expand(value, mappingFunc)
		}
		if !ok {
			continue
		}

		env[e.Name] = value
	}

	return env
}

// extractPorts lists the ports a container spec declares, as the kubelet
// collector does, without their host port: the tagger resolves the %%port%%
// template variables of autodiscovery tag annotations from them. It returns
// nil for a container that declares none.
func extractPorts(specPorts []corev1.ContainerPort) []workloadmeta.ContainerPort {
	if len(specPorts) == 0 {
		return nil
	}

	ports := make([]workloadmeta.ContainerPort, 0, len(specPorts))
	for _, port := range specPorts {
		ports = append(ports, workloadmeta.ContainerPort{
			Name:     port.Name,
			Port:     int(port.ContainerPort),
			Protocol: string(port.Protocol),
		})
	}

	return ports
}

func extractResources(spec *corev1.Container) workloadmeta.ContainerResources {
	return workloadmeta.ContainerResources{
		GPUVendorList: gpuVendorsFromLimits(spec.Resources.Limits),
	}
}

func extractResizePolicy(spec *corev1.Container) workloadmeta.ContainerResizePolicy {
	var policy workloadmeta.ContainerResizePolicy

	for _, rule := range spec.ResizePolicy {
		switch rule.ResourceName {
		case corev1.ResourceCPU:
			policy.CPURestartPolicy = string(rule.RestartPolicy)
		case corev1.ResourceMemory:
			policy.MemoryRestartPolicy = string(rule.RestartPolicy)
		}
	}

	return policy
}

func gpuVendorsFromContainers(containerSpecLists ...[]corev1.Container) []string {
	unique := make(map[string]struct{})
	for _, specs := range containerSpecLists {
		for _, spec := range specs {
			for _, vendor := range gpuVendorsFromLimits(spec.Resources.Limits) {
				unique[vendor] = struct{}{}
			}
		}
	}

	return sortedKeys(unique)
}

func gpuVendorsFromLimits(limits corev1.ResourceList) []string {
	unique := make(map[string]struct{})
	for resourceName := range limits {
		if vendor, found := gpu.ExtractSimpleGPUName(gpu.ResourceGPU(resourceName)); found {
			unique[vendor] = struct{}{}
		}
	}

	return sortedKeys(unique)
}

// sortedKeys returns the set's members in a stable order, so an unchanged pod
// always yields the same GPUVendorList instead of whatever order map
// iteration happens to produce.
func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
