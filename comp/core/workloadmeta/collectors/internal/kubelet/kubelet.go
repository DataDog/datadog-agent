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
	"strings"
	"time"

	"go.uber.org/fx"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/internal/third_party/golang/expansion"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	pkgcontainersimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	collectorID         = "kubelet"
	componentName       = "workloadmeta-kubelet"
	expireFreq          = 15 * time.Second
	dockerImageIDPrefix = "docker-pullable://"
)

type collector struct {
	id         string
	catalog    workloadmeta.AgentType
	watcher    *kubelet.PodWatcher
	store      workloadmeta.Component
	lastExpire time.Time
	expireFreq time.Duration
}

// NewCollector returns a kubelet CollectorProvider that instantiates its collector
func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:      collectorID,
			catalog: workloadmeta.NodeAgent | workloadmeta.ProcessAgent,
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

	var err error

	c.store = store
	c.lastExpire = time.Now()
	c.expireFreq = expireFreq
	c.watcher, err = kubelet.NewPodWatcher(expireFreq)
	if err != nil {
		return err
	}

	return nil
}

func (c *collector) Pull(ctx context.Context) error {
	updatedPods, err := c.watcher.PullChanges(ctx)
	if err != nil {
		return err
	}

	events := c.parsePods(updatedPods)

	if time.Since(c.lastExpire) >= c.expireFreq {
		var expiredIDs []string
		expiredIDs, err = c.watcher.Expire()
		if err == nil {
			events = append(events, c.parseExpires(expiredIDs)...)
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

func (c *collector) parsePods(pods []*kubelet.Pod) []workloadmeta.CollectorEvent {
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
			len(pod.Spec.InitContainers) > 10000 {
			log.Errorf("pod %s has a crazy number of containers: %d or init containers: %d. Skipping it!",
				podMeta.UID, len(pod.Spec.Containers), len(pod.Spec.InitContainers))
			continue
		}

		podID := workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   podMeta.UID,
		}

		podInitContainers, initContainerEvents := c.parsePodContainers(
			pod,
			pod.Spec.InitContainers,
			pod.Status.InitContainers,
			&podID,
		)

		podContainers, containerEvents := c.parsePodContainers(
			pod,
			pod.Spec.Containers,
			pod.Status.Containers,
			&podID,
		)

		GPUVendors := getGPUVendorsFromContainers(initContainerEvents, containerEvents)

		podOwners := pod.Owners()
		owners := make([]workloadmeta.KubernetesPodOwner, 0, len(podOwners))
		for _, o := range podOwners {
			owners = append(owners, workloadmeta.KubernetesPodOwner{
				Kind: o.Kind,
				Name: o.Name,
				ID:   o.ID,
			})
		}

		PodSecurityContext := extractPodSecurityContext(&pod.Spec)
		RuntimeClassName := extractPodRuntimeClassName(&pod.Spec)

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
			Ready:                      kubelet.IsPodReady(pod),
			Phase:                      pod.Status.Phase,
			IP:                         pod.Status.PodIP,
			PriorityClass:              pod.Spec.PriorityClassName,
			QOSClass:                   pod.Status.QOSClass,
			GPUVendorList:              GPUVendors,
			RuntimeClass:               RuntimeClassName,
			SecurityContext:            PodSecurityContext,
		}

		events = append(events, initContainerEvents...)
		events = append(events, containerEvents...)
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNodeOrchestrator,
			Type:   workloadmeta.EventTypeSet,
			Entity: entity,
		})
	}

	return events
}

func (c *collector) parsePodContainers(
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
				Image:           image,
				EnvVars:         env,
				SecurityContext: containerSecurityContext,
				Ports:           ports,
				Runtime:         workloadmeta.ContainerRuntime(runtime),
				State:           containerState,
				Owner:           parent,
				Resources:       resources,
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

func extractEnvFromSpec(envSpec []kubelet.EnvVar) map[string]string {
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

		runtimeVal := e.Value
		if runtimeVal != "" {
			runtimeVal = expansion.Expand(runtimeVal, mappingFunc)
		}

		env[e.Name] = runtimeVal
	}

	return env
}

func extractGPUVendor(gpuNamePrefix kubelet.ResourceName) string {
	gpuVendor := ""
	switch gpuNamePrefix {
	case kubelet.ResourcePrefixNvidiaMIG, kubelet.ResourceGenericNvidiaGPU:
		gpuVendor = "nvidia"
	case kubelet.ResourcePrefixAMDGPU:
		gpuVendor = "amd"
	case kubelet.ResourcePrefixIntelGPU:
		gpuVendor = "intel"
	default:
		gpuVendor = string(gpuNamePrefix)
	}
	return gpuVendor
}

func extractResources(spec *kubelet.ContainerSpec) workloadmeta.ContainerResources {
	resources := workloadmeta.ContainerResources{}
	if cpuReq, found := spec.Resources.Requests[kubelet.ResourceCPU]; found {
		resources.CPURequest = pointer.Ptr(cpuReq.AsApproximateFloat64() * 100) // For 100Mi, AsApproximate returns 0.1, we return 10%
	}

	if memoryReq, found := spec.Resources.Requests[kubelet.ResourceMemory]; found {
		resources.MemoryRequest = pointer.Ptr(uint64(memoryReq.Value()))
	}

	// extract GPU resource info from the possible GPU sources
	uniqueGPUVendor := make(map[string]bool)

	resourceKeys := make([]kubelet.ResourceName, 0, len(spec.Resources.Requests))
	for resourceName := range spec.Resources.Requests {
		resourceKeys = append(resourceKeys, resourceName)
	}

	for _, gpuResourceName := range kubelet.GetGPUResourceNames() {
		for _, resourceKey := range resourceKeys {
			if strings.HasPrefix(string(resourceKey), string(gpuResourceName)) {
				if gpuReq, found := spec.Resources.Requests[resourceKey]; found {
					resources.GPURequest = pointer.Ptr(uint64(gpuReq.Value()))
					uniqueGPUVendor[extractGPUVendor(gpuResourceName)] = true
					break
				}
			}
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

func (c *collector) parseExpires(expiredIDs []string) []workloadmeta.CollectorEvent {
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
