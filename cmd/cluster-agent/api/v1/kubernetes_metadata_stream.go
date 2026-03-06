// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"context"
	"maps"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
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
	keepAliveInterval   = 9 * time.Minute
	wmetaSubscriberName = "kube-metadata-stream"
)

type podServiceEntry struct {
	namespace string
	podName   string
	services  sets.Set[string]
}

type namespaceEntry struct {
	labels      map[string]string
	annotations map[string]string
}

// KubeMetadataStreamServer streams pod-to-service mappings and namespace
// labels/annotations from the DCA to node agents.
type KubeMetadataStreamServer struct {
	store *controllers.MetaBundleStore
	wmeta workloadmeta.Component

	namespacesMutex      sync.RWMutex
	namespaces           map[string]namespaceEntry // keys are namespace names
	namespaceSubscribers map[string]chan struct{}  // keys are node names
}

// NewKubeMetadataStreamServer creates a new KubeMetadataStreamServer
func NewKubeMetadataStreamServer(store *controllers.MetaBundleStore, wmeta workloadmeta.Component) *KubeMetadataStreamServer {
	return &KubeMetadataStreamServer{
		store:                store,
		wmeta:                wmeta,
		namespaces:           make(map[string]namespaceEntry),
		namespaceSubscribers: make(map[string]chan struct{}),
	}
}

// Start subscribes to workloadmeta for namespace metadata changes and
// maintains the namespace state. It must be called before serving streams.
func (srv *KubeMetadataStreamServer) Start(ctx context.Context) {
	ch := srv.wmeta.Subscribe(
		wmetaSubscriberName,
		workloadmeta.NormalPriority,
		namespaceMetadataFilter(),
	)

	go func() {
		defer srv.wmeta.Unsubscribe(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case bundle, ok := <-ch:
				if !ok {
					return
				}
				bundle.Acknowledge()
				srv.processNamespaceEvents(bundle.Events)
			}
		}
	}()
}

// StreamKubeMetadata streams pod-to-service mappings and namespace metadata to
// the requesting node agent.
func (srv *KubeMetadataStreamServer) StreamKubeMetadata(req *pb.KubeMetadataStreamRequest, stream pb.AgentSecure_StreamKubeMetadataServer) error {
	nodeName := req.GetNodeName()

	podServicesNotifyCh := srv.store.Subscribe(nodeName)
	defer srv.store.Unsubscribe(nodeName)

	namespacesNotifyCh := srv.subscribeToNamespaceEvents(nodeName)
	defer srv.unsubscribeFromNamespaceEvents(nodeName)

	// Send initial full state
	lastSentPodServicesState := srv.buildPodServiceMappingsSnapshot(nodeName)
	lastSentNamespacesState := srv.buildNamespacesSnapshot()
	initialResp := fullStateResponse(lastSentPodServicesState, lastSentNamespacesState)
	if err := grpc.DoWithTimeout(func() error {
		return stream.Send(initialResp)
	}, streamSendTimeout); err != nil {
		log.Warnf("Error sending initial kube metadata state for node %s: %s", nodeName, err)
		return err
	}

	ticker := time.NewTicker(keepAliveInterval)
	defer ticker.Stop()

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return nil

		case <-podServicesNotifyCh:
			currentPodServiceMappingsState := srv.buildPodServiceMappingsSnapshot(nodeName)
			podServiceMappingsDiff := computePodServiceMappingsDiff(lastSentPodServicesState, currentPodServiceMappingsState)
			if len(podServiceMappingsDiff) == 0 {
				continue
			}
			resp := &pb.KubeMetadataStreamResponse{
				IsFullState: false,
				Mappings:    podServiceMappingsDiff,
			}
			if err := grpc.DoWithTimeout(func() error {
				return stream.Send(resp)
			}, streamSendTimeout); err != nil {
				log.Warnf("Error sending pod-service metadata diff for node %s: %s", nodeName, err)
				return err
			}
			lastSentPodServicesState = currentPodServiceMappingsState
			ticker.Reset(keepAliveInterval)

		case <-namespacesNotifyCh:
			currentNamespacesState := srv.buildNamespacesSnapshot()
			namespacesDiff := computeNamespacesDiff(lastSentNamespacesState, currentNamespacesState)
			if len(namespacesDiff) == 0 {
				continue
			}
			resp := &pb.KubeMetadataStreamResponse{
				IsFullState:       false,
				NamespaceMetadata: namespacesDiff,
			}
			if err := grpc.DoWithTimeout(func() error {
				return stream.Send(resp)
			}, streamSendTimeout); err != nil {
				log.Warnf("Error sending namespace metadata diff for node %s: %s", nodeName, err)
				return err
			}
			lastSentNamespacesState = currentNamespacesState
			ticker.Reset(keepAliveInterval)

		case <-ticker.C:
			// Send empty keepalive
			if err := grpc.DoWithTimeout(func() error {
				return stream.Send(&pb.KubeMetadataStreamResponse{})
			}, streamSendTimeout); err != nil {
				log.Warnf("Error sending kube metadata keepalive for node %s: %s", nodeName, err)
				return err
			}
		}
	}
}

