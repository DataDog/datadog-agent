// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

// Package kubelet implements the kubelet Workloadmeta collector.
package kubelet

import (
	"context"
	stdErrors "errors"
	"slices"
	"strings"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/internal/third_party/golang/expansion"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	pkgcontainersimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
	"github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID         = "kubelet"
	componentName       = "workloadmeta-kubelet"
	expireFreq          = 15 * time.Second
	dockerImageIDPrefix = "docker-pullable://"
)

type dependencies struct {
	fx.In

	Config config.Component
}

type collector struct {
	id                         string
	catalog                    workloadmeta.AgentType
	store                      workloadmeta.Component
	collectEphemeralContainers bool

	// These fields are only used when querying the Kubelet directly
	kubeUtil             kubelet.KubeUtilInterface
	lastSeenPodUIDs      map[string]time.Time
	lastSeenContainerIDs map[string]time.Time

	// usePodWatcher indicates whether to use the pod watcher for collecting
	// pods. The new implementation queries the Kubelet directly instead. This
	// option is only here as a fallback in case the new implementation causes
	// issues.
	usePodWatcher bool
	watcher       *kubelet.PodWatcher // only used if usePodWatcher is true
	lastExpire    time.Time           // only used if usePodWatcher is true
}

// NewCollector returns a kubelet CollectorProvider that instantiates its collector
func NewCollector(deps dependencies) (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:                         collectorID,
			catalog:                    workloadmeta.NodeAgent | workloadmeta.ProcessAgent,
			collectEphemeralContainers: deps.Config.GetBool("include_ephemeral_containers"),
			usePodWatcher:              deps.Config.GetBool("kubelet_use_pod_watcher"),
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

func (c *collector) Start(_ context.Context, store workloadmeta.Component) error {
	if !env.IsFeaturePresent(env.Kubernetes) {
		return errors.NewDisabled(componentName, "Agent is not running on Kubernetes")
	}

	c.store = store

	var err error

	if c.usePodWatcher {
		c.watcher, err = kubelet.NewPodWatcher(expireFreq)
		if err != nil {
			return err
		}
		c.lastExpire = time.Now()
	} else {
		c.kubeUtil, err = kubelet.GetKubeUtil()
		if err != nil {
			return err
		}
		c.lastSeenPodUIDs = make(map[string]time.Time)
		c.lastSeenContainerIDs = make(map[string]time.Time)
	}

	return nil
}

func (c *collector) Pull(ctx context.Context) error {
	if c.usePodWatcher {
		return c.pullUsingPodWatcher(ctx)
	}

	return c.pullFromKubelet(ctx)
}

func (c *collector) pullFromKubelet(ctx context.Context) error {
	podList, err := c.kubeUtil.GetLocalPodList(ctx)
	if err != nil {
		return err
	}

	events := parsePods(podList, c.collectEphemeralContainers)

	// Mark return pods and containers as seen now
	now := time.Now()
	for _, pod := range podList {
		if pod.Metadata.UID != "" {
			c.lastSeenPodUIDs[pod.Metadata.UID] = now
		}
		for _, container := range pod.Status.GetAllContainers() {
			if container.ID != "" {
				c.lastSeenContainerIDs[container.ID] = now
			}
		}
	}

	expireEvents := c.eventsForExpiredEntities(now)
	events = append(events, expireEvents...)

	c.store.Notify(events)

	return nil
}

// eventsForExpiredEntities returns a list of workloadmeta.CollectorEvent
// containing events for expired pods and containers.
// The old implementation based on a pod watcher expired pods and containers
// at a set frequency (expireFreq). Instead, we could delete them on every
// pull by keeping a list of items from the last pull and removing those
// not seen in the current one. That would be simpler and likely safe,
// but to avoid unexpected issues, we’ll keep the old behavior for now.
func (c *collector) eventsForExpiredEntities(now time.Time) []workloadmeta.CollectorEvent {
	var events []workloadmeta.CollectorEvent

	// Find expired pods
	var expiredPodUIDs []string
	for uid, lastSeen := range c.lastSeenPodUIDs {
		if now.Sub(lastSeen) > expireFreq {
			expiredPodUIDs = append(expiredPodUIDs, uid)
			delete(c.lastSeenPodUIDs, uid)
		}
	}

	// Find expired containers
	var expiredContainerIDs []string
	for containerID, lastSeen := range c.lastSeenContainerIDs {
		if now.Sub(lastSeen) > expireFreq {
			expiredContainerIDs = append(expiredContainerIDs, containerID)
			delete(c.lastSeenContainerIDs, containerID)
		}
	}

	events = append(events, parseExpiredPods(expiredPodUIDs)...)
	events = append(events, parseExpiredContainers(expiredContainerIDs)...)

	return events
}

func parseExpiredPods(expiredPodUIDs []string) []workloadmeta.CollectorEvent {
	events := make([]workloadmeta.CollectorEvent, 0, len(expiredPodUIDs))

	for _, uid := range expiredPodUIDs {
		entity := &workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesPod,
				ID:   uid,
			},
			FinishedAt: time.Now(),
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNodeOrchestrator,
			Type:   workloadmeta.EventTypeUnset,
			Entity: entity,
		})
	}

	return events
}

