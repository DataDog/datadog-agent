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

	return snapshotFromProto(snapshot), nil
}

// Ring implements cm.Store against the remote Ring.
func (p *PeerClient) Ring(ctx context.Context) (cm.RingInfo, error) {
	ring, err := p.client.Ring(ctx, &pb.ClusterMetadataRingRequest{})
	if err != nil {
		return cm.RingInfo{}, err
	}

	return ringFromProto(ring), nil
}

// ConsumerClient implements cm.Store against a replica's
// ClusterMetadataConsumer service: the coordinator-facing surface. Any
// replica can serve it — pod queries fan out from the serving replica to
// its peers, and node streams come from the node's owner.
type ConsumerClient struct {
	client pb.ClusterMetadataConsumerClient
}

var _ cm.Store = (*ConsumerClient)(nil)

// NewConsumerClient returns a consumer client over an established connection.
func NewConsumerClient(conn *grpc.ClientConn) *ConsumerClient {
	return &ConsumerClient{client: pb.NewClusterMetadataConsumerClient(conn)}
}

// Lookup implements cm.Store against the remote consumer Query.
func (c *ConsumerClient) Lookup(ctx context.Context, req cm.LookupRequest) (cm.LookupAnswer, error) {
	answer, err := c.client.Query(ctx, &pb.ClusterMetadataQueryRequest{
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

// LookupOrigin implements cm.Store against the remote consumer QueryOrigin.
func (c *ConsumerClient) LookupOrigin(ctx context.Context, req cm.OriginLookupRequest) (cm.LookupAnswer, error) {
	answer, err := c.client.QueryOrigin(ctx, &pb.ClusterMetadataOriginRequest{
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

// Subscribe implements cm.Store against the remote consumer stream. The
// returned channel closes when the server ends the stream — ownership of the
// node moved — and the consumer re-resolves the owner through Ring.
func (c *ConsumerClient) Subscribe(ctx context.Context, node string, scope cm.Scope) (<-chan cm.NodeEvent, func(), error) {
	stream, err := c.client.Subscribe(ctx, &pb.ClusterMetadataSubscribeRequest{
		Node:  node,
		Scope: scopeToProto(scope),
	})
	if err != nil {
		return nil, nil, err
	}

	// Server-streaming RPCs deliver handler errors on Recv, not on open: the
	// first receive runs eagerly so a refusal (wrong owner, not synced) is
	// the synchronous error the contract promises, not a silent empty stream.
	first, err := stream.Recv()
	if err != nil {
		return nil, nil, err
	}

	events := make(chan cm.NodeEvent, streamBufferSize)
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(events)
		defer cancel()

		send := func(event *pb.ClusterMetadataNodeEvent) bool {
			select {
			case events <- nodeEventFromProto(event):
				return true
			case <-ctx.Done():
				return false
			}
		}

		if !send(first) {
			return
		}
		for {
			event, err := stream.Recv()
			if err != nil {
				// The stream ended — ownership moved: the caller reads the
				// closed channel and re-resolves the owner through Ring.
				return
			}
			if !send(event) {
				return
			}
		}
	}()
	return events, cancel, nil
}

func nodeEventFromProto(event *pb.ClusterMetadataNodeEvent) cm.NodeEvent {
	return cm.NodeEvent{
		Kind:      event.GetKind(),
		Namespace: event.GetNamespace(),
		Name:      event.GetName(),
		Deleted:   event.GetDeleted(),
		Tags:      event.GetTags(),
	}
}

// Snapshot implements cm.Store against the remote consumer Snapshot.
func (c *ConsumerClient) Snapshot(ctx context.Context, kind string, namespace string, scope cm.Scope) (cm.ShardSnapshot, error) {
	snapshot, err := c.client.Snapshot(ctx, &pb.ClusterMetadataSnapshotRequest{
		Kind:      kind,
		Namespace: namespace,
		Scope:     scopeToProto(scope),
	})
	if err != nil {
		return cm.ShardSnapshot{}, err
	}
	return snapshotFromProto(snapshot), nil
}

// Ring implements cm.Store against the remote consumer Ring.
func (c *ConsumerClient) Ring(ctx context.Context) (cm.RingInfo, error) {
	ring, err := c.client.Ring(ctx, &pb.ClusterMetadataRingRequest{})
	if err != nil {
		return cm.RingInfo{}, err
	}
	return ringFromProto(ring), nil
}
