// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package proto provides conversions between Workloadmeta types and protobuf.
package proto

import (
	"errors"
	"fmt"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var emptyTimestampUnix = new(time.Time).Unix()

// Conversions from Workloadmeta types to protobuf

// ProtobufEventFromWorkloadmetaEvent converts the given workloadmeta.Event into
// protobuf.
//
// Note: the IsComplete field is intentionally not included in this conversion.
// Users of remote workloadmeta don't need completeness info. Also, the field is
// not directly usable on the receiving side without further changes: the
// receiving store has only one source (SourceRemoteWorkloadmeta), so its
// completeness calculation based on expectedSources is not the same as in the
// core agent.
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
	case workloadmeta.KindContainerImageMetadata:
		containerImageMetadata := entity.(*workloadmeta.ContainerImageMetadata)

		protoContainerImageMetadata, err := protoContainerImageMetadataFromWorkloadmetaContainerImageMetadata(containerImageMetadata)
		if err != nil {
			return nil, err
		}

		return &pb.WorkloadmetaEvent{
			Type:                   protoEventType,
			ContainerImageMetadata: protoContainerImageMetadata,
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
	case workloadmeta.KindProcess:
		process := entity.(*workloadmeta.Process)
		protoProcess, err := protoProcessFromWorkloadmetaProcess(process)
		if err != nil {
			return nil, err
		}
		return &pb.WorkloadmetaEvent{
			Type:    protoEventType,
			Process: protoProcess,
		}, nil
	case workloadmeta.KindCRD:
		crd := entity.(*workloadmeta.CRD)
		protoCrd, err := protoCRDFromWorkloadmetaCRD(crd)
		if err != nil {
			return nil, err
		}

		return &pb.WorkloadmetaEvent{
			Type: protoEventType,
			Crd:  protoCrd,
		}, nil
	}

	// We have not defined a conversion for the workloadmeta type included in
	// the given event.
	// This is not considered to be an error because we only support some
	// types. The list is defined in the remote workloadmeta collector.
	return nil, nil
}

// ProtobufFilterFromWorkloadmetaFilter converts the given workloadmeta.Filter into protobuf
func ProtobufFilterFromWorkloadmetaFilter(filter *workloadmeta.Filter) (*pb.WorkloadmetaFilter, error) {
	if filter == nil {
		return nil, nil
	}

	kinds := filter.Kinds()
	protoKinds := make([]pb.WorkloadmetaKind, 0, len(kinds))
	for _, kind := range kinds {
		protoKind, err := toProtoKind(kind)
		if err != nil {
			return nil, err
		}

		protoKinds = append(protoKinds, protoKind)
	}

	protoSource, err := toProtoSource(filter.Source())
	if err != nil {
		return nil, err
	}

	protoEventType, err := toProtoEventType(filter.EventType())
	if err != nil {
		return nil, err
	}

	return &pb.WorkloadmetaFilter{
		Kinds:     protoKinds,
		Source:    protoSource,
		EventType: protoEventType,
	}, nil
}