func parseExpiredContainers(expiredContainerIDs []string) []workloadmeta.CollectorEvent {
	events := make([]workloadmeta.CollectorEvent, 0, len(expiredContainerIDs))

	for _, containerID := range expiredContainerIDs {
		// Split the container ID to get just the ID part (remove runtime prefix like "docker://")
		_, id := containers.SplitEntityName(containerID)

		entity := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   id,
			},
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNodeOrchestrator,
			Type:   workloadmeta.EventTypeUnset,
			Entity: entity,
		})
	}

	return events
}

func (c *collector) pullUsingPodWatcher(ctx context.Context) error {
	updatedPods, err := c.watcher.PullChanges(ctx)
	if err != nil {
		return err
	}

	events := parsePods(updatedPods, c.collectEphemeralContainers)

	if time.Since(c.lastExpire) >= expireFreq {
		var expiredIDs []string
		expiredIDs, err = c.watcher.Expire()
		if err == nil {
			events = append(events, parseExpires(expiredIDs)...)
			c.lastExpire = time.Now()
		}
	}

	c.store.Notify(events)

	return err
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

func parsePods(pods []*kubelet.Pod, collectEphemeralContainers bool) []workloadmeta.CollectorEvent {
	events := []workloadmeta.CollectorEvent{}

	for _, pod := range pods {
		podMeta := pod.Metadata
		if podMeta.UID == "" {
			log.Debugf("pod has no UID. meta: %+v", podMeta)
			continue
		}

		// Validate allocation size.
		// Limits hardcoded here are huge enough to never be hit.
		if len(pod.Spec.Containers) > 10000 ||
			len(pod.Spec.InitContainers) > 10000 || len(pod.Spec.EphemeralContainers) > 10000 {
			log.Errorf("pod %s has a crazy number of containers: %d, init containers: %d or ephemeral containers: %d. Skipping it!",
				podMeta.UID, len(pod.Spec.Containers), len(pod.Spec.InitContainers), len(pod.Spec.EphemeralContainers))
			continue
		}

		podID := workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   podMeta.UID,
		}

		podInitContainers, initContainerEvents := parsePodContainers(
			pod,
			pod.Spec.InitContainers,
			pod.Status.InitContainers,
			&podID,
		)

		podContainers, containerEvents := parsePodContainers(
			pod,
			pod.Spec.Containers,
			pod.Status.Containers,
			&podID,
		)

		var podEphemeralContainers []workloadmeta.OrchestratorContainer
		var ephemeralContainerEvents []workloadmeta.CollectorEvent
		if collectEphemeralContainers {
			podEphemeralContainers, ephemeralContainerEvents = parsePodContainers(
				pod,
				pod.Spec.EphemeralContainers,
				pod.Status.EphemeralContainers,
				&podID,
			)
		}

		GPUVendors := getGPUVendorsFromContainers(initContainerEvents, containerEvents)

		podOwners := pod.Owners()
		owners := make([]workloadmeta.KubernetesPodOwner, 0, len(podOwners))
		for _, o := range podOwners {
			owners = append(owners, workloadmeta.KubernetesPodOwner{
				Kind:       o.Kind,
				Name:       o.Name,
				ID:         o.ID,
				Controller: o.Controller,
			})
		}

		PodSecurityContext := extractPodSecurityContext(&pod.Spec)
		RuntimeClassName := extractPodRuntimeClassName(&pod.Spec)

		var startTime *time.Time
		if !pod.Status.StartTime.IsZero() {
			startTime = &pod.Status.StartTime
		}

		entity := &workloadmeta.KubernetesPod{
			EntityID: podID,
			EntityMeta: workloadmeta.EntityMeta{
				Name:        podMeta.Name,
				Namespace:   podMeta.Namespace,
				Annotations: podMeta.Annotations,
				Labels:      podMeta.Labels,
			},
			Owners:                     owners,
			PersistentVolumeClaimNames: pod.GetPersistentVolumeClaimNames(),
			InitContainers:             podInitContainers,
			Containers:                 podContainers,
			EphemeralContainers:        podEphemeralContainers,
			Ready:                      kubelet.IsPodReady(pod),
			Phase:                      pod.Status.Phase,
			IP:                         pod.Status.PodIP,
			PriorityClass:              pod.Spec.PriorityClassName,
			QOSClass:                   pod.Status.QOSClass,
			GPUVendorList:              GPUVendors,
			RuntimeClass:               RuntimeClassName,
			SecurityContext:            PodSecurityContext,
			CreationTimestamp:          podMeta.CreationTimestamp,
			StartTime:                  startTime,
			NodeName:                   pod.Spec.NodeName,
			HostIP:                     pod.Status.HostIP,
			HostNetwork:                pod.Spec.HostNetwork,
			InitContainerStatuses:      convertContainerStatuses(pod.Status.InitContainers),
			ContainerStatuses:          convertContainerStatuses(pod.Status.Containers),
			Conditions:                 convertConditions(pod.Status.Conditions),
			Volumes:                    convertVolumes(pod.Spec.Volumes),
			Tolerations:                convertTolerations(pod.Spec.Tolerations),
			Reason:                     pod.Status.Reason,
		}

		events = append(events, initContainerEvents...)
		events = append(events, containerEvents...)
		events = append(events, ephemeralContainerEvents...)
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNodeOrchestrator,
			Type:   workloadmeta.EventTypeSet,
			Entity: entity,
		})
	}

	return events
}