func (srv *KubeMetadataStreamServer) processNamespaceEvents(events []workloadmeta.Event) {
	srv.namespacesMutex.Lock()
	defer srv.namespacesMutex.Unlock()

	changed := false
	for _, event := range events {
		metadata := event.Entity.(*workloadmeta.KubernetesMetadata)
		namespaceName := metadata.Name

		switch event.Type {
		case workloadmeta.EventTypeSet:
			srv.namespaces[namespaceName] = namespaceEntry{
				labels:      metadata.Labels,
				annotations: metadata.Annotations,
			}
			changed = true
		case workloadmeta.EventTypeUnset:
			if _, exists := srv.namespaces[namespaceName]; exists {
				delete(srv.namespaces, namespaceName)
				changed = true
			}
		default:
			log.Errorf("Unknown event type %d for namespace %s", event.Type, namespaceName)
		}
	}

	if changed {
		srv.notifyNamespaceSubscribers()
	}
}

func (srv *KubeMetadataStreamServer) notifyNamespaceSubscribers() {
	for _, ch := range srv.namespaceSubscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (srv *KubeMetadataStreamServer) subscribeToNamespaceEvents(nodeName string) <-chan struct{} {
	srv.namespacesMutex.Lock()
	defer srv.namespacesMutex.Unlock()

	ch := make(chan struct{}, 1)
	srv.namespaceSubscribers[nodeName] = ch
	return ch
}

func (srv *KubeMetadataStreamServer) unsubscribeFromNamespaceEvents(nodeName string) {
	srv.namespacesMutex.Lock()
	defer srv.namespacesMutex.Unlock()

	delete(srv.namespaceSubscribers, nodeName)
}

func (srv *KubeMetadataStreamServer) buildNamespacesSnapshot() map[string]namespaceEntry {
	srv.namespacesMutex.RLock()
	defer srv.namespacesMutex.RUnlock()

	snapshot := make(map[string]namespaceEntry, len(srv.namespaces))
	for ns, entry := range srv.namespaces {
		snapshot[ns] = namespaceEntry{
			labels:      maps.Clone(entry.labels),
			annotations: maps.Clone(entry.annotations),
		}
	}
	return snapshot
}

// buildPodServiceMappingsSnapshot reads the current bundle for a node and converts it to a
// snapshot map keyed by "namespace/podName".
func (srv *KubeMetadataStreamServer) buildPodServiceMappingsSnapshot(nodeName string) map[string]podServiceEntry {
	bundle, ok := srv.store.Get(nodeName)
	if !ok {
		return nil
	}
	return bundleToPodServiceMappingsSnapshot(bundle)
}

func namespaceMetadataFilter() *workloadmeta.Filter {
	return workloadmeta.NewFilterBuilder().AddKindWithEntityFilter(
		workloadmeta.KindKubernetesMetadata,
		func(entity workloadmeta.Entity) bool {
			metadata := entity.(*workloadmeta.KubernetesMetadata)
			return workloadmeta.IsNamespaceMetadata(metadata)
		},
	).Build()
}

func bundleToPodServiceMappingsSnapshot(bundle *apiserver.MetadataMapperBundle) map[string]podServiceEntry {
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
// is_full_state=true containing all current mappings and namespace labels and
// annotations.
func fullStateResponse(podServices map[string]podServiceEntry, namespaces map[string]namespaceEntry) *pb.KubeMetadataStreamResponse {
	mappings := make([]*pb.PodServiceMapping, 0, len(podServices))
	for _, entry := range podServices {
		mappings = append(mappings, &pb.PodServiceMapping{
			Namespace:    entry.namespace,
			PodName:      entry.podName,
			ServiceNames: sets.List(entry.services),
			Type:         pb.KubeMetadataEventType_SET,
		})
	}

	namespacesMetadata := make([]*pb.NamespaceMetadata, 0, len(namespaces))
	for namespace, entry := range namespaces {
		namespacesMetadata = append(namespacesMetadata, &pb.NamespaceMetadata{
			Namespace:   namespace,
			Labels:      entry.labels,
			Annotations: entry.annotations,
			Type:        pb.KubeMetadataEventType_SET,
		})
	}

	return &pb.KubeMetadataStreamResponse{
		IsFullState:       true,
		Mappings:          mappings,
		NamespaceMetadata: namespacesMetadata,
	}
}

// computePodServiceMappingsDiff compares old and new snapshots and returns set/unset events.
func computePodServiceMappingsDiff(old, current map[string]podServiceEntry) []*pb.PodServiceMapping {
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

// computeNamespacesDiff compares old and new namespace snapshots and returns set/unset events.
func computeNamespacesDiff(old, current map[string]namespaceEntry) []*pb.NamespaceMetadata {
	var diff []*pb.NamespaceMetadata

	for ns, cur := range current {
		prev, existed := old[ns]
		if !existed || !maps.Equal(prev.labels, cur.labels) || !maps.Equal(prev.annotations, cur.annotations) {
			diff = append(diff, &pb.NamespaceMetadata{
				Namespace:   ns,
				Labels:      cur.labels,
				Annotations: cur.annotations,
				Type:        pb.KubeMetadataEventType_SET,
			})
		}
	}

	for ns := range old {
		if _, exists := current[ns]; !exists {
			diff = append(diff, &pb.NamespaceMetadata{
				Namespace: ns,
				Type:      pb.KubeMetadataEventType_UNSET,
			})
		}
	}

	return diff
}
