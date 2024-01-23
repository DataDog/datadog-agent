// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package proto

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

var emptyTimestampUnix = new(time.Time).Unix()

// Conversions from Workloadmeta types to protobuf

// ProtobufEventFromWorkloadmetaEvent converts the given workloadmeta.Event into protobuf
func ProtobufEventFromWorkloadmetaEvent(event workloadmeta.Event) (*pb.WorkloadmetaEvent, error) {
	panic("not called")
}

func protoContainerFromWorkloadmetaContainer(container *workloadmeta.Container) (*pb.Container, error) {
	panic("not called")
}

func toProtoEventType(eventType workloadmeta.EventType) (pb.WorkloadmetaEventType, error) {
	panic("not called")
}

func toProtoEntityIDFromContainer(container *workloadmeta.Container) (*pb.WorkloadmetaEntityId, error) {
	panic("not called")
}

func toProtoKind(kind workloadmeta.Kind) (pb.WorkloadmetaKind, error) {
	panic("not called")
}

func toProtoEntityMetaFromContainer(container *workloadmeta.Container) *pb.EntityMeta {
	panic("not called")
}

func toProtoImage(image *workloadmeta.ContainerImage) *pb.ContainerImage {
	panic("not called")
}

func toProtoContainerPort(port *workloadmeta.ContainerPort) *pb.ContainerPort {
	panic("not called")
}

func toProtoRuntime(runtime workloadmeta.ContainerRuntime) (pb.Runtime, error) {
	panic("not called")
}

func toProtoContainerState(state *workloadmeta.ContainerState) (*pb.ContainerState, error) {
	panic("not called")
}

func toProtoContainerStatus(status workloadmeta.ContainerStatus) (pb.ContainerStatus, error) {
	panic("not called")
}

func toProtoContainerHealth(health workloadmeta.ContainerHealth) (pb.ContainerHealth, error) {
	panic("not called")
}

func protoKubernetesPodFromWorkloadmetaKubernetesPod(kubernetesPod *workloadmeta.KubernetesPod) (*pb.KubernetesPod, error) {
	panic("not called")
}

func toProtoEntityMetaFromKubernetesPod(kubernetesPod *workloadmeta.KubernetesPod) *pb.EntityMeta {
	panic("not called")
}

func toProtoEntityIDFromKubernetesPod(kubernetesPod *workloadmeta.KubernetesPod) (*pb.WorkloadmetaEntityId, error) {
	panic("not called")
}

func toProtoKubernetesPodOwner(kubernetesPodOwner *workloadmeta.KubernetesPodOwner) *pb.KubernetesPodOwner {
	panic("not called")
}

func toProtoOrchestratorContainer(container workloadmeta.OrchestratorContainer) *pb.OrchestratorContainer {
	panic("not called")
}

func protoECSTaskFromWorkloadmetaECSTask(ecsTask *workloadmeta.ECSTask) (*pb.ECSTask, error) {
	panic("not called")
}

func toProtoEntityMetaFromECSTask(ecsTask *workloadmeta.ECSTask) *pb.EntityMeta {
	panic("not called")
}

func toProtoEntityIDFromECSTask(ecsTask *workloadmeta.ECSTask) (*pb.WorkloadmetaEntityId, error) {
	panic("not called")
}

func toProtoLaunchType(launchType workloadmeta.ECSLaunchType) (pb.ECSLaunchType, error) {
	panic("not called")
}

// Conversions from protobuf to Workloadmeta types

// WorkloadmetaFilterFromProtoFilter converts the given protobuf filter into a workloadmeta.Filter
func WorkloadmetaFilterFromProtoFilter(protoFilter *pb.WorkloadmetaFilter) (*workloadmeta.Filter, error) {
	panic("not called")
}

// WorkloadmetaEventFromProtoEvent converts the given protobuf workloadmeta event into a workloadmeta.Event
func WorkloadmetaEventFromProtoEvent(protoEvent *pb.WorkloadmetaEvent) (workloadmeta.Event, error) {
	panic("not called")
}

func toWorkloadmetaKind(protoKind pb.WorkloadmetaKind) (workloadmeta.Kind, error) {
	panic("not called")
}

func toWorkloadmetaSource(protoSource pb.WorkloadmetaSource) (workloadmeta.Source, error) {
	panic("not called")
}

func toWorkloadmetaEventType(protoEventType pb.WorkloadmetaEventType) (workloadmeta.EventType, error) {
	panic("not called")
}

func toWorkloadmetaContainer(protoContainer *pb.Container) (*workloadmeta.Container, error) {
	panic("not called")
}

func toWorkloadmetaContainerPort(protoPort *pb.ContainerPort) workloadmeta.ContainerPort {
	panic("not called")
}

func toWorkloadmetaEntityID(protoEntityID *pb.WorkloadmetaEntityId) (workloadmeta.EntityID, error) {
	panic("not called")
}

func toWorkloadmetaEntityMeta(protoEntityMeta *pb.EntityMeta) workloadmeta.EntityMeta {
	panic("not called")
}

func toWorkloadmetaImage(protoImage *pb.ContainerImage) workloadmeta.ContainerImage {
	panic("not called")
}

func toWorkloadmetaContainerRuntime(protoRuntime pb.Runtime) (workloadmeta.ContainerRuntime, error) {
	panic("not called")
}

func toWorkloadmetaContainerState(protoContainerState *pb.ContainerState) (workloadmeta.ContainerState, error) {
	panic("not called")
}

func toWorkloadmetaContainerStatus(protoContainerStatus pb.ContainerStatus) (workloadmeta.ContainerStatus, error) {
	panic("not called")
}

func toWorkloadmetaContainerHealth(protoContainerHealth pb.ContainerHealth) (workloadmeta.ContainerHealth, error) {
	panic("not called")
}

func toWorkloadmetaKubernetesPod(protoKubernetesPod *pb.KubernetesPod) (*workloadmeta.KubernetesPod, error) {
	panic("not called")
}

func toWorkloadmetaPodOwner(protoPodOwner *pb.KubernetesPodOwner) workloadmeta.KubernetesPodOwner {
	panic("not called")
}

func toWorkloadmetaOrchestratorContainer(protoOrchestratorContainer *pb.OrchestratorContainer) workloadmeta.OrchestratorContainer {
	panic("not called")
}

func toWorkloadmetaECSTask(protoECSTask *pb.ECSTask) (*workloadmeta.ECSTask, error) {
	panic("not called")
}

func toECSLaunchType(protoLaunchType pb.ECSLaunchType) (workloadmeta.ECSLaunchType, error) {
	panic("not called")
}