func parsePodContainers(
	pod *kubelet.Pod,
	containerSpecs []kubelet.ContainerSpec,
	containerStatuses []kubelet.ContainerStatus,
	parent *workloadmeta.EntityID,
) ([]workloadmeta.OrchestratorContainer, []workloadmeta.CollectorEvent) {
	podContainers := make([]workloadmeta.OrchestratorContainer, 0, len(containerStatuses))
	events := make([]workloadmeta.CollectorEvent, 0, len(containerStatuses))

	for _, container := range containerStatuses {
		if container.ID == "" {
			// A container without an ID has not been created by
			// the runtime yet, so we ignore them until it's
			// detected again.
			continue
		}

		var containerSecurityContext *workloadmeta.ContainerSecurityContext
		var readinessProbe *workloadmeta.ContainerProbe
		var env map[string]string
		var ports []workloadmeta.ContainerPort
		var resources workloadmeta.ContainerResources

		// When running on docker, the image ID contains a prefix that's not
		// included in other runtimes. Remove it for consistency.
		// See https://github.com/kubernetes/kubernetes/issues/95968
		imageID := strings.TrimPrefix(container.ImageID, dockerImageIDPrefix)

		image, err := workloadmeta.NewContainerImage(imageID, container.Image)
		if err != nil {
			if stdErrors.Is(err, pkgcontainersimage.ErrImageIsSha256) {
				// try the resolved image ID if the image name in the container
				// status is a SHA256. this seems to happen sometimes when
				// pinning the image to a SHA256
				image, err = workloadmeta.NewContainerImage(imageID, imageID)
			}

			if err != nil {
				log.Debugf("cannot split image name %q nor %q: %s", container.Image, imageID, err)
			}
		}

		runtime, containerID := containers.SplitEntityName(container.ID)
		podContainer := workloadmeta.OrchestratorContainer{
			ID:   containerID,
			Name: container.Name,
		}

		containerSpec := findContainerSpec(container.Name, containerSpecs)
		if containerSpec != nil {
			env = extractEnvFromSpec(containerSpec.Env)
			resources = extractResources(containerSpec)

			podContainer.Image, err = workloadmeta.NewContainerImage(imageID, containerSpec.Image)
			if err != nil {
				log.Debugf("cannot split image name %q: %s", containerSpec.Image, err)
			}

			podContainer.Image.ID = imageID
			containerSecurityContext = extractContainerSecurityContext(containerSpec)
			readinessProbe = extractReadinessProbe(containerSpec)
			ports = make([]workloadmeta.ContainerPort, 0, len(containerSpec.Ports))
			for _, port := range containerSpec.Ports {
				ports = append(ports, workloadmeta.ContainerPort{
					Name:     port.Name,
					Port:     port.ContainerPort,
					Protocol: port.Protocol,
				})
			}
		} else {
			log.Debugf("cannot find spec for container %q", container.Name)
		}

		var allocatedResources []workloadmeta.ContainerAllocatedResource
		for _, resource := range container.ResolvedAllocatedResources {
			allocatedResources = append(allocatedResources, workloadmeta.ContainerAllocatedResource{
				Name: resource.Name,
				ID:   resource.ID,
			})
		}

		containerState := workloadmeta.ContainerState{}
		if st := container.State.Running; st != nil {
			containerState.Running = true
			containerState.Status = workloadmeta.ContainerStatusRunning
			containerState.StartedAt = st.StartedAt
			containerState.CreatedAt = st.StartedAt // CreatedAt not available
		} else if st := container.State.Terminated; st != nil {
			containerState.Running = false
			containerState.Status = workloadmeta.ContainerStatusStopped
			containerState.CreatedAt = st.StartedAt
			containerState.StartedAt = st.StartedAt
			containerState.FinishedAt = st.FinishedAt
		}

		// Kubelet considers containers without probe to be ready
		if container.Ready {
			containerState.Health = workloadmeta.ContainerHealthHealthy
		} else {
			containerState.Health = workloadmeta.ContainerHealthUnhealthy
		}

		podContainers = append(podContainers, podContainer)
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNodeOrchestrator,
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   containerID,
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: container.Name,
					Labels: map[string]string{
						kubernetes.CriContainerNamespaceLabel: pod.Metadata.Namespace,
					},
				},
				Image:                      image,
				EnvVars:                    env,
				SecurityContext:            containerSecurityContext,
				ReadinessProbe:             readinessProbe,
				Ports:                      ports,
				Runtime:                    workloadmeta.ContainerRuntime(runtime),
				State:                      containerState,
				Owner:                      parent,
				Resources:                  resources,
				ResolvedAllocatedResources: allocatedResources,
			},
		})
	}

	return podContainers, events
}

