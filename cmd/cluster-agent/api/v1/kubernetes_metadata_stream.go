// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taglist"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
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

type kueueQueueTagsEntry struct {
	namespace  string
	localQueue string
	tags       workloadmeta.KueueQueueTags
}

// KubeMetadataStreamServer streams pod-to-service mappings and namespace
// labels/annotations from the DCA to node agents.
type KubeMetadataStreamServer struct {
	store *controllers.MetaBundleStore
	wmeta workloadmeta.Component

	namespacesMutex sync.RWMutex
	namespaces      map[string]namespaceEntry // keys are namespace names
	kueueQueues     map[string]kueueQueueTagsEntry
	// namespaceSubscribers holds notification channels per node name. A node
	// can have multiple subscribers because more than one process (for example,
	// the running agent plus "agent diagnose", "agent check", etc.) may stream
	// metadata for the same node concurrently.
	namespaceSubscribers map[string][]chan struct{}
}

// NewKubeMetadataStreamServer creates a new KubeMetadataStreamServer
func NewKubeMetadataStreamServer(store *controllers.MetaBundleStore, wmeta workloadmeta.Component) *KubeMetadataStreamServer {
	return &KubeMetadataStreamServer{
		store:                store,
		wmeta:                wmeta,
		namespaces:           make(map[string]namespaceEntry),
		kueueQueues:          make(map[string]kueueQueueTagsEntry),
		namespaceSubscribers: make(map[string][]chan struct{}),
	}
}

// Start subscribes to workloadmeta for metadata changes and maintains state.
// It must be called before serving streams.
func (srv *KubeMetadataStreamServer) Start(ctx context.Context) {
	ch := srv.wmeta.Subscribe(
		wmetaSubscriberName,
		workloadmeta.NormalPriority,
		kubeMetadataStreamFilter(),
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
				srv.processWmetaEvents(bundle.Events)
			}
		}
	}()
}

// StreamKubeMetadata streams pod-to-service mappings and namespace metadata to
// the requesting node agent.
func (srv *KubeMetadataStreamServer) StreamKubeMetadata(req *pb.KubeMetadataStreamRequest, stream pb.AgentSecure_StreamKubeMetadataServer) error {
	nodeName := req.GetNodeName()

	podServicesNotifyCh := srv.store.Subscribe(nodeName)
	defer srv.store.Unsubscribe(nodeName, podServicesNotifyCh)

	namespacesNotifyCh := srv.subscribeToNamespaceEvents(nodeName)
	defer srv.unsubscribeFromNamespaceEvents(nodeName, namespacesNotifyCh)

	// Send initial full state
	lastSentPodServicesState := srv.buildPodServiceMappingsSnapshot(nodeName)
	lastSentNamespacesState := srv.buildNamespacesSnapshot()
	lastSentKueueQueuesState := srv.buildKueueQueuesSnapshot()
	initialResp := fullStateResponse(lastSentPodServicesState, lastSentNamespacesState, lastSentKueueQueuesState)
	initialSendSpan := tracer.StartSpan("cluster_agent.metadata_stream.send_full_state",
		tracer.ResourceName("sendFullState"),
		tracer.Tag("node_name", nodeName),
	)
	if err := grpc.DoWithTimeout(func() error {
		return stream.Send(initialResp)
	}, streamSendTimeout); err != nil {
		log.Warnf("Error sending initial kube metadata state for node %s: %s", nodeName, err)
		initialSendSpan.Finish(tracer.WithError(err))
		return err
	}
	initialSendSpan.Finish()

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
			sendSpan := tracer.StartSpan("cluster_agent.metadata_stream.send_diff",
				tracer.ResourceName("sendDiff"),
				tracer.Tag("node_name", nodeName),
				tracer.Tag("event_type", "pod_services"),
			)
			if err := grpc.DoWithTimeout(func() error {
				return stream.Send(resp)
			}, streamSendTimeout); err != nil {
				log.Warnf("Error sending pod-service metadata diff for node %s: %s", nodeName, err)
				sendSpan.Finish(tracer.WithError(err))
				return err
			}
			sendSpan.Finish()
			lastSentPodServicesState = currentPodServiceMappingsState
			ticker.Reset(keepAliveInterval)

		case <-namespacesNotifyCh:
			currentNamespacesState := srv.buildNamespacesSnapshot()
			namespacesDiff := computeNamespacesDiff(lastSentNamespacesState, currentNamespacesState)
			currentKueueQueuesState := srv.buildKueueQueuesSnapshot()
			kueueQueuesDiff := computeKueueQueueTagsDiff(lastSentKueueQueuesState, currentKueueQueuesState)
			if len(namespacesDiff)+len(kueueQueuesDiff) == 0 {
				continue
			}
			resp := &pb.KubeMetadataStreamResponse{
				IsFullState:       false,
				NamespaceMetadata: namespacesDiff,
				KueueQueueTags:    kueueQueuesDiff,
			}
			sendSpan := tracer.StartSpan("cluster_agent.metadata_stream.send_diff",
				tracer.ResourceName("sendDiff"),
				tracer.Tag("node_name", nodeName),
				tracer.Tag("event_type", "metadata"),
			)
			if err := grpc.DoWithTimeout(func() error {
				return stream.Send(resp)
			}, streamSendTimeout); err != nil {
				log.Warnf("Error sending metadata diff for node %s: %s", nodeName, err)
				sendSpan.Finish(tracer.WithError(err))
				return err
			}
			sendSpan.Finish()
			lastSentNamespacesState = currentNamespacesState
			lastSentKueueQueuesState = currentKueueQueuesState
			ticker.Reset(keepAliveInterval)

		case <-ticker.C:
			// Send empty keepalive
			keepaliveSpan := tracer.StartSpan("cluster_agent.metadata_stream.send_keepalive",
				tracer.ResourceName("sendKeepalive"),
				tracer.Tag("node_name", nodeName),
			)
			if err := grpc.DoWithTimeout(func() error {
				return stream.Send(&pb.KubeMetadataStreamResponse{})
			}, streamSendTimeout); err != nil {
				log.Warnf("Error sending kube metadata keepalive for node %s: %s", nodeName, err)
				keepaliveSpan.Finish(tracer.WithError(err))
				return err
			}
			keepaliveSpan.Finish()
		}
	}
}

