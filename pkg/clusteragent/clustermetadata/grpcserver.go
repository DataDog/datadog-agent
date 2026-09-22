// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	cm "github.com/DataDog/datadog-agent/pkg/clustermetadata"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// PeerServer implements the generated ClusterMetadata gRPC service: the
// peer-facing surface of one replica. Every method answers from this
// replica's local cache only — a peer never fans out to further peers,
// which is the recursion boundary of the ring: the coordinator broadcasts,
// each peer answers for its own shard.
type PeerServer struct {
	store *LocalStore
	pb.UnimplementedClusterMetadataServer
}

// NewPeerServer returns the gRPC server for the store. The store must be the
// same replica's LocalStore; the peer-facing methods deliberately call the
// local paths, not the coordinator ones.
func NewPeerServer(store *LocalStore) *PeerServer {
	return &PeerServer{store: store}
}

// Register wires the ring's two serving surfaces into a gRPC server: the
// peer service (local answers only) and the consumer service (coordinator
// semantics, any replica can serve it).
func (s *PeerServer) Register(server *grpc.Server) {
	pb.RegisterClusterMetadataServer(server, s)
	pb.RegisterClusterMetadataConsumerServer(server, NewConsumerServer(s.store))
}

// Query answers a named-entity lookup from the local cache.
func (s *PeerServer) Query(ctx context.Context, req *pb.ClusterMetadataQueryRequest) (*pb.ClusterMetadataAnswer, error) {
	answer, err := s.store.localLookup(ctx, cmLookupRequest(req))
	if err != nil {
		return nil, err
	}
	return answerToProto(answer), nil
}

// QueryOrigin answers an origin lookup from the local cache.
func (s *PeerServer) QueryOrigin(ctx context.Context, req *pb.ClusterMetadataOriginRequest) (*pb.ClusterMetadataAnswer, error) {
	answer, err := s.store.localLookupOrigin(ctx, cmOriginRequest(req))
	if err != nil {
		return nil, err
	}
	return answerToProto(answer), nil
}

// Snapshot returns this replica's shard of an enumeration.
func (s *PeerServer) Snapshot(ctx context.Context, req *pb.ClusterMetadataSnapshotRequest) (*pb.ClusterMetadataSnapshot, error) {
	snapshot, err := s.store.Snapshot(ctx, req.GetKind(), req.GetNamespace(), scopeFromProto(req.GetScope()))
	if err != nil {
		return nil, err
	}
	return snapshotToProto(snapshot), nil
}

// Ring returns this replica's view of the ring.
func (s *PeerServer) Ring(ctx context.Context, _ *pb.ClusterMetadataRingRequest) (*pb.ClusterMetadataRing, error) {
	ring, err := s.store.Ring(ctx)
	if err != nil {
		return nil, err
	}
	return ringToProto(ring), nil
}

// ConsumerServer implements the generated ClusterMetadataConsumer gRPC
// service: the consumer-facing surface. Every method has coordinator
// semantics — a pod query fans out to the peer replicas and answers for the
// whole ring, so a consumer can ask any replica; a node stream is served by
// its owner.
type ConsumerServer struct {
	store *LocalStore
	pb.UnimplementedClusterMetadataConsumerServer
}

// NewConsumerServer returns the consumer-facing gRPC server for the store.
func NewConsumerServer(store *LocalStore) *ConsumerServer {
	return &ConsumerServer{store: store}
}

// Query answers from the coordinator: any replica can serve it.
func (s *ConsumerServer) Query(ctx context.Context, req *pb.ClusterMetadataQueryRequest) (*pb.ClusterMetadataAnswer, error) {
	answer, err := s.store.Lookup(ctx, cmLookupRequest(req))
	if err != nil {
		return nil, err
	}
	return answerToProto(answer), nil
}

// QueryOrigin answers from the coordinator: any replica can serve it.
func (s *ConsumerServer) QueryOrigin(ctx context.Context, req *pb.ClusterMetadataOriginRequest) (*pb.ClusterMetadataAnswer, error) {
	answer, err := s.store.LookupOrigin(ctx, cmOriginRequest(req))
	if err != nil {
		return nil, err
	}
	return answerToProto(answer), nil
}

// Snapshot returns this replica's shard of an enumeration.
func (s *ConsumerServer) Snapshot(ctx context.Context, req *pb.ClusterMetadataSnapshotRequest) (*pb.ClusterMetadataSnapshot, error) {
	snapshot, err := s.store.Snapshot(ctx, req.GetKind(), req.GetNamespace(), scopeFromProto(req.GetScope()))
	if err != nil {
		return nil, err
	}
	return snapshotToProto(snapshot), nil
}

// Ring returns this replica's view of the ring.
func (s *ConsumerServer) Ring(ctx context.Context, _ *pb.ClusterMetadataRingRequest) (*pb.ClusterMetadataRing, error) {
	ring, err := s.store.Ring(ctx)
	if err != nil {
		return nil, err
	}
	return ringToProto(ring), nil
}

// Subscribe streams one node's events from its owning replica. The stream
// ends with an error when ownership of the node moves: the consumer
// re-resolves the owner through Ring and resubscribes.
func (s *ConsumerServer) Subscribe(req *pb.ClusterMetadataSubscribeRequest, stream pb.ClusterMetadataConsumer_SubscribeServer) error {
	events, cancel, err := s.store.Subscribe(stream.Context(), req.GetNode(), scopeFromProto(req.GetScope()))
	if err != nil {
		// The replica does not own the node or the node is not synced yet:
		// the consumer re-resolves the owner through Ring and retries.
		return status.Error(codes.Unavailable, err.Error())
	}
	defer cancel()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case event, ok := <-events:
			if !ok {
				// Ownership of the node moved: resubscribe to the new owner.
				return status.Error(codes.Unavailable, "ownership of node "+req.GetNode()+" moved")
			}
			if err := stream.Send(eventToProto(event)); err != nil {
				return err
			}
		}
	}
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
