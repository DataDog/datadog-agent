// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"time"

	"k8s.io/apimachinery/pkg/util/sets"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/controllers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	streamSendTimeout = 10 * time.Second
	// keepAliveInterval must be shorter than the client's timeout (10 min) to
	// prevent the client from treating an idle stream as dead.
	keepAliveInterval = 9 * time.Minute
)

type podServiceEntry struct {
	namespace string
	podName   string
	services  sets.Set[string]
}

// KubeMetadataStreamServer streams pod-to-service metadata from the DCA to node agents.
type KubeMetadataStreamServer struct {
	store *controllers.MetaBundleStore
}

// NewKubeMetadataStreamServer creates a new stream server with the given meta
// bundle store.
func NewKubeMetadataStreamServer(store *controllers.MetaBundleStore) *KubeMetadataStreamServer {
	return &KubeMetadataStreamServer{store: store}
}

// StreamKubeMetadata streams pod-to-service mappings to the requesting node
// agent.
func (s *KubeMetadataStreamServer) StreamKubeMetadata(req *pb.KubeMetadataStreamRequest, srv pb.AgentSecure_StreamKubeMetadataServer) error {
	nodeName := req.GetNodeName()

	notifyCh := s.store.Subscribe(nodeName)
	defer s.store.Unsubscribe(nodeName)

	// Send initial full state
	lastSentState := s.buildSnapshot(nodeName)
	initialResp := fullStateResponse(lastSentState)
	if err := grpc.DoWithTimeout(func() error {
		return srv.Send(initialResp)
	}, streamSendTimeout); err != nil {
		log.Warnf("Error sending initial kube metadata state for node %s: %s", nodeName, err)
		return err
	}

	ticker := time.NewTicker(keepAliveInterval)
	defer ticker.Stop()

	ctx := srv.Context()
	for {
		select {
		case <-ctx.Done():
			return nil

		case <-notifyCh:
			currentState := s.buildSnapshot(nodeName)
			diff := computeDiff(lastSentState, currentState)
			if len(diff) == 0 {
				continue
			}
			resp := &pb.KubeMetadataStreamResponse{
				IsFullState: false,
				Mappings:    diff,
			}
			if err := grpc.DoWithTimeout(func() error {
				return srv.Send(resp)
			}, streamSendTimeout); err != nil {
				log.Warnf("Error sending kube metadata diff for node %s: %s", nodeName, err)
				return err
			}
			lastSentState = currentState
			ticker.Reset(keepAliveInterval)

		case <-ticker.C:
			// Send empty keepalive
			if err := grpc.DoWithTimeout(func() error {
				return srv.Send(&pb.KubeMetadataStreamResponse{})
			}, streamSendTimeout); err != nil {
				log.Warnf("Error sending kube metadata keepalive for node %s: %s", nodeName, err)
				return err
			}
		}
	}
}

// buildSnapshot reads the current bundle for a node and converts it to a
// snapshot map keyed by "namespace/podName".
func (s *KubeMetadataStreamServer) buildSnapshot(nodeName string) map[string]podServiceEntry {
	bundle, ok := s.store.Get(nodeName)
	if !ok {
		return nil
	}
	return bundleToSnapshot(bundle)
}

func bundleToSnapshot(bundle *apiserver.MetadataMapperBundle) map[string]podServiceEntry {
	snapshot := make(map[string]podServiceEntry)
	for ns, pods := range bundle.Services {
		for podName, svcs := range pods {
			key := ns + "/" + podName
			snapshot[key] = podServiceEntry{
				namespace: ns,
				podName:   podName,
				services:  svcs.Clone(),
			}
		}
	}
	return snapshot
}

// fullStateResponse creates a KubeMetadataStreamResponse with
// is_full_state=true containing all current mappings.
func fullStateResponse(snapshot map[string]podServiceEntry) *pb.KubeMetadataStreamResponse {
	mappings := make([]*pb.PodServiceMapping, 0, len(snapshot))
	for _, entry := range snapshot {
		mappings = append(mappings, &pb.PodServiceMapping{
			Namespace:    entry.namespace,
			PodName:      entry.podName,
			ServiceNames: sets.List(entry.services),
			Type:         pb.KubeMetadataEventType_SET,
		})
	}

	return &pb.KubeMetadataStreamResponse{
		IsFullState: true,
		Mappings:    mappings,
	}
}

// computeDiff compares old and new snapshots and returns set/unset events.
func computeDiff(old, current map[string]podServiceEntry) []*pb.PodServiceMapping {
	var diff []*pb.PodServiceMapping

	// Add events for new or changed entries
	for key, cur := range current {
		prev, existed := old[key]
		if !existed || !prev.services.Equal(cur.services) {
			diff = append(diff, &pb.PodServiceMapping{
				Namespace:    cur.namespace,
				PodName:      cur.podName,
				ServiceNames: sets.List(cur.services),
				Type:         pb.KubeMetadataEventType_SET,
			})
		}
	}

	// Add events for removed entries
	for key, prev := range old {
		if _, exists := current[key]; !exists {
			diff = append(diff, &pb.PodServiceMapping{
				Namespace: prev.namespace,
				PodName:   prev.podName,
				Type:      pb.KubeMetadataEventType_UNSET,
			})
		}
	}

	return diff
}