func (srv *KubeMetadataStreamServer) processWmetaEvents(events []workloadmeta.Event) {
	srv.namespacesMutex.Lock()
	defer srv.namespacesMutex.Unlock()

	changed := false
	for _, event := range events {
		switch entity := event.Entity.(type) {
		case *workloadmeta.KubernetesMetadata:
			if srv.processNamespaceEvent(event.Type, entity) {
				changed = true
			}
		case *workloadmeta.KubernetesKueueQueue:
			if srv.processKueueQueueEvent(event.Type, entity) {
				changed = true
			}
		default:
			log.Errorf("Unexpected workloadmeta entity %T in kube metadata stream", event.Entity)
		}
	}

	if changed {
		srv.notifyNamespaceSubscribers()
	}
}

func (srv *KubeMetadataStreamServer) processNamespaceEvent(eventType workloadmeta.EventType, metadata *workloadmeta.KubernetesMetadata) bool {
	namespaceName := metadata.Name

	switch eventType {
	case workloadmeta.EventTypeSet:
		srv.namespaces[namespaceName] = namespaceEntry{
			labels:      metadata.Labels,
			annotations: metadata.Annotations,
		}
		return true
	case workloadmeta.EventTypeUnset:
		if _, exists := srv.namespaces[namespaceName]; exists {
			delete(srv.namespaces, namespaceName)
			return true
		}
	case workloadmeta.EventTypeAll:
		log.Errorf("Unexpected event type %d for namespace %s", eventType, namespaceName)
	default:
		log.Errorf("Unknown event type %d for namespace %s", eventType, namespaceName)
	}
	return false
}

func (srv *KubeMetadataStreamServer) processKueueQueueEvent(eventType workloadmeta.EventType, queue *workloadmeta.KubernetesKueueQueue) bool {
	if queue.QueueType != workloadmeta.KueueLocalQueue {
		return false
	}

	key := queue.Namespace + "/" + queue.Name
	switch eventType {
	case workloadmeta.EventTypeSet:
		srv.kueueQueues[key] = kueueQueueTagsEntry{
			namespace:  queue.Namespace,
			localQueue: queue.Name,
			tags:       buildKueueQueueTags(queue),
		}
		return true
	case workloadmeta.EventTypeUnset:
		if _, exists := srv.kueueQueues[key]; exists {
			delete(srv.kueueQueues, key)
			return true
		}
	case workloadmeta.EventTypeAll:
		log.Errorf("Unexpected event type %d for Kueue queue %s", eventType, key)
	default:
		log.Errorf("Unknown event type %d for Kueue queue %s", eventType, key)
	}
	return false
}

func (srv *KubeMetadataStreamServer) notifyNamespaceSubscribers() {
	for _, channels := range srv.namespaceSubscribers {
		for _, ch := range channels {
			select {
			// Non-blocking send: if a signal is already pending, we drop it.
			// This is safe because the consumer re-reads the full state from
			// the store on each signal.
			case ch <- struct{}{}:
			default:
			}
		}
	}
}

func (srv *KubeMetadataStreamServer) subscribeToNamespaceEvents(nodeName string) <-chan struct{} {
	srv.namespacesMutex.Lock()
	defer srv.namespacesMutex.Unlock()

	ch := make(chan struct{}, 1)
	srv.namespaceSubscribers[nodeName] = append(srv.namespaceSubscribers[nodeName], ch)

	log.Debugf("Subscribed to namespace metadata updates for node %s (subscribers=%d)",
		nodeName,
		len(srv.namespaceSubscribers[nodeName]))

	return ch
}

