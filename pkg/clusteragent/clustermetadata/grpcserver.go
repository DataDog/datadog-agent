// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"context"

	"google.golang.org/grpc"

	cm "github.com/DataDog/datadog-agent/pkg/clustermetadata"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// PeerServer implements the generated ClusterMetadata gRPC service.
type PeerServer struct {
	store *LocalStore
	pb.UnimplementedClusterMetadataServer
}

func NewPeerServer(store *LocalStore) *PeerServer {
	return &PeerServer{store: store}
}

func (s *PeerServer) Register(server *grpc.Server) {
	pb.RegisterClusterMetadataServer(server, s)
}

func (s *PeerServer) Query(ctx context.Context, req *pb.ClusterMetadataQueryRequest) (*pb.ClusterMetadataAnswer, error) {
	answer, err := s.store.localLookup(ctx, cmLookupRequest(req))
	if err != nil {
		return nil, err
	}
	return answerToProto(answer), nil
}

func (s *PeerServer) QueryOrigin(ctx context.Context, req *pb.ClusterMetadataOriginRequest) (*pb.ClusterMetadataAnswer, error) {
	answer, err := s.store.localLookupOrigin(ctx, cmOriginRequest(req))
	if err != nil {
		return nil, err
	}
	return answerToProto(answer), nil
}

func (s *PeerServer) Snapshot(ctx context.Context, req *pb.ClusterMetadataSnapshotRequest) (*pb.ClusterMetadataSnapshot, error) {
	snapshot, err := s.store.Snapshot(ctx, req.GetKind(), req.GetNamespace(), scopeFromProto(req.GetScope()))
	if err != nil {
		return nil, err
	}

	response := &pb.ClusterMetadataSnapshot{Nodes: snapshot.Nodes}
	for _, event := range snapshot.Events {
		response.Events = append(response.Events, &pb.ClusterMetadataNodeEvent{
			Kind:      event.Kind,
			Namespace: event.Namespace,
			Name:      event.Name,
			Deleted:   event.Deleted,
			Tags:      event.Tags,
		})
	}
	return response, nil
}

func (s *PeerServer) Ring(ctx context.Context, _ *pb.ClusterMetadataRingRequest) (*pb.ClusterMetadataRing, error) {
	ring, err := s.store.Ring(ctx)
	if err != nil {
		return nil, err
	}

	response := &pb.ClusterMetadataRing{}
	for _, member := range ring.Members {
		response.Members = append(response.Members, &pb.ClusterMetadataRingMember{
			Name:  member.Name,
			Nodes: member.Nodes,
			Ready: member.Ready,
		})
	}
	return response, nil
}

func cmLookupRequest(req *pb.ClusterMetadataQueryRequest) cm.LookupRequest {
	return cm.LookupRequest{
		Key: cm.EntityKey{
			Kind:      req.GetKey().GetKind(),
			Namespace: req.GetKey().GetNamespace(),
			Name:      req.GetKey().GetName(),
		},
		Scope: scopeFromProto(req.GetScope()),
	}
}

func cmOriginRequest(req *pb.ClusterMetadataOriginRequest) cm.OriginLookupRequest {
	return cm.OriginLookupRequest{
		Key: cm.OriginKey{
			PodUID:        req.GetPodUid(),
			ContainerID:   req.GetContainerId(),
			ContainerName: req.GetContainerName(),
		},
		Scope: scopeFromProto(req.GetScope()),
	}
}