func getGPUVendorsFromContainers(initContainerEvents, containerEvents []workloadmeta.CollectorEvent) []string {
	gpuUniqueTypes := make(map[string]bool)
	for _, event := range append(initContainerEvents, containerEvents...) {
		container := event.Entity.(*workloadmeta.Container)
		for _, GPUVendor := range container.Resources.GPUVendorList {
			gpuUniqueTypes[GPUVendor] = true
		}
	}

	GPUVendors := make([]string, 0, len(gpuUniqueTypes))
	for GPUVendor := range gpuUniqueTypes {
		GPUVendors = append(GPUVendors, GPUVendor)
	}

	return GPUVendors
}

func extractPodRuntimeClassName(spec *kubelet.Spec) string {
	if spec.RuntimeClassName == nil {
		return ""
	}
	return *spec.RuntimeClassName
}

func extractPodSecurityContext(spec *kubelet.Spec) *workloadmeta.PodSecurityContext {
	if spec.SecurityContext == nil {
		return nil
	}

	return &workloadmeta.PodSecurityContext{
		RunAsUser:  spec.SecurityContext.RunAsUser,
		RunAsGroup: spec.SecurityContext.RunAsGroup,
		FsGroup:    spec.SecurityContext.FsGroup,
	}
}

func extractContainerSecurityContext(spec *kubelet.ContainerSpec) *workloadmeta.ContainerSecurityContext {
	if spec.SecurityContext == nil {
		return nil
	}

	var caps *workloadmeta.Capabilities
	if spec.SecurityContext.Capabilities != nil {
		caps = &workloadmeta.Capabilities{
			Add:  spec.SecurityContext.Capabilities.Add,
			Drop: spec.SecurityContext.Capabilities.Drop,
		}
	}

	privileged := false
	if spec.SecurityContext.Privileged != nil {
		privileged = *spec.SecurityContext.Privileged
	}

	var seccompProfile *workloadmeta.SeccompProfile
	if spec.SecurityContext.SeccompProfile != nil {
		localhostProfile := ""
		if spec.SecurityContext.SeccompProfile.LocalhostProfile != nil {
			localhostProfile = *spec.SecurityContext.SeccompProfile.LocalhostProfile
		}

		spType := workloadmeta.SeccompProfileType(spec.SecurityContext.SeccompProfile.Type)

		seccompProfile = &workloadmeta.SeccompProfile{
			Type:             spType,
			LocalhostProfile: localhostProfile,
		}
	}

	return &workloadmeta.ContainerSecurityContext{
		Capabilities:   caps,
		Privileged:     privileged,
		SeccompProfile: seccompProfile,
	}
}

