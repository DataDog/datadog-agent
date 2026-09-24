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

// Register wires the peer service into a gRPC server.
func (s *PeerServer) Register(server *grpc.Server) {
	pb.RegisterClusterMetadataServer(server, s)
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