func (srv *KubeMetadataStreamServer) unsubscribeFromNamespaceEvents(nodeName string, ch <-chan struct{}) {
	srv.namespacesMutex.Lock()
	defer srv.namespacesMutex.Unlock()

	channels := srv.namespaceSubscribers[nodeName]
	for i, c := range channels {
		if c == ch {
			srv.namespaceSubscribers[nodeName] = slices.Delete(channels, i, i+1)
			break
		}
	}

	remaining := len(srv.namespaceSubscribers[nodeName])
	if remaining == 0 {
		delete(srv.namespaceSubscribers, nodeName)
	}

	log.Debugf("Unsubscribed from namespace metadata updates for node %s (subscribers=%d)", nodeName, remaining)
}

func (srv *KubeMetadataStreamServer) buildNamespacesSnapshot() map[string]namespaceEntry {
	srv.namespacesMutex.RLock()
	defer srv.namespacesMutex.RUnlock()

	snapshot := make(map[string]namespaceEntry, len(srv.namespaces))
	for ns, entry := range srv.namespaces {
		snapshot[ns] = namespaceEntry{
			labels:      entry.labels,
			annotations: entry.annotations,
		}
	}
	return snapshot
}

func (srv *KubeMetadataStreamServer) buildKueueQueuesSnapshot() map[string]kueueQueueTagsEntry {
	srv.namespacesMutex.RLock()
	defer srv.namespacesMutex.RUnlock()

	snapshot := make(map[string]kueueQueueTagsEntry, len(srv.kueueQueues))
	for key, entry := range srv.kueueQueues {
		snapshot[key] = entry
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

func kubeMetadataStreamFilter() *workloadmeta.Filter {
	return workloadmeta.NewFilterBuilder().AddKindWithEntityFilter(
		workloadmeta.KindKubernetesMetadata,
		func(entity workloadmeta.Entity) bool {
			metadata := entity.(*workloadmeta.KubernetesMetadata)
			return workloadmeta.IsNamespaceMetadata(metadata)
		},
	).AddKind(workloadmeta.KindKubernetesKueueQueue).Build()
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
func fullStateResponse(podServices map[string]podServiceEntry, namespaces map[string]namespaceEntry, kueueQueues map[string]kueueQueueTagsEntry) *pb.KubeMetadataStreamResponse {
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

	kueueQueueTags := make([]*pb.KueueQueueTags, 0, len(kueueQueues))
	for _, entry := range kueueQueues {
		kueueQueueTags = append(kueueQueueTags, protoKueueQueueTags(entry, pb.KubeMetadataEventType_SET))
	}

	return &pb.KubeMetadataStreamResponse{
		IsFullState:       true,
		Mappings:          mappings,
		NamespaceMetadata: namespacesMetadata,
		KueueQueueTags:    kueueQueueTags,
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

func computeKueueQueueTagsDiff(old, current map[string]kueueQueueTagsEntry) []*pb.KueueQueueTags {
	var diff []*pb.KueueQueueTags

	for key, cur := range current {
		prev, existed := old[key]
		if !existed || !kueueQueueTagsEqual(prev.tags, cur.tags) {
			diff = append(diff, protoKueueQueueTags(cur, pb.KubeMetadataEventType_SET))
		}
	}

	for key, prev := range old {
		if _, exists := current[key]; !exists {
			diff = append(diff, protoKueueQueueTags(prev, pb.KubeMetadataEventType_UNSET))
		}
	}

	return diff
}

func protoKueueQueueTags(entry kueueQueueTagsEntry, eventType pb.KubeMetadataEventType) *pb.KueueQueueTags {
	return &pb.KueueQueueTags{
		Namespace:                   entry.namespace,
		LocalQueue:                  entry.localQueue,
		LowCardinalityTags:          entry.tags.Low,
		OrchestratorCardinalityTags: entry.tags.Orchestrator,
		HighCardinalityTags:         entry.tags.High,
		StandardTags:                entry.tags.Standard,
		Type:                        eventType,
	}
}

func kueueQueueTagsEqual(left, right workloadmeta.KueueQueueTags) bool {
	return slices.Equal(left.Low, right.Low) &&
		slices.Equal(left.Orchestrator, right.Orchestrator) &&
		slices.Equal(left.High, right.High) &&
		slices.Equal(left.Standard, right.Standard)
}

func buildKueueQueueTags(queue *workloadmeta.KubernetesKueueQueue) workloadmeta.KueueQueueTags {
	tagList := taglist.NewTagList()
	switch queue.QueueType {
	case workloadmeta.KueueLocalQueue:
		tagList.AddLow(tags.KueueLocalQueue, queue.Name)
		tagList.AddLow(tags.KueueClusterQueue, queue.ClusterQueueName)
		tagList.AddLow(tags.KubeNamespace, queue.Namespace)
	case workloadmeta.KueueClusterQueue:
		tagList.AddLow(tags.KueueClusterQueue, queue.Name)
	}

	low, orch, high, standard := tagList.Compute()
	return workloadmeta.KueueQueueTags{
		Low:          low,
		Orchestrator: orch,
		High:         high,
		Standard:     standard,
	}
}