func extractReadinessProbe(spec *kubelet.ContainerSpec) *workloadmeta.ContainerProbe {
	if spec.ReadinessProbe == nil {
		return nil
	}

	return &workloadmeta.ContainerProbe{
		InitialDelaySeconds: int32(spec.ReadinessProbe.InitialDelaySeconds),
	}
}

func extractEnvFromSpec(envSpec []kubelet.EnvVar) map[string]string {
	// filter out env vars that have external sources (eg. ConfigMap, Secret, etc.)
	envSpec = slices.DeleteFunc(envSpec, func(v kubelet.EnvVar) bool {
		return v.ValueFrom != nil
	})

	env := make(map[string]string)
	mappingFunc := expansion.MappingFuncFor(env)

	// TODO: Implement support of environment variables set from ConfigMap,
	// Secret, DownwardAPI.
	// See https://github.com/kubernetes/kubernetes/blob/d20fd4088476ec39c5ae2151b8fffaf0f4834418/pkg/kubelet/kubelet_pods.go#L566
	// for the complete environment variable resolution process that is
	// done by the kubelet.

	for _, e := range envSpec {
		if !containers.EnvVarFilterFromConfig().IsIncluded(e.Name) {
			continue
		}

		ok := true
		runtimeVal := e.Value
		if runtimeVal != "" {
			runtimeVal, ok = expansion.Expand(runtimeVal, mappingFunc)
		}

		// Ignore environment variables that failed to expand
		// This occurs when the env var references another env var
		// that has its value sourced from an external source
		// (eg. ConfigMap, Secret, DownwardAPI)
		if !ok {
			continue
		}

		env[e.Name] = runtimeVal
	}

	return env
}

func extractResources(spec *kubelet.ContainerSpec) workloadmeta.ContainerResources {
	resources := workloadmeta.ContainerResources{}

	if spec.Resources == nil {
		// Ephemeral containers do not have resources defined
		return resources
	}

	if cpuReq, found := spec.Resources.Requests[kubelet.ResourceCPU]; found {
		resources.CPURequest = kubernetes.FormatCPURequests(cpuReq)
	}

	if memoryReq, found := spec.Resources.Requests[kubelet.ResourceMemory]; found {
		resources.MemoryRequest = kubernetes.FormatMemoryRequests(memoryReq)
	}

	if cpuLimit, found := spec.Resources.Limits[kubelet.ResourceCPU]; found {
		resources.CPULimit = kubernetes.FormatCPURequests(cpuLimit)
	}

	if memoryLimit, found := spec.Resources.Limits[kubelet.ResourceMemory]; found {
		resources.MemoryLimit = kubernetes.FormatMemoryRequests(memoryLimit)
	}

	// extract GPU resource info from the possible GPU sources
	uniqueGPUVendor := make(map[string]struct{})
	for resourceName := range spec.Resources.Requests {
		gpuName, found := gpu.ExtractSimpleGPUName(gpu.ResourceGPU(resourceName))
		if found {
			uniqueGPUVendor[gpuName] = struct{}{}
		}
	}

	gpuVendorList := make([]string, 0, len(uniqueGPUVendor))
	for GPUVendor := range uniqueGPUVendor {
		gpuVendorList = append(gpuVendorList, GPUVendor)
	}
	resources.GPUVendorList = gpuVendorList

	return resources
}

func findContainerSpec(name string, specs []kubelet.ContainerSpec) *kubelet.ContainerSpec {
	for _, spec := range specs {
		if spec.Name == name {
			return &spec
		}
	}

	return nil
}

