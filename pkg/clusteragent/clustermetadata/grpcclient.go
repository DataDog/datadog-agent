// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"context"
	"fmt"

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

// Subscribe is not implemented for peer clients.
func (p *PeerClient) Subscribe(ctx context.Context, node string, scope cm.Scope) (<-chan cm.NodeEvent, func(), error) {
	return nil, nil, fmt.Errorf("node streams are not served between replicas")
}

// Snapshot implements cm.Store against the remote Snapshot.
func (p *PeerClient) Snapshot(ctx context.Context, kind string, namespace string, scope cm.Scope) (cm.ShardSnapshot, error) {
	snapshot, err := p.client.Snapshot(ctx, &pb.ClusterMetadataSnapshotRequest{
		Kind:      kind,
		Namespace: namespace,
		Scope:     scopeToProto(scope),
	})
	if err != nil {
		return cm.ShardSnapshot{}, err
	}

	result := cm.ShardSnapshot{Nodes: snapshot.GetNodes()}
	for _, event := range snapshot.GetEvents() {
		result.Events = append(result.Events, cm.NodeEvent{
			Kind:      event.GetKind(),
			Namespace: event.GetNamespace(),
			Name:      event.GetName(),
			Deleted:   event.GetDeleted(),
			Tags:      event.GetTags(),
		})
	}
	return result, nil
}

// Ring implements cm.Store against the remote Ring.
func (p *PeerClient) Ring(ctx context.Context) (cm.RingInfo, error) {
	ring, err := p.client.Ring(ctx, &pb.ClusterMetadataRingRequest{})
	if err != nil {
		return cm.RingInfo{}, err
	}

	info := cm.RingInfo{}
	for _, member := range ring.GetMembers() {
		info.Members = append(info.Members, cm.RingMember{
			Name:  member.GetName(),
			Nodes: member.GetNodes(),
			Ready: member.GetReady(),
		})
	}
	return info, nil
}