func protoContainerFromWorkloadmetaContainer(container *workloadmeta.Container) (*pb.Container, error) {
	var pbContainerPorts []*pb.ContainerPort
	for _, port := range container.Ports {
		pbContainerPorts = append(pbContainerPorts, toProtoContainerPort(&port))
	}

	protoEntityID, err := toProtoEntityIDFromContainer(container)
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

	protoResolvedAllocatedResources := toProtoResolvedAllocatedResources(container.ResolvedAllocatedResources)

	var ownerEntityID *pb.WorkloadmetaEntityId
	if container.Owner != nil {
		kind, err := toProtoKind(container.Owner.Kind)
		if err != nil {
			return nil, err
		}
		ownerEntityID = &pb.WorkloadmetaEntityId{
			Kind: kind,
			Id:   container.Owner.ID,
		}
	}

	return &pb.Container{
		EntityId:                   protoEntityID,
		EntityMeta:                 toProtoEntityMetaFromContainer(container),
		EnvVars:                    container.EnvVars,
		Hostname:                   container.Hostname,
		Image:                      toProtoImage(&container.Image),
		NetworkIps:                 container.NetworkIPs,
		Pid:                        int32(container.PID),
		Ports:                      pbContainerPorts,
		Runtime:                    protoRuntime,
		State:                      protoContainerState,
		CollectorTags:              container.CollectorTags,
		CgroupPath:                 container.CgroupPath,
		ResolvedAllocatedResources: protoResolvedAllocatedResources,
		Resources:                  toProtoContainerResources(container.Resources),
		Owner:                      ownerEntityID,
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

func toProtoSource(source workloadmeta.Source) (pb.WorkloadmetaSource, error) {
	switch source {
	case workloadmeta.SourceAll:
		return pb.WorkloadmetaSource_ALL, nil
	case workloadmeta.SourceRuntime:
		return pb.WorkloadmetaSource_RUNTIME, nil
	case workloadmeta.SourceNodeOrchestrator:
		return pb.WorkloadmetaSource_NODE_ORCHESTRATOR, nil
	case workloadmeta.SourceClusterOrchestrator:
		return pb.WorkloadmetaSource_CLUSTER_ORCHESTRATOR, nil
	}

	return pb.WorkloadmetaSource_ALL, fmt.Errorf("unknown source: %s", source)
}

func toProtoEntityIDFromContainer(container *workloadmeta.Container) (*pb.WorkloadmetaEntityId, error) {
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
	case workloadmeta.KindContainerImageMetadata:
		return pb.WorkloadmetaKind_CONTAINER_IMAGE_METADATA, nil
	case workloadmeta.KindKubernetesPod:
		return pb.WorkloadmetaKind_KUBERNETES_POD, nil
	case workloadmeta.KindECSTask:
		return pb.WorkloadmetaKind_ECS_TASK, nil
	case workloadmeta.KindProcess:
		return pb.WorkloadmetaKind_PROCESS, nil
	case workloadmeta.KindCRD:
		return pb.WorkloadmetaKind_CRD, nil
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
		Id:         image.ID,
		RawName:    image.RawName,
		Name:       image.Name,
		ShortName:  image.ShortName,
		Tag:        image.Tag,
		RepoDigest: image.RepoDigest,
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
	case "":
		// we need to handle "" because we don't enforce populating this property by collectors
		return pb.Runtime_UNKNOWN, nil
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

	return pb.Runtime_DOCKER, fmt.Errorf("unknown runtime: %q", runtime)
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

func toProtoResolvedAllocatedResources(resources []workloadmeta.ContainerAllocatedResource) []*pb.ContainerAllocatedResource {
	var protoResolvedAllocatedResources []*pb.ContainerAllocatedResource
	for _, resource := range resources {
		protoResolvedAllocatedResources = append(protoResolvedAllocatedResources, &pb.ContainerAllocatedResource{
			Name: resource.Name,
			ID:   resource.ID,
		})
	}

	return protoResolvedAllocatedResources
}

func toProtoContainerResources(resources workloadmeta.ContainerResources) *pb.ContainerResources {
	if resources.CPURequest == nil &&
		resources.CPULimit == nil &&
		resources.MemoryRequest == nil &&
		resources.MemoryLimit == nil {
		return nil
	}

	return &pb.ContainerResources{
		CpuRequest:    resources.CPURequest,
		CpuLimit:      resources.CPULimit,
		MemoryRequest: resources.MemoryRequest,
		MemoryLimit:   resources.MemoryLimit,
	}
}

func toProtoContainerStatus(status workloadmeta.ContainerStatus) (pb.ContainerStatus, error) {
	switch status {
	case "", workloadmeta.ContainerStatusUnknown:
		// we need to handle "" because we don't enforce populating this property by collectors
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

	return pb.ContainerStatus_CONTAINER_STATUS_UNKNOWN, fmt.Errorf("unknown status: %q", status)
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

func protoContainerImageMetadataFromWorkloadmetaContainerImageMetadata(containerImageMetadata *workloadmeta.ContainerImageMetadata) (*pb.ContainerImageMetadata, error) {
	protoEntityID, err := toProtoEntityID(&containerImageMetadata.EntityID)
	if err != nil {
		return nil, err
	}

	var protoLayers []*pb.ContainerImageLayer
	for _, layer := range containerImageMetadata.Layers {
		protoLayers = append(protoLayers, &pb.ContainerImageLayer{
			MediaType: layer.MediaType,
			Digest:    layer.Digest,
			SizeBytes: layer.SizeBytes,
			Urls:      layer.URLs,
		})
	}

	return &pb.ContainerImageMetadata{
		EntityId:     protoEntityID,
		EntityMeta:   toProtoEntityMetaFromContainerImageMetadata(containerImageMetadata),
		RepoTags:     containerImageMetadata.RepoTags,
		RepoDigests:  containerImageMetadata.RepoDigests,
		MediaType:    containerImageMetadata.MediaType,
		SizeBytes:    containerImageMetadata.SizeBytes,
		Os:           containerImageMetadata.OS,
		OsVersion:    containerImageMetadata.OSVersion,
		Architecture: containerImageMetadata.Architecture,
		Variant:      containerImageMetadata.Variant,
		Layers:       protoLayers,
		Sbom: &pb.CompressedSBOM{
			Bom:                containerImageMetadata.SBOM.Bom,
			GenerationTime:     timestamppb.New(containerImageMetadata.SBOM.GenerationTime),
			GenerationDuration: uint64(containerImageMetadata.SBOM.GenerationDuration),
			GenerationMethod:   containerImageMetadata.SBOM.GenerationMethod,
			Status:             string(containerImageMetadata.SBOM.Status),
			Error:              containerImageMetadata.SBOM.Error,
		},
	}, nil
}

func toProtoEntityMetaFromContainerImageMetadata(containerImageMetadata *workloadmeta.ContainerImageMetadata) *pb.EntityMeta {
	return &pb.EntityMeta{
		Name:        containerImageMetadata.Name,
		Namespace:   containerImageMetadata.Namespace,
		Annotations: containerImageMetadata.Annotations,
		Labels:      containerImageMetadata.Labels,
	}
}

func protoKubernetesPodFromWorkloadmetaKubernetesPod(kubernetesPod *workloadmeta.KubernetesPod) (*pb.KubernetesPod, error) {
	protoEntityID, err := toProtoEntityIDFromKubernetesPod(kubernetesPod)
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

	var protoEphemeralContainers []*pb.OrchestratorContainer
	for _, container := range kubernetesPod.EphemeralContainers {
		protoEphemeralContainers = append(protoEphemeralContainers, toProtoOrchestratorContainer(container))
	}

	return &pb.KubernetesPod{
		EntityId:                   protoEntityID,
		EntityMeta:                 toProtoEntityMetaFromKubernetesPod(kubernetesPod),
		Owners:                     protoKubernetesPodOwners,
		PersistentVolumeClaimNames: kubernetesPod.PersistentVolumeClaimNames,
		InitContainers:             protoInitContainers,
		Containers:                 protoOrchestratorContainers,
		EphemeralContainers:        protoEphemeralContainers,
		Ready:                      kubernetesPod.Ready,
		Phase:                      kubernetesPod.Phase,
		Ip:                         kubernetesPod.IP,
		PriorityClass:              kubernetesPod.PriorityClass,
		QosClass:                   kubernetesPod.QOSClass,
		RuntimeClass:               kubernetesPod.RuntimeClass,
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

func toProtoEntityIDFromKubernetesPod(kubernetesPod *workloadmeta.KubernetesPod) (*pb.WorkloadmetaEntityId, error) {
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
	protoEntityID, err := toProtoEntityIDFromECSTask(ecsTask)
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
		AwsAccountID:          ecsTask.AWSAccountID,
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

func toProtoEntityIDFromECSTask(ecsTask *workloadmeta.ECSTask) (*pb.WorkloadmetaEntityId, error) {
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

func protoProcessFromWorkloadmetaProcess(process *workloadmeta.Process) (*pb.Process, error) {
	protoEntityID, err := toProtoEntityID(&process.EntityID)
	if err != nil {
		return nil, err
	}

	var protoOwner *pb.WorkloadmetaEntityId
	if process.Owner != nil {
		protoOwner, err = toProtoEntityID(process.Owner)
		if err != nil {
			return nil, err
		}
	}

	return &pb.Process{
		EntityId:       protoEntityID,
		Pid:            process.Pid,
		Nspid:          process.NsPid,
		Ppid:           process.Ppid,
		Name:           process.Name,
		Cwd:            process.Cwd,
		Exe:            process.Exe,
		Comm:           process.Comm,
		Cmdline:        process.Cmdline,
		Uids:           process.Uids,
		Gids:           process.Gids,
		ContainerId:    process.ContainerID,
		CreationTime:   process.CreationTime.Unix(),
		Language:       toProtoLanguage(process.Language),
		Owner:          protoOwner,
		Service:        toProtoService(process.Service),
		InjectionState: pb.InjectionState(process.InjectionState),
	}, nil
}

func toProtoEntityMetaFromCrd(crd *workloadmeta.CRD) *pb.EntityMeta {
	if crd == nil {
		return nil
	}

	return &pb.EntityMeta{
		Name:        crd.EntityMeta.Name,
		Namespace:   crd.EntityMeta.Namespace,
		Annotations: crd.EntityMeta.Annotations,
		Labels:      crd.EntityMeta.Labels,
	}
}

func protoCRDFromWorkloadmetaCRD(crd *workloadmeta.CRD) (*pb.Crd, error) {
	protoEntityID, err := toProtoEntityID(&crd.EntityID)
	if err != nil {
		return nil, err
	}

	return &pb.Crd{
		EnityId:    protoEntityID,
		EntityMeta: toProtoEntityMetaFromCrd(crd),
		Group:      crd.Group,
		Kind:       crd.Kind,
		Version:    crd.Version,
	}, nil
}

func toProtoEntityID(entityID *workloadmeta.EntityID) (*pb.WorkloadmetaEntityId, error) {
	if entityID == nil {
		return nil, nil
	}

	protoKind, err := toProtoKind(entityID.Kind)
	if err != nil {
		return nil, err
	}

	return &pb.WorkloadmetaEntityId{
		Kind: protoKind,
		Id:   entityID.ID,
	}, nil
}

func toProtoLanguage(language *languagemodels.Language) *pb.Language {
	if language == nil {
		return nil
	}

	return &pb.Language{
		Name:    string(language.Name),
		Version: language.Version,
	}
}

func toProtoTracerMetadata(tracerMetadata tracermetadata.TracerMetadata) *pb.TracerMetadata {
	return &pb.TracerMetadata{
		RuntimeId:   tracerMetadata.RuntimeID,
		ServiceName: tracerMetadata.ServiceName,
	}
}

func toProtoUST(ust workloadmeta.UST) *pb.UST {
	if ust.Service == "" && ust.Env == "" && ust.Version == "" {
		return nil
	}

	return &pb.UST{
		Service: ust.Service,
		Env:     ust.Env,
		Version: ust.Version,
	}
}

func toProtoService(service *workloadmeta.Service) *pb.Service {
	if service == nil {
		return nil
	}

	var protoTracerMetadata []*pb.TracerMetadata
	for _, tracerMeta := range service.TracerMetadata {
		protoTracerMetadata = append(protoTracerMetadata, toProtoTracerMetadata(tracerMeta))
	}

	var tcpPorts []int32
	for _, port := range service.TCPPorts {
		tcpPorts = append(tcpPorts, int32(port))
	}

	var udpPorts []int32
	for _, port := range service.UDPPorts {
		udpPorts = append(udpPorts, int32(port))
	}

	return &pb.Service{
		GeneratedName:            service.GeneratedName,
		GeneratedNameSource:      service.GeneratedNameSource,
		AdditionalGeneratedNames: service.AdditionalGeneratedNames,
		TracerMetadata:           protoTracerMetadata,
		TcpPorts:                 tcpPorts,
		UdpPorts:                 udpPorts,
		ApmInstrumentation:       service.APMInstrumentation,
		Ust:                      toProtoUST(service.UST),
	}
}

// Conversions from protobuf to Workloadmeta types

// WorkloadmetaFilterFromProtoFilter converts the given protobuf filter into a workloadmeta.Filter
func WorkloadmetaFilterFromProtoFilter(protoFilter *pb.WorkloadmetaFilter) (*workloadmeta.Filter, error) {
	if protoFilter == nil {
		// Return filter that subscribes to everything
		return workloadmeta.NewFilterBuilder().Build(), nil
	}

	filterBuilder := workloadmeta.NewFilterBuilder()

	for _, protoKind := range protoFilter.Kinds {
		kind, err := toWorkloadmetaKind(protoKind)
		if err != nil {
			return nil, err
		}
		filterBuilder = filterBuilder.AddKind(kind)
	}

	source, err := toWorkloadmetaSource(protoFilter.Source)
	if err != nil {
		return nil, err
	}

	eventType, err := toWorkloadmetaEventType(protoFilter.EventType)
	if err != nil {
		return nil, err
	}

	filter := filterBuilder.
		SetEventType(eventType).
		SetSource(source).Build()

	return filter, nil
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
	} else if protoEvent.Process != nil {
		process, err := toWorkloadmetaProcess(protoEvent.Process)
		if err != nil {
			return workloadmeta.Event{}, err
		}
		return workloadmeta.Event{
			Type:   eventType,
			Entity: process,
		}, nil
	} else if protoEvent.Crd != nil {
		crd, err := toWorkloadmetaCrd(protoEvent.Crd)
		if err != nil {
			return workloadmeta.Event{}, err
		}
		return workloadmeta.Event{
			Type:   eventType,
			Entity: crd,
		}, nil
	} else if protoEvent.ContainerImageMetadata != nil {
		containerImageMetadata, err := toWorkloadmetaContainerImageMetadata(protoEvent.ContainerImageMetadata)
		if err != nil {
			return workloadmeta.Event{}, err
		}
		return workloadmeta.Event{
			Type:   eventType,
			Entity: containerImageMetadata,
		}, nil
	}

	return workloadmeta.Event{}, errors.New("unknown entity")
}

func toWorkloadmetaKind(protoKind pb.WorkloadmetaKind) (workloadmeta.Kind, error) {
	switch protoKind {
	case pb.WorkloadmetaKind_CONTAINER:
		return workloadmeta.KindContainer, nil
	case pb.WorkloadmetaKind_CONTAINER_IMAGE_METADATA:
		return workloadmeta.KindContainerImageMetadata, nil
	case pb.WorkloadmetaKind_KUBERNETES_POD:
		return workloadmeta.KindKubernetesPod, nil
	case pb.WorkloadmetaKind_ECS_TASK:
		return workloadmeta.KindECSTask, nil
	case pb.WorkloadmetaKind_PROCESS:
		return workloadmeta.KindProcess, nil
	case pb.WorkloadmetaKind_CRD:
		return workloadmeta.KindCRD, nil
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
	entityID, err := toWorkloadmetaEntityID(protoContainer.EntityId)
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

	resources := toWorkloadmetaResolvedAllocatedResources(protoContainer.ResolvedAllocatedResources)

	var owner *workloadmeta.EntityID
	if protoContainer.Owner != nil {
		ownerEntityID, err := toWorkloadmetaEntityID(protoContainer.Owner)
		if err != nil {
			return nil, err
		}
		owner = &ownerEntityID
	}

	return &workloadmeta.Container{
		EntityID:                   entityID,
		EntityMeta:                 toWorkloadmetaEntityMeta(protoContainer.EntityMeta),
		EnvVars:                    protoContainer.EnvVars,
		Hostname:                   protoContainer.Hostname,
		Image:                      toWorkloadmetaImage(protoContainer.Image),
		NetworkIPs:                 protoContainer.NetworkIps,
		PID:                        int(protoContainer.Pid),
		Ports:                      ports,
		Runtime:                    runtime,
		State:                      state,
		CollectorTags:              protoContainer.CollectorTags,
		CgroupPath:                 protoContainer.CgroupPath,
		ResolvedAllocatedResources: resources,
		Resources:                  toWorkloadmetaContainerResources(protoContainer.Resources),
		Owner:                      owner,
	}, nil
}

func toWorkloadmetaContainerPort(protoPort *pb.ContainerPort) workloadmeta.ContainerPort {
	return workloadmeta.ContainerPort{
		Name:     protoPort.Name,
		Port:     int(protoPort.Port),
		Protocol: protoPort.Protocol,
	}
}

func toWorkloadmetaResolvedAllocatedResources(protoResolvedAllocatedResources []*pb.ContainerAllocatedResource) []workloadmeta.ContainerAllocatedResource {
	var resources []workloadmeta.ContainerAllocatedResource
	for _, protoResource := range protoResolvedAllocatedResources {
		resources = append(resources, workloadmeta.ContainerAllocatedResource{
			Name: protoResource.Name,
			ID:   protoResource.ID,
		})
	}

	return resources
}

func toWorkloadmetaContainerResources(protoResources *pb.ContainerResources) workloadmeta.ContainerResources {
	if protoResources == nil {
		return workloadmeta.ContainerResources{}
	}

	return workloadmeta.ContainerResources{
		CPURequest:    protoResources.CpuRequest,
		CPULimit:      protoResources.CpuLimit,
		MemoryRequest: protoResources.MemoryRequest,
		MemoryLimit:   protoResources.MemoryLimit,
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
		ID:         protoImage.Id,
		RawName:    protoImage.RawName,
		Name:       protoImage.Name,
		ShortName:  protoImage.ShortName,
		Tag:        protoImage.Tag,
		RepoDigest: protoImage.RepoDigest,
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
	case pb.Runtime_UNKNOWN:
		return "", nil
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
	entityID, err := toWorkloadmetaEntityID(protoKubernetesPod.EntityId)
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

	var ephemeralContainers []workloadmeta.OrchestratorContainer
	for _, protoContainer := range protoKubernetesPod.EphemeralContainers {
		ephemeralContainers = append(ephemeralContainers, toWorkloadmetaOrchestratorContainer(protoContainer))
	}

	return &workloadmeta.KubernetesPod{
		EntityID:                   entityID,
		EntityMeta:                 toWorkloadmetaEntityMeta(protoKubernetesPod.EntityMeta),
		Owners:                     owners,
		PersistentVolumeClaimNames: protoKubernetesPod.PersistentVolumeClaimNames,
		Containers:                 containers,
		EphemeralContainers:        ephemeralContainers,
		Ready:                      protoKubernetesPod.Ready,
		Phase:                      protoKubernetesPod.Phase,
		IP:                         protoKubernetesPod.Ip,
		PriorityClass:              protoKubernetesPod.PriorityClass,
		QOSClass:                   protoKubernetesPod.QosClass,
		RuntimeClass:               protoKubernetesPod.RuntimeClass,
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
	entityID, err := toWorkloadmetaEntityID(protoECSTask.EntityId)
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
		EntityID:              entityID,
		EntityMeta:            toWorkloadmetaEntityMeta(protoECSTask.EntityMeta),
		Tags:                  protoECSTask.Tags,
		ContainerInstanceTags: protoECSTask.ContainerInstanceTags,
		ClusterName:           protoECSTask.ClusterName,
		Region:                protoECSTask.Region,
		AWSAccountID:          protoECSTask.AwsAccountID,
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

func toWorkloadmetaProcess(protoProcess *pb.Process) (*workloadmeta.Process, error) {
	entityID, err := toWorkloadmetaEntityID(protoProcess.EntityId)
	if err != nil {
		return nil, err
	}

	var owner *workloadmeta.EntityID
	if protoProcess.Owner != nil {
		ownerEntityID, err := toWorkloadmetaEntityID(protoProcess.Owner)
		if err != nil {
			return nil, err
		}
		owner = &ownerEntityID
	}

	var creationTime time.Time
	if protoProcess.CreationTime != emptyTimestampUnix {
		creationTime = time.Unix(protoProcess.CreationTime, 0)
	}

	return &workloadmeta.Process{
		EntityID:       entityID,
		Pid:            protoProcess.Pid,
		NsPid:          protoProcess.Nspid,
		Ppid:           protoProcess.Ppid,
		Name:           protoProcess.Name,
		Cwd:            protoProcess.Cwd,
		Exe:            protoProcess.Exe,
		Comm:           protoProcess.Comm,
		Cmdline:        protoProcess.Cmdline,
		Uids:           protoProcess.Uids,
		Gids:           protoProcess.Gids,
		ContainerID:    protoProcess.ContainerId,
		CreationTime:   creationTime,
		Language:       toWorkloadmetaLanguage(protoProcess.Language),
		Owner:          owner,
		Service:        toWorkloadmetaService(protoProcess.Service),
		InjectionState: workloadmeta.InjectionState(protoProcess.InjectionState),
	}, nil
}

func toWorkloadmetaContainerImageMetadata(protoContainerImageMetadata *pb.ContainerImageMetadata) (*workloadmeta.ContainerImageMetadata, error) {
	entityID, err := toWorkloadmetaEntityID(protoContainerImageMetadata.EntityId)
	if err != nil {
		return nil, err
	}

	var layers []workloadmeta.ContainerImageLayer
	for _, protoLayer := range protoContainerImageMetadata.Layers {
		layers = append(layers, workloadmeta.ContainerImageLayer{
			MediaType: protoLayer.MediaType,
			Digest:    protoLayer.Digest,
			SizeBytes: protoLayer.SizeBytes,
			URLs:      protoLayer.Urls,
		})
	}

	return &workloadmeta.ContainerImageMetadata{
		EntityID:     entityID,
		EntityMeta:   toWorkloadmetaEntityMeta(protoContainerImageMetadata.EntityMeta),
		RepoTags:     protoContainerImageMetadata.RepoTags,
		RepoDigests:  protoContainerImageMetadata.RepoDigests,
		MediaType:    protoContainerImageMetadata.MediaType,
		SizeBytes:    protoContainerImageMetadata.SizeBytes,
		OS:           protoContainerImageMetadata.Os,
		OSVersion:    protoContainerImageMetadata.OsVersion,
		Architecture: protoContainerImageMetadata.Architecture,
		Variant:      protoContainerImageMetadata.Variant,
		Layers:       layers,
		SBOM: &workloadmeta.CompressedSBOM{
			Bom:                protoContainerImageMetadata.Sbom.Bom,
			GenerationTime:     protoContainerImageMetadata.Sbom.GenerationTime.AsTime(),
			GenerationDuration: time.Duration(protoContainerImageMetadata.Sbom.GenerationDuration),
			GenerationMethod:   protoContainerImageMetadata.Sbom.GenerationMethod,
			Status:             workloadmeta.SBOMStatus(protoContainerImageMetadata.Sbom.Status),
			Error:              protoContainerImageMetadata.Sbom.Error,
		},
	}, nil
}

func toWorkloadmetaCrd(protoCrd *pb.Crd) (*workloadmeta.CRD, error) {
	entityID, err := toWorkloadmetaEntityID(protoCrd.EnityId)
	if err != nil {
		return nil, err
	}

	return &workloadmeta.CRD{
		EntityID:   entityID,
		EntityMeta: toWorkloadmetaEntityMeta(protoCrd.EntityMeta),
		Group:      protoCrd.Group,
		Kind:       protoCrd.Kind,
		Version:    protoCrd.Version,
	}, nil
}

func toWorkloadmetaLanguage(protoLanguage *pb.Language) *languagemodels.Language {
	if protoLanguage == nil {
		return nil
	}

	return &languagemodels.Language{
		Name:    languagemodels.LanguageName(protoLanguage.Name),
		Version: protoLanguage.Version,
	}
}

func toWorkloadmetaTracerMetadata(protoTracerMetadata *pb.TracerMetadata) tracermetadata.TracerMetadata {
	return tracermetadata.TracerMetadata{
		RuntimeID:   protoTracerMetadata.RuntimeId,
		ServiceName: protoTracerMetadata.ServiceName,
	}
}

func toWorkloadmetaUST(protoUST *pb.UST) workloadmeta.UST {
	if protoUST == nil {
		return workloadmeta.UST{}
	}

	return workloadmeta.UST{
		Service: protoUST.Service,
		Env:     protoUST.Env,
		Version: protoUST.Version,
	}
}

func toWorkloadmetaService(protoService *pb.Service) *workloadmeta.Service {
	if protoService == nil {
		return nil
	}

	var tracerMetadata []tracermetadata.TracerMetadata
	for _, protoTracerMeta := range protoService.TracerMetadata {
		tracerMetadata = append(tracerMetadata, toWorkloadmetaTracerMetadata(protoTracerMeta))
	}

	var tcpPorts []uint16
	for _, port := range protoService.TcpPorts {
		tcpPorts = append(tcpPorts, uint16(port))
	}

	var udpPorts []uint16
	for _, port := range protoService.UdpPorts {
		udpPorts = append(udpPorts, uint16(port))
	}

	return &workloadmeta.Service{
		GeneratedName:            protoService.GeneratedName,
		GeneratedNameSource:      protoService.GeneratedNameSource,
		AdditionalGeneratedNames: protoService.AdditionalGeneratedNames,
		TracerMetadata:           tracerMetadata,
		TCPPorts:                 tcpPorts,
		UDPPorts:                 udpPorts,
		APMInstrumentation:       protoService.ApmInstrumentation,
		UST:                      toWorkloadmetaUST(protoService.Ust),
	}
}