func convertVolumes(volumes []kubelet.VolumeSpec) []workloadmeta.KubernetesPodVolume {
	if volumes == nil {
		return nil
	}

	result := make([]workloadmeta.KubernetesPodVolume, len(volumes))

	for i, volume := range volumes {
		result[i] = workloadmeta.KubernetesPodVolume{
			Name: volume.Name,
		}

		if volume.PersistentVolumeClaim != nil {
			result[i].PersistentVolumeClaim = &workloadmeta.KubernetesPersistentVolumeClaim{
				ClaimName: volume.PersistentVolumeClaim.ClaimName,
				ReadOnly:  volume.PersistentVolumeClaim.ReadOnly,
			}
		}

		if volume.Ephemeral != nil && volume.Ephemeral.VolumeClaimTemplate != nil {
			result[i].Ephemeral = &workloadmeta.KubernetesEphemeralVolume{
				Name:        volume.Ephemeral.VolumeClaimTemplate.Metadata.Name,
				UID:         volume.Ephemeral.VolumeClaimTemplate.Metadata.UID,
				Annotations: volume.Ephemeral.VolumeClaimTemplate.Metadata.Annotations,
				Labels:      volume.Ephemeral.VolumeClaimTemplate.Metadata.Labels,
			}
		}
	}

	return result
}

func convertTolerations(tolerations []kubelet.Toleration) []workloadmeta.KubernetesPodToleration {
	if tolerations == nil {
		return nil
	}

	result := make([]workloadmeta.KubernetesPodToleration, len(tolerations))

	for i, toleration := range tolerations {
		result[i] = workloadmeta.KubernetesPodToleration{
			Key:               toleration.Key,
			Operator:          toleration.Operator,
			Value:             toleration.Value,
			Effect:            toleration.Effect,
			TolerationSeconds: toleration.TolerationSeconds,
		}
	}

	return result
}

func convertConditions(conditions []kubelet.Conditions) []workloadmeta.KubernetesPodCondition {
	if conditions == nil {
		return nil
	}

	result := make([]workloadmeta.KubernetesPodCondition, len(conditions))

	for i, condition := range conditions {
		result[i] = workloadmeta.KubernetesPodCondition{
			Type:   condition.Type,
			Status: condition.Status,
		}
	}

	return result
}

func convertContainerStatuses(containerStatuses []kubelet.ContainerStatus) []workloadmeta.KubernetesContainerStatus {
	if containerStatuses == nil {
		return nil
	}

	result := make([]workloadmeta.KubernetesContainerStatus, len(containerStatuses))

	for i, status := range containerStatuses {
		result[i] = workloadmeta.KubernetesContainerStatus{
			ContainerID:          status.ID,
			Name:                 status.Name,
			Image:                status.Image,
			ImageID:              status.ImageID,
			Ready:                status.Ready,
			RestartCount:         int32(status.RestartCount),
			State:                convertContainerState(status.State),
			LastTerminationState: convertContainerState(status.LastState),
		}
	}

	return result
}

func convertContainerState(state kubelet.ContainerState) workloadmeta.KubernetesContainerState {
	result := workloadmeta.KubernetesContainerState{}

	if state.Waiting != nil {
		result.Waiting = &workloadmeta.KubernetesContainerStateWaiting{
			Reason: state.Waiting.Reason,
		}
	}

	if state.Running != nil {
		result.Running = &workloadmeta.KubernetesContainerStateRunning{
			StartedAt: state.Running.StartedAt,
		}
	}

	if state.Terminated != nil {
		result.Terminated = &workloadmeta.KubernetesContainerStateTerminated{
			ExitCode:   state.Terminated.ExitCode,
			StartedAt:  state.Terminated.StartedAt,
			FinishedAt: state.Terminated.FinishedAt,
			Reason:     state.Terminated.Reason,
		}
	}

	return result
}

func parseExpires(expiredIDs []string) []workloadmeta.CollectorEvent {
	events := make([]workloadmeta.CollectorEvent, 0, len(expiredIDs))
	podTerminatedTime := time.Now()

	for _, expiredID := range expiredIDs {
		prefix, id := containers.SplitEntityName(expiredID)

		var entity workloadmeta.Entity

		if prefix == kubelet.KubePodEntityName {
			entity = &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   id,
				},
				FinishedAt: podTerminatedTime,
			}
		} else {
			entity = &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   id,
				},
			}
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNodeOrchestrator,
			Type:   workloadmeta.EventTypeUnset,
			Entity: entity,
		})
	}

	return events
}
