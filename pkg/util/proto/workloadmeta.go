// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package proto

import (
	"fmt"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

var emptyTimestampUnix = new(time.Time).Unix()

// Conversions from Workloadmeta types to protobuf

// ProtobufEventFromWorkloadmetaEvent converts the given workloadmeta.Event into protobuf
func ProtobufEventFromWorkloadmetaEvent(event workloadmeta.Event) (*pb.WorkloadmetaEvent, error) {
	entity := event.Entity

	if entity == nil {
		return nil, nil
	}

	entityID := entity.GetID()

	protoEventType, err := toProtoEventType(event.Type)
	if err != nil {
		return nil, err
	}

	switch entityID.Kind {
	case workloadmeta.KindContainer:
		container := entity.(*workloadmeta.Container)

		protoContainer, err := protoContainerFromWorkloadmetaContainer(container)
		if err != nil {
			return nil, err
		}

		return &pb.WorkloadmetaEvent{
			Type:      protoEventType,
			Container: protoContainer,
		}, nil
	case workloadmeta.KindKubernetesPod:
		kubernetesPod := entity.(*workloadmeta.KubernetesPod)

		protoKubernetesPod, err := protoKubernetesPodFromWorkloadmetaKubernetesPod(kubernetesPod)
		if err != nil {
			return nil, err
		}

		return &pb.WorkloadmetaEvent{
			Type:          protoEventType,
			KubernetesPod: protoKubernetesPod,
		}, nil
	case workloadmeta.KindECSTask:
		ecsTask := entity.(*workloadmeta.ECSTask)

		protoECSTask, err := protoECSTaskFromWorkloadmetaECSTask(ecsTask)
		if err != nil {
			return nil, err
		}

		return &pb.WorkloadmetaEvent{
			Type:    protoEventType,
			EcsTask: protoECSTask,
		}, nil
	}

	return nil, fmt.Errorf("unknown kind: %s", entityID.Kind)
}

func protoContainerFromWorkloadmetaContainer(container *workloadmeta.Container) (*pb.Container, error) {
	var pbContainerPorts []*pb.ContainerPort
	for _, port := range container.Ports {
		pbContainerPorts = append(pbContainerPorts, toProtoContainerPort(&port))
	}

	protoEntityID, err := toProtoEntityIdFromContainer(container)
	if err != nil {
		return nil, err
	}

	protoRuntime, err := toProtoRuntime(container.Runtime)
	if err != nil {
		return nil, err
	}

	protoContainerState, err := toProtoContainerState(&container.State)
	if err != nil {
		return nil, err
	}

	return &pb.Container{
		EntityId:      protoEntityID,
		EntityMeta:    toProtoEntityMetaFromContainer(container),
		EnvVars:       container.EnvVars,
		Hostname:      container.Hostname,
		Image:         toProtoImage(&container.Image),
		NetworkIps:    container.NetworkIPs,
		Pid:           int32(container.PID),
		Ports:         pbContainerPorts,
		Runtime:       protoRuntime,
		State:         protoContainerState,
		CollectorTags: container.CollectorTags,
	}, nil
}

func toProtoEventType(eventType workloadmeta.EventType) (pb.WorkloadmetaEventType, error) {
	switch eventType {
	case workloadmeta.EventTypeAll:
		return pb.WorkloadmetaEventType_EVENT_TYPE_ALL, nil
	case workloadmeta.EventTypeSet:
		return pb.WorkloadmetaEventType_EVENT_TYPE_SET, nil
	case workloadmeta.EventTypeUnset:
		return pb.WorkloadmetaEventType_EVENT_TYPE_UNSET, nil
	}

	return pb.WorkloadmetaEventType_EVENT_TYPE_ALL, fmt.Errorf("unknown event type: %d", eventType)
}

func toProtoEntityIdFromContainer(container *workloadmeta.Container) (*pb.WorkloadmetaEntityId, error) {
	protoKind, err := toProtoKind(container.Kind)
	if err != nil {
		return nil, err
	}

	return &pb.WorkloadmetaEntityId{
		Kind: protoKind,
		Id:   container.ID,
	}, nil
}

func toProtoKind(kind workloadmeta.Kind) (pb.WorkloadmetaKind, error) {
	switch kind {
	case workloadmeta.KindContainer:
		return pb.WorkloadmetaKind_CONTAINER, nil
	case workloadmeta.KindKubernetesPod:
		return pb.WorkloadmetaKind_KUBERNETES_POD, nil
	case workloadmeta.KindECSTask:
		return pb.WorkloadmetaKind_ECS_TASK, nil
	}

	return pb.WorkloadmetaKind_CONTAINER, fmt.Errorf("unknown kind: %s", kind)
}

func toProtoEntityMetaFromContainer(container *workloadmeta.Container) *pb.EntityMeta {
	return &pb.EntityMeta{
		Name:        container.Name,
		Namespace:   container.Namespace,
		Annotations: container.Annotations,
		Labels:      container.Labels,
	}
}

func toProtoImage(image *workloadmeta.ContainerImage) *pb.ContainerImage {
	return &pb.ContainerImage{
		Id:        image.ID,
		RawName:   image.RawName,
		Name:      image.Name,
		ShortName: image.ShortName,
		Tag:       image.Tag,
	}
}

func toProtoContainerPort(port *workloadmeta.ContainerPort) *pb.ContainerPort {
	return &pb.ContainerPort{
		Name:     port.Name,
		Port:     int32(port.Port),
		Protocol: port.Protocol,
	}
}

func toProtoRuntime(runtime workloadmeta.ContainerRuntime) (pb.Runtime, error) {
	switch runtime {
	case workloadmeta.ContainerRuntimeDocker:
		return pb.Runtime_DOCKER, nil
	case workloadmeta.ContainerRuntimeContainerd:
		return pb.Runtime_CONTAINERD, nil
	case workloadmeta.ContainerRuntimePodman:
		return pb.Runtime_PODMAN, nil
	case workloadmeta.ContainerRuntimeCRIO:
		return pb.Runtime_CRIO, nil
	case workloadmeta.ContainerRuntimeGarden:
		return pb.Runtime_GARDEN, nil
	case workloadmeta.ContainerRuntimeECSFargate:
		return pb.Runtime_ECS_FARGATE, nil
	}

	return pb.Runtime_DOCKER, fmt.Errorf("unknown runtime: %s", runtime)
}

func toProtoContainerState(state *workloadmeta.ContainerState) (*pb.ContainerState, error) {
	protoContainerStatus, err := toProtoContainerStatus(state.Status)
	if err != nil {
		return nil, err
	}

	protoContainerHealth, err := toProtoContainerHealth(state.Health)
	if err != nil {
		return nil, err
	}

	res := &pb.ContainerState{
		Running:    state.Running,
		Status:     protoContainerStatus,
		Health:     protoContainerHealth,
		CreatedAt:  state.CreatedAt.Unix(),
		StartedAt:  state.StartedAt.Unix(),
		FinishedAt: state.FinishedAt.Unix(),
	}

	if state.ExitCode != nil {
		res.ExitCode = *state.ExitCode
	}

	return res, nil
}

func toProtoContainerStatus(status workloadmeta.ContainerStatus) (pb.ContainerStatus, error) {
	switch status {
	case workloadmeta.ContainerStatusUnknown:
		return pb.ContainerStatus_CONTAINER_STATUS_UNKNOWN, nil
	case workloadmeta.ContainerStatusCreated:
		return pb.ContainerStatus_CONTAINER_STATUS_CREATED, nil
	case workloadmeta.ContainerStatusRunning:
		return pb.ContainerStatus_CONTAINER_STATUS_RUNNING, nil
	case workloadmeta.ContainerStatusRestarting:
		return pb.ContainerStatus_CONTAINER_STATUS_RESTARTING, nil
	case workloadmeta.ContainerStatusPaused:
		return pb.ContainerStatus_CONTAINER_STATUS_PAUSED, nil
	case workloadmeta.ContainerStatusStopped:
		return pb.ContainerStatus_CONTAINER_STATUS_STOPPED, nil
	}

	return pb.ContainerStatus_CONTAINER_STATUS_UNKNOWN, fmt.Errorf("unknown status: %s", status)
}

func toProtoContainerHealth(health workloadmeta.ContainerHealth) (pb.ContainerHealth, error) {
	switch health {
	// Some workloadmeta collectors don't set the health, so we need to handle ""
	case "", workloadmeta.ContainerHealthUnknown:
		return pb.ContainerHealth_CONTAINER_HEALTH_UNKNOWN, nil
	case workloadmeta.ContainerHealthHealthy:
		return pb.ContainerHealth_CONTAINER_HEALTH_HEALTHY, nil
	case workloadmeta.ContainerHealthUnhealthy:
		return pb.ContainerHealth_CONTAINER_HEALTH_UNHEALTHY, nil
	}

	return pb.ContainerHealth_CONTAINER_HEALTH_UNKNOWN, fmt.Errorf("unknown health state: %s", health)
}

func protoKubernetesPodFromWorkloadmetaKubernetesPod(kubernetesPod *workloadmeta.KubernetesPod) (*pb.KubernetesPod, error) {
	protoEntityID, err := toProtoEntityIdFromKubernetesPod(kubernetesPod)
	if err != nil {
		return nil, err
	}

	var protoKubernetesPodOwners []*pb.KubernetesPodOwner
	for _, podOwner := range kubernetesPod.Owners {
		protoKubernetesPodOwners = append(protoKubernetesPodOwners, toProtoKubernetesPodOwner(&podOwner))
	}

	var protoOrchestratorContainers []*pb.OrchestratorContainer
	for _, container := range kubernetesPod.Containers {
		protoOrchestratorContainers = append(protoOrchestratorContainers, toProtoOrchestratorContainer(container))
	}

	var protoInitContainers []*pb.OrchestratorContainer
	for _, container := range kubernetesPod.InitContainers {
		protoInitContainers = append(protoInitContainers, toProtoOrchestratorContainer(container))
	}

	return &pb.KubernetesPod{
		EntityId:                   protoEntityID,
		EntityMeta:                 toProtoEntityMetaFromKubernetesPod(kubernetesPod),
		Owners:                     protoKubernetesPodOwners,
		PersistentVolumeClaimNames: kubernetesPod.PersistentVolumeClaimNames,
		InitContainers:             protoInitContainers,
		Containers:                 protoOrchestratorContainers,
		Ready:                      kubernetesPod.Ready,
		Phase:                      kubernetesPod.Phase,
		Ip:                         kubernetesPod.IP,
		PriorityClass:              kubernetesPod.PriorityClass,
		QosClass:                   kubernetesPod.QOSClass,
		KubeServices:               kubernetesPod.KubeServices,
		NamespaceLabels:            kubernetesPod.NamespaceLabels,
	}, nil
}

func toProtoEntityMetaFromKubernetesPod(kubernetesPod *workloadmeta.KubernetesPod) *pb.EntityMeta {
	return &pb.EntityMeta{
		Name:        kubernetesPod.Name,
		Namespace:   kubernetesPod.Namespace,
		Annotations: kubernetesPod.Annotations,
		Labels:      kubernetesPod.Labels,
	}
}

func toProtoEntityIdFromKubernetesPod(kubernetesPod *workloadmeta.KubernetesPod) (*pb.WorkloadmetaEntityId, error) {
	protoKind, err := toProtoKind(kubernetesPod.Kind)
	if err != nil {
		return nil, err
	}

	return &pb.WorkloadmetaEntityId{
		Kind: protoKind,
		Id:   kubernetesPod.ID,
	}, nil
}

func toProtoKubernetesPodOwner(kubernetesPodOwner *workloadmeta.KubernetesPodOwner) *pb.KubernetesPodOwner {
	return &pb.KubernetesPodOwner{
		Kind: kubernetesPodOwner.Kind,
		Name: kubernetesPodOwner.Name,
		Id:   kubernetesPodOwner.ID,
	}
}

func toProtoOrchestratorContainer(container workloadmeta.OrchestratorContainer) *pb.OrchestratorContainer {
	return &pb.OrchestratorContainer{
		Id:    container.ID,
		Name:  container.Name,
		Image: toProtoImage(&container.Image),
	}
}

func protoECSTaskFromWorkloadmetaECSTask(ecsTask *workloadmeta.ECSTask) (*pb.ECSTask, error) {
	protoEntityID, err := toProtoEntityIdFromECSTask(ecsTask)
	if err != nil {
		return nil, err
	}

	protoLaunchType, err := toProtoLaunchType(ecsTask.LaunchType)
	if err != nil {
		return nil, err
	}

	var protoOrchestratorContainers []*pb.OrchestratorContainer
	for _, container := range ecsTask.Containers {
		protoOrchestratorContainers = append(protoOrchestratorContainers, toProtoOrchestratorContainer(container))
	}

	return &pb.ECSTask{
		EntityId:              protoEntityID,
		EntityMeta:            toProtoEntityMetaFromECSTask(ecsTask),
		Tags:                  ecsTask.Tags,
		ContainerInstanceTags: ecsTask.ContainerInstanceTags,
		ClusterName:           ecsTask.ClusterName,
		Region:                ecsTask.Region,
		AvailabilityZone:      ecsTask.AvailabilityZone,
		Family:                ecsTask.Family,
		Version:               ecsTask.Version,
		LaunchType:            protoLaunchType,
		Containers:            protoOrchestratorContainers,
	}, nil
}

func toProtoEntityMetaFromECSTask(ecsTask *workloadmeta.ECSTask) *pb.EntityMeta {
	return &pb.EntityMeta{
		Name:        ecsTask.Name,
		Namespace:   ecsTask.Namespace,
		Annotations: ecsTask.Annotations,
		Labels:      ecsTask.Labels,
	}
}

func toProtoEntityIdFromECSTask(ecsTask *workloadmeta.ECSTask) (*pb.WorkloadmetaEntityId, error) {
	protoKind, err := toProtoKind(ecsTask.Kind)
	if err != nil {
		return nil, err
	}

	return &pb.WorkloadmetaEntityId{
		Kind: protoKind,
		Id:   ecsTask.ID,
	}, nil
}

func toProtoLaunchType(launchType workloadmeta.ECSLaunchType) (pb.ECSLaunchType, error) {
	switch launchType {
	case workloadmeta.ECSLaunchTypeEC2:
		return pb.ECSLaunchType_EC2, nil
	case workloadmeta.ECSLaunchTypeFargate:
		return pb.ECSLaunchType_FARGATE, nil
	}

	return pb.ECSLaunchType_EC2, fmt.Errorf("unknown launch type: %s", launchType)
}

// Conversions from protobuf to Workloadmeta types

// WorkloadmetaFilterFromProtoFilter converts the given protobuf filter into a workloadmeta.Filter
func WorkloadmetaFilterFromProtoFilter(protoFilter *pb.WorkloadmetaFilter) (*workloadmeta.Filter, error) {
	if protoFilter == nil {
		// Return filter that subscribes to everything
		return workloadmeta.NewFilter(nil, workloadmeta.SourceAll, workloadmeta.EventTypeAll), nil
	}

	var kinds []workloadmeta.Kind

	for _, protoKind := range protoFilter.Kinds {
		kind, err := toWorkloadmetaKind(protoKind)
		if err != nil {
			return nil, err
		}

		kinds = append(kinds, kind)
	}

	source, err := toWorkloadmetaSource(protoFilter.Source)
	if err != nil {
		return nil, err
	}

	eventType, err := toWorkloadmetaEventType(protoFilter.EventType)
	if err != nil {
		return nil, err
	}

	return workloadmeta.NewFilter(kinds, source, eventType), nil
}

// WorkloadmetaEventFromProtoEvent converts the given protobuf workloadmeta event into a workloadmeta.Event
func WorkloadmetaEventFromProtoEvent(protoEvent *pb.WorkloadmetaEvent) (workloadmeta.Event, error) {
	if protoEvent == nil {
		return workloadmeta.Event{}, nil
	}

	eventType, err := toWorkloadmetaEventType(protoEvent.Type)
	if err != nil {
		return workloadmeta.Event{}, err
	}

	if protoEvent.Container != nil {
		container, err := toWorkloadmetaContainer(protoEvent.Container)
		if err != nil {
			return workloadmeta.Event{}, err
		}

		return workloadmeta.Event{
			Type:   eventType,
			Entity: container,
		}, nil

	} else if protoEvent.KubernetesPod != nil {
		kubernetesPod, err := toWorkloadmetaKubernetesPod(protoEvent.KubernetesPod)
		if err != nil {
			return workloadmeta.Event{}, err
		}

		return workloadmeta.Event{
			Type:   eventType,
			Entity: kubernetesPod,
		}, nil
	} else if protoEvent.EcsTask != nil {
		ecsTask, err := toWorkloadmetaECSTask(protoEvent.EcsTask)
		if err != nil {
			return workloadmeta.Event{}, err
		}

		return workloadmeta.Event{
			Type:   eventType,
			Entity: ecsTask,
		}, nil
	}

	return workloadmeta.Event{}, fmt.Errorf("unknown entity")
}

func toWorkloadmetaKind(protoKind pb.WorkloadmetaKind) (workloadmeta.Kind, error) {
	switch protoKind {
	case pb.WorkloadmetaKind_CONTAINER:
		return workloadmeta.KindContainer, nil
	case pb.WorkloadmetaKind_KUBERNETES_POD:
		return workloadmeta.KindKubernetesPod, nil
	case pb.WorkloadmetaKind_ECS_TASK:
		return workloadmeta.KindECSTask, nil
	}

	return workloadmeta.KindContainer, fmt.Errorf("unknown kind: %s", protoKind)
}

func toWorkloadmetaSource(protoSource pb.WorkloadmetaSource) (workloadmeta.Source, error) {
	switch protoSource {
	case pb.WorkloadmetaSource_ALL:
		return workloadmeta.SourceAll, nil
	case pb.WorkloadmetaSource_RUNTIME:
		return workloadmeta.SourceRuntime, nil
	case pb.WorkloadmetaSource_NODE_ORCHESTRATOR:
		return workloadmeta.SourceNodeOrchestrator, nil
	case pb.WorkloadmetaSource_CLUSTER_ORCHESTRATOR:
		return workloadmeta.SourceClusterOrchestrator, nil
	}

	return workloadmeta.SourceAll, fmt.Errorf("unknown source: %s", protoSource)
}

func toWorkloadmetaEventType(protoEventType pb.WorkloadmetaEventType) (workloadmeta.EventType, error) {
	switch protoEventType {
	case pb.WorkloadmetaEventType_EVENT_TYPE_ALL:
		return workloadmeta.EventTypeAll, nil
	case pb.WorkloadmetaEventType_EVENT_TYPE_SET:
		return workloadmeta.EventTypeSet, nil
	case pb.WorkloadmetaEventType_EVENT_TYPE_UNSET:
		return workloadmeta.EventTypeUnset, nil
	}

	return workloadmeta.EventTypeAll, fmt.Errorf("unknown event type: %s", protoEventType)
}

func toWorkloadmetaContainer(protoContainer *pb.Container) (*workloadmeta.Container, error) {
	entityId, err := toWorkloadmetaEntityID(protoContainer.EntityId)
	if err != nil {
		return nil, err
	}

	runtime, err := toWorkloadmetaContainerRuntime(protoContainer.Runtime)
	if err != nil {
		return nil, err
	}

	var ports []workloadmeta.ContainerPort
	for _, port := range protoContainer.Ports {
		ports = append(ports, toWorkloadmetaContainerPort(port))
	}

	state, err := toWorkloadmetaContainerState(protoContainer.State)
	if err != nil {
		return nil, err
	}

	return &workloadmeta.Container{
		EntityID:      entityId,
		EntityMeta:    toWorkloadmetaEntityMeta(protoContainer.EntityMeta),
		EnvVars:       protoContainer.EnvVars,
		Hostname:      protoContainer.Hostname,
		Image:         toWorkloadmetaImage(protoContainer.Image),
		NetworkIPs:    protoContainer.NetworkIps,
		PID:           int(protoContainer.Pid),
		Ports:         ports,
		Runtime:       runtime,
		State:         state,
		CollectorTags: protoContainer.CollectorTags,
	}, nil
}

func toWorkloadmetaContainerPort(protoPort *pb.ContainerPort) workloadmeta.ContainerPort {
	return workloadmeta.ContainerPort{
		Name:     protoPort.Name,
		Port:     int(protoPort.Port),
		Protocol: protoPort.Protocol,
	}
}

func toWorkloadmetaEntityID(protoEntityID *pb.WorkloadmetaEntityId) (workloadmeta.EntityID, error) {
	kind, err := toWorkloadmetaKind(protoEntityID.Kind)
	if err != nil {
		return workloadmeta.EntityID{}, err
	}

	return workloadmeta.EntityID{
		Kind: kind,
		ID:   protoEntityID.Id,
	}, nil
}

func toWorkloadmetaEntityMeta(protoEntityMeta *pb.EntityMeta) workloadmeta.EntityMeta {
	return workloadmeta.EntityMeta{
		Name:        protoEntityMeta.Name,
		Namespace:   protoEntityMeta.Namespace,
		Annotations: protoEntityMeta.Annotations,
		Labels:      protoEntityMeta.Labels,
	}
}

func toWorkloadmetaImage(protoImage *pb.ContainerImage) workloadmeta.ContainerImage {
	return workloadmeta.ContainerImage{
		ID:        protoImage.Id,
		RawName:   protoImage.RawName,
		Name:      protoImage.Name,
		ShortName: protoImage.ShortName,
		Tag:       protoImage.Tag,
	}
}

func toWorkloadmetaContainerRuntime(protoRuntime pb.Runtime) (workloadmeta.ContainerRuntime, error) {
	switch protoRuntime {
	case pb.Runtime_DOCKER:
		return workloadmeta.ContainerRuntimeDocker, nil
	case pb.Runtime_CONTAINERD:
		return workloadmeta.ContainerRuntimeContainerd, nil
	case pb.Runtime_PODMAN:
		return workloadmeta.ContainerRuntimePodman, nil
	case pb.Runtime_CRIO:
		return workloadmeta.ContainerRuntimeCRIO, nil
	case pb.Runtime_GARDEN:
		return workloadmeta.ContainerRuntimeGarden, nil
	case pb.Runtime_ECS_FARGATE:
		return workloadmeta.ContainerRuntimeECSFargate, nil
	}

	return workloadmeta.ContainerRuntimeDocker, fmt.Errorf("unknown runtime: %s", protoRuntime)
}

func toWorkloadmetaContainerState(protoContainerState *pb.ContainerState) (workloadmeta.ContainerState, error) {
	status, err := toWorkloadmetaContainerStatus(protoContainerState.Status)
	if err != nil {
		return workloadmeta.ContainerState{}, err
	}

	health, err := toWorkloadmetaContainerHealth(protoContainerState.Health)
	if err != nil {
		return workloadmeta.ContainerState{}, err
	}

	containerState := workloadmeta.ContainerState{
		Running: protoContainerState.Running,
		Status:  status,
		Health:  health,
	}

	if protoContainerState.CreatedAt != emptyTimestampUnix {
		containerState.CreatedAt = time.Unix(protoContainerState.CreatedAt, 0)
	}

	if protoContainerState.StartedAt != emptyTimestampUnix {
		containerState.StartedAt = time.Unix(protoContainerState.StartedAt, 0)
	}

	if protoContainerState.FinishedAt != emptyTimestampUnix {
		containerState.FinishedAt = time.Unix(protoContainerState.FinishedAt, 0)
	}

	if protoContainerState.ExitCode != 0 {
		containerState.ExitCode = &protoContainerState.ExitCode
	}

	return containerState, nil
}

func toWorkloadmetaContainerStatus(protoContainerStatus pb.ContainerStatus) (workloadmeta.ContainerStatus, error) {
	switch protoContainerStatus {
	case pb.ContainerStatus_CONTAINER_STATUS_UNKNOWN:
		return workloadmeta.ContainerStatusUnknown, nil
	case pb.ContainerStatus_CONTAINER_STATUS_CREATED:
		return workloadmeta.ContainerStatusCreated, nil
	case pb.ContainerStatus_CONTAINER_STATUS_RUNNING:
		return workloadmeta.ContainerStatusRunning, nil
	case pb.ContainerStatus_CONTAINER_STATUS_RESTARTING:
		return workloadmeta.ContainerStatusRestarting, nil
	case pb.ContainerStatus_CONTAINER_STATUS_PAUSED:
		return workloadmeta.ContainerStatusPaused, nil
	case pb.ContainerStatus_CONTAINER_STATUS_STOPPED:
		return workloadmeta.ContainerStatusStopped, nil
	}

	return workloadmeta.ContainerStatusUnknown, fmt.Errorf("unknown container status: %s", protoContainerStatus)
}

func toWorkloadmetaContainerHealth(protoContainerHealth pb.ContainerHealth) (workloadmeta.ContainerHealth, error) {
	switch protoContainerHealth {
	case pb.ContainerHealth_CONTAINER_HEALTH_UNKNOWN:
		return workloadmeta.ContainerHealthUnknown, nil
	case pb.ContainerHealth_CONTAINER_HEALTH_HEALTHY:
		return workloadmeta.ContainerHealthHealthy, nil
	case pb.ContainerHealth_CONTAINER_HEALTH_UNHEALTHY:
		return workloadmeta.ContainerHealthUnhealthy, nil
	}

	return workloadmeta.ContainerHealthUnknown, fmt.Errorf("unknown container health: %s", protoContainerHealth)
}

func toWorkloadmetaKubernetesPod(protoKubernetesPod *pb.KubernetesPod) (*workloadmeta.KubernetesPod, error) {
	entityId, err := toWorkloadmetaEntityID(protoKubernetesPod.EntityId)
	if err != nil {
		return nil, err
	}

	var owners []workloadmeta.KubernetesPodOwner
	for _, protoPodOwner := range protoKubernetesPod.Owners {
		owners = append(owners, toWorkloadmetaPodOwner(protoPodOwner))
	}

	var containers []workloadmeta.OrchestratorContainer
	for _, protoContainer := range protoKubernetesPod.Containers {
		containers = append(containers, toWorkloadmetaOrchestratorContainer(protoContainer))
	}

	return &workloadmeta.KubernetesPod{
		EntityID:                   entityId,
		EntityMeta:                 toWorkloadmetaEntityMeta(protoKubernetesPod.EntityMeta),
		Owners:                     owners,
		PersistentVolumeClaimNames: protoKubernetesPod.PersistentVolumeClaimNames,
		Containers:                 containers,
		Ready:                      protoKubernetesPod.Ready,
		Phase:                      protoKubernetesPod.Phase,
		IP:                         protoKubernetesPod.Ip,
		PriorityClass:              protoKubernetesPod.PriorityClass,
		QOSClass:                   protoKubernetesPod.QosClass,
		KubeServices:               protoKubernetesPod.KubeServices,
		NamespaceLabels:            protoKubernetesPod.NamespaceLabels,
	}, nil
}

func toWorkloadmetaPodOwner(protoPodOwner *pb.KubernetesPodOwner) workloadmeta.KubernetesPodOwner {
	return workloadmeta.KubernetesPodOwner{
		Kind: protoPodOwner.Kind,
		Name: protoPodOwner.Name,
		ID:   protoPodOwner.Id,
	}
}

func toWorkloadmetaOrchestratorContainer(protoOrchestratorContainer *pb.OrchestratorContainer) workloadmeta.OrchestratorContainer {
	return workloadmeta.OrchestratorContainer{
		ID:    protoOrchestratorContainer.Id,
		Name:  protoOrchestratorContainer.Name,
		Image: toWorkloadmetaImage(protoOrchestratorContainer.Image),
	}
}

func toWorkloadmetaECSTask(protoECSTask *pb.ECSTask) (*workloadmeta.ECSTask, error) {
	entityId, err := toWorkloadmetaEntityID(protoECSTask.EntityId)
	if err != nil {
		return nil, err
	}

	launchType, err := toECSLaunchType(protoECSTask.LaunchType)
	if err != nil {
		return nil, err
	}

	var containers []workloadmeta.OrchestratorContainer
	for _, protoContainer := range protoECSTask.Containers {
		containers = append(containers, toWorkloadmetaOrchestratorContainer(protoContainer))
	}

	return &workloadmeta.ECSTask{
		EntityID:              entityId,
		EntityMeta:            toWorkloadmetaEntityMeta(protoECSTask.EntityMeta),
		Tags:                  protoECSTask.Tags,
		ContainerInstanceTags: protoECSTask.ContainerInstanceTags,
		ClusterName:           protoECSTask.ClusterName,
		Region:                protoECSTask.Region,
		AvailabilityZone:      protoECSTask.AvailabilityZone,
		Family:                protoECSTask.Family,
		Version:               protoECSTask.Version,
		LaunchType:            launchType,
		Containers:            containers,
	}, nil
}

func toECSLaunchType(protoLaunchType pb.ECSLaunchType) (workloadmeta.ECSLaunchType, error) {
	switch protoLaunchType {
	case pb.ECSLaunchType_EC2:
		return workloadmeta.ECSLaunchTypeEC2, nil
	case pb.ECSLaunchType_FARGATE:
		return workloadmeta.ECSLaunchTypeFargate, nil
	}

	return workloadmeta.ECSLaunchTypeEC2, fmt.Errorf("unknown launch type: %s", protoLaunchType)
}
