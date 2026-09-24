// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"context"

	"google.golang.org/grpc"

	cm "github.com/DataDog/datadog-agent/pkg/clustermetadata"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// PeerClient implements cm.Store between replicas using the ClusterMetadata
// gRPC service.
type PeerClient struct {
	client pb.ClusterMetadataClient
}

var _ cm.Store = (*PeerClient)(nil)

// NewPeerClient returns a peer client over an established connection.
func NewPeerClient(conn *grpc.ClientConn) *PeerClient {
	return &PeerClient{client: pb.NewClusterMetadataClient(conn)}
}

// Lookup implements cm.Store against the remote Query.
func (p *PeerClient) Lookup(ctx context.Context, req cm.LookupRequest) (cm.LookupAnswer, error) {
	answer, err := p.client.Query(ctx, &pb.ClusterMetadataQueryRequest{
		Key: &pb.ClusterMetadataEntityKey{
			Kind:      req.Key.Kind,
			Namespace: req.Key.Namespace,
			Name:      req.Key.Name,
		},
		Scope: scopeToProto(req.Scope),
	})
	if err != nil {
		return cm.LookupAnswer{}, err
	}
	return answerFromProto(answer), nil
}

// LookupOrigin implements cm.Store against the remote QueryOrigin.
func (p *PeerClient) LookupOrigin(ctx context.Context, req cm.OriginLookupRequest) (cm.LookupAnswer, error) {
	answer, err := p.client.QueryOrigin(ctx, &pb.ClusterMetadataOriginRequest{
		PodUid:        req.Key.PodUID,
		ContainerId:   req.Key.ContainerID,
		ContainerName: req.Key.ContainerName,
		Scope:         scopeToProto(req.Scope),
	})
	if err != nil {
		return cm.LookupAnswer{}, err
	}
	return answerFromProto(answer), nil
}
