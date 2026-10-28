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

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"k8s.io/apimachinery/pkg/util/sets"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
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
	// Keep a bounded history of shared diffs. Streams that fall behind receive
	// a full state instead, so coalesced notifications cannot lose deletions.
	metadataHistorySize = 128
)

var metadataStreamHistoryResyncs = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
	"metadata_streaming",
	"history_resyncs",
	nil,
	"Number of full metadata states successfully sent because an agent fell behind the retained version history",
	telemetry.Options{NoDoubleUnderscoreSep: true},
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

type kueueQueueEntry struct {
	namespace        string
	name             string
	queueType        workloadmeta.KueueQueueType
	clusterQueueName string
	labels           map[string]string
	annotations      map[string]string
	uid              string
}

type kueueResourceFlavorEntry struct {
	name               string
	labels             map[string]string
	annotations        map[string]string
	uid                string
	nodeAffinityLabels map[string]string
}

type kueuePodSetAssignmentEntry struct {
	name    string
	flavors map[string]string
}

type kueueWorkloadEntry struct {
	namespace         string
	name              string
	queueName         string
	clusterQueueName  string
	labels            map[string]string
	annotations       map[string]string
	uid               string
	podSetAssignments []kueuePodSetAssignmentEntry
}

type kaiPodGroupEntry struct {
	namespace   string
	name        string
	queueName   string
	labels      map[string]string
	annotations map[string]string
	uid         string
}

// kaiPodRef is the node and PodGroup of a pod that KAI assigned to a PodGroup.
type kaiPodRef struct {
	nodeName    string
	podGroupKey string
}

type metadataSnapshot struct {
	namespaces           map[string]namespaceEntry
	kueueQueues          map[string]kueueQueueEntry
	kueueResourceFlavors map[string]kueueResourceFlavorEntry
	kueueWorkloads       map[string]kueueWorkloadEntry
}

// KubeMetadataStreamServer streams pod-to-service mappings and namespace
// labels/annotations from the DCA to node agents.
type KubeMetadataStreamServer struct {
	store *controllers.MetaBundleStore
	wmeta workloadmeta.Component

	metadataMutex   sync.RWMutex
	metadata        metadataSnapshot
	metadataVersion uint64
	// Responses are immutable after publication and shared by all streams.
	metadataHistory [metadataHistorySize]*pb.KubeMetadataStreamResponse
	// namespaceSubscribers holds notification channels per node name. A node
	// can have multiple subscribers because more than one process (for example,
	// the running agent plus "agent diagnose", "agent check", etc.) may stream
	// metadata for the same node concurrently. Subscribers are also notified
	// when the KAI PodGroups of their node change.
	namespaceSubscribers map[string][]chan struct{}

	// KAI PodGroups can be as many as pods, so they are kept out of the shared
	// history: each stream sends only the PodGroups of its node's pods, diffed
	// against what it last sent, like pod-to-service mappings.
	kaiEnabled   bool
	kaiPodGroups map[string]kaiPodGroupEntry
	// kaiPods indexes the pods assigned to a PodGroup by pod UID.
	kaiPods map[string]kaiPodRef
	// kaiNodePodGroups counts the pods of each PodGroup per node, and
	// kaiPodGroupNodes is the reverse index used to notify the nodes of a
	// changed PodGroup.
	kaiNodePodGroups map[string]map[string]int
	kaiPodGroupNodes map[string]map[string]int
}

// NewKubeMetadataStreamServer creates a new KubeMetadataStreamServer
func NewKubeMetadataStreamServer(store *controllers.MetaBundleStore, wmeta workloadmeta.Component) *KubeMetadataStreamServer {
	return &KubeMetadataStreamServer{
		store:                store,
		wmeta:                wmeta,
		metadata:             newMetadataSnapshot(),
		namespaceSubscribers: make(map[string][]chan struct{}),
		kaiEnabled:           pkgconfigsetup.Datadog().GetBool("cluster_agent.kai_scheduler.enabled"),
		kaiPodGroups:         make(map[string]kaiPodGroupEntry),
		kaiPods:              make(map[string]kaiPodRef),
		kaiNodePodGroups:     make(map[string]map[string]int),
		kaiPodGroupNodes:     make(map[string]map[string]int),
	}
}

func newMetadataSnapshot() metadataSnapshot {
	return metadataSnapshot{
		namespaces:           make(map[string]namespaceEntry),
		kueueQueues:          make(map[string]kueueQueueEntry),
		kueueResourceFlavors: make(map[string]kueueResourceFlavorEntry),
		kueueWorkloads:       make(map[string]kueueWorkloadEntry),
	}
}

// Start subscribes to workloadmeta for metadata changes and maintains state.
// It must be called before serving streams.
func (srv *KubeMetadataStreamServer) Start(ctx context.Context) {
	ch := srv.wmeta.Subscribe(
		wmetaSubscriberName,
		workloadmeta.NormalPriority,
		kubeMetadataStreamFilter(srv.kaiEnabled),
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
	metadata, lastSentMetadataVersion := srv.buildMetadataSnapshotWithVersion()
	lastSentKAIPodGroupsState := srv.buildKAIPodGroupsSnapshot(nodeName)
	initialResp := fullStateResponse(lastSentPodServicesState, metadata, lastSentKAIPodGroupsState)
	initialSendSpan := tracer.StartSpan("cluster_agent.metadata_stream.send_full_state",
		tracer.ResourceName("sendFullState"),
		tracer.Tag(ext.SpanKind, ext.SpanKindServer),
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
			if err := grpc.DoWithTimeout(func() error {
				return stream.Send(resp)
			}, streamSendTimeout); err != nil {
				log.Warnf("Error sending pod-service metadata diff for node %s: %s", nodeName, err)
				return err
			}
			lastSentPodServicesState = currentPodServiceMappingsState
			ticker.Reset(keepAliveInterval)

		case <-namespacesNotifyCh:
			updates, version, ok := srv.metadataUpdatesSince(lastSentMetadataVersion)
			currentKAIPodGroupsState := srv.buildKAIPodGroupsSnapshot(nodeName)
			if !ok {
				// A full state replaces all client caches, including pod-service
				// mappings and KAI PodGroups. Refresh their baselines along with
				// the metadata cursor.
				lastSentPodServicesState = srv.buildPodServiceMappingsSnapshot(nodeName)
				metadata, currentVersion := srv.buildMetadataSnapshotWithVersion()
				updates = []*pb.KubeMetadataStreamResponse{fullStateResponse(lastSentPodServicesState, metadata, currentKAIPodGroupsState)}
				version = currentVersion
			} else if kaiPodGroupsDiff := computeKAIPodGroupsDiff(lastSentKAIPodGroupsState, currentKAIPodGroupsState); len(kaiPodGroupsDiff) > 0 {
				updates = append(updates, &pb.KubeMetadataStreamResponse{
					IsFullState:  false,
					KaiPodGroups: kaiPodGroupsDiff,
				})
			}
			for _, resp := range updates {
				if err := grpc.DoWithTimeout(func() error {
					return stream.Send(resp)
				}, streamSendTimeout); err != nil {
					log.Warnf("Error sending metadata update for node %s: %s", nodeName, err)
					return err
				}
				if !ok {
					metadataStreamHistoryResyncs.Inc()
				}
				ticker.Reset(keepAliveInterval)
			}
			lastSentMetadataVersion = version
			lastSentKAIPodGroupsState = currentKAIPodGroupsState

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
	srv.metadataMutex.Lock()
	defer srv.metadataMutex.Unlock()

	// Snapshot and compare once per bundle, not once per connected agent. Only
	// bundles with shared metadata are snapshotted: pod and KAI PodGroup bundles
	// are frequent and would copy it for nothing.
	var previous *metadataSnapshot
	snapshotShared := func() {
		if previous == nil {
			snapshot := srv.metadata.clone()
			previous = &snapshot
		}
	}
	changed := false
	kaiNodes := make(map[string]struct{})
	for _, event := range events {
		switch entity := event.Entity.(type) {
		case *workloadmeta.KubernetesMetadata:
			snapshotShared()
			if srv.metadata.processNamespaceEvent(event.Type, entity) {
				changed = true
			}
		case *workloadmeta.KubernetesKueueQueue:
			snapshotShared()
			if srv.metadata.processKueueQueueEvent(event.Type, entity) {
				changed = true
			}
		case *workloadmeta.KubernetesKueueResourceFlavor:
			snapshotShared()
			if srv.metadata.processKueueResourceFlavorEvent(event.Type, entity) {
				changed = true
			}
		case *workloadmeta.KubernetesKueueWorkload:
			snapshotShared()
			if srv.metadata.processKueueWorkloadEvent(event.Type, entity) {
				changed = true
			}
		case *workloadmeta.KubernetesKAIPodGroup:
			if srv.processKAIPodGroupEvent(event.Type, entity) {
				for nodeName := range srv.kaiPodGroupNodes[entity.EntityID.ID] {
					kaiNodes[nodeName] = struct{}{}
				}
			}
		case *workloadmeta.KubernetesPod:
			for _, nodeName := range srv.processKAIPodEvent(event.Type, entity) {
				kaiNodes[nodeName] = struct{}{}
			}
		default:
			log.Errorf("Unexpected workloadmeta entity %T in kube metadata stream", event.Entity)
		}
	}

	if changed {
		diff := computeMetadataDiff(*previous, srv.metadata)
		if !diff.isEmpty() {
			srv.metadataVersion++
			srv.metadataHistory[srv.metadataVersion%metadataHistorySize] = diff.response(false)
			// Notifying all subscribers also covers the nodes in kaiNodes.
			srv.notifyNamespaceSubscribers()
			return
		}
	}
	srv.notifyNodeSubscribers(kaiNodes)
}

// metadataUpdatesSince returns shared, ordered diffs up to the current version.
// A false result means the cursor is too old and needs a full-state resync.
func (srv *KubeMetadataStreamServer) metadataUpdatesSince(version uint64) ([]*pb.KubeMetadataStreamResponse, uint64, bool) {
	srv.metadataMutex.RLock()
	defer srv.metadataMutex.RUnlock()

	if srv.metadataVersion-version > metadataHistorySize {
		return nil, srv.metadataVersion, false
	}
	if version == srv.metadataVersion {
		return nil, version, true
	}
	updates := make([]*pb.KubeMetadataStreamResponse, 0, srv.metadataVersion-version)
	for next := version + 1; next <= srv.metadataVersion; next++ {
		updates = append(updates, srv.metadataHistory[next%metadataHistorySize])
	}
	return updates, srv.metadataVersion, true
}

func (s *metadataSnapshot) processNamespaceEvent(eventType workloadmeta.EventType, metadata *workloadmeta.KubernetesMetadata) bool {
	namespaceName := metadata.Name

	switch eventType {
	case workloadmeta.EventTypeSet:
		s.namespaces[namespaceName] = namespaceEntry{
			labels:      metadata.Labels,
			annotations: metadata.Annotations,
		}
		return true
	case workloadmeta.EventTypeUnset:
		if _, exists := s.namespaces[namespaceName]; exists {
			delete(s.namespaces, namespaceName)
			return true
		}
	default:
		log.Errorf("Unknown event type %d for namespace %s", eventType, namespaceName)
	}
	return false
}

func (s *metadataSnapshot) processKueueQueueEvent(eventType workloadmeta.EventType, queue *workloadmeta.KubernetesKueueQueue) bool {
	key := queue.EntityID.ID
	switch eventType {
	case workloadmeta.EventTypeSet:
		s.kueueQueues[key] = kueueQueueEntry{
			namespace:        queue.Namespace,
			name:             queue.Name,
			queueType:        queue.QueueType,
			clusterQueueName: queue.ClusterQueueName,
			labels:           queue.Labels,
			annotations:      queue.Annotations,
			uid:              queue.UID,
		}
		return true
	case workloadmeta.EventTypeUnset:
		if _, exists := s.kueueQueues[key]; exists {
			delete(s.kueueQueues, key)
			return true
		}
	default:
		log.Errorf("Unknown event type %d for Kueue queue %s", eventType, key)
	}
	return false
}

func (s *metadataSnapshot) processKueueResourceFlavorEvent(eventType workloadmeta.EventType, flavor *workloadmeta.KubernetesKueueResourceFlavor) bool {
	key := flavor.EntityID.ID
	switch eventType {
	case workloadmeta.EventTypeSet:
		s.kueueResourceFlavors[key] = kueueResourceFlavorEntry{
			name:               flavor.Name,
			labels:             flavor.Labels,
			annotations:        flavor.Annotations,
			uid:                flavor.UID,
			nodeAffinityLabels: flavor.NodeAffinityLabels,
		}
		return true
	case workloadmeta.EventTypeUnset:
		if _, exists := s.kueueResourceFlavors[key]; exists {
			delete(s.kueueResourceFlavors, key)
			return true
		}
	default:
		log.Errorf("Unknown event type %d for Kueue resource flavor %s", eventType, key)
	}
	return false
}

func (s *metadataSnapshot) processKueueWorkloadEvent(eventType workloadmeta.EventType, workload *workloadmeta.KubernetesKueueWorkload) bool {
	key := workload.EntityID.ID
	switch eventType {
	case workloadmeta.EventTypeSet:
		s.kueueWorkloads[key] = kueueWorkloadEntry{
			namespace:         workload.Namespace,
			name:              workload.Name,
			queueName:         workload.QueueName,
			clusterQueueName:  workload.ClusterQueueName,
			labels:            workload.Labels,
			annotations:       workload.Annotations,
			uid:               workload.UID,
			podSetAssignments: kueuePodSetAssignmentEntries(workload.PodSetAssignments),
		}
		return true
	case workloadmeta.EventTypeUnset:
		if _, exists := s.kueueWorkloads[key]; exists {
			delete(s.kueueWorkloads, key)
			return true
		}
	case workloadmeta.EventTypeAll:
		log.Errorf("Unexpected event type %d for Kueue Workload %s", eventType, key)
	default:
		log.Errorf("Unknown event type %d for Kueue Workload %s", eventType, key)
	}
	return false
}

// processKAIPodGroupEvent applies the event and returns whether the PodGroup
// changed. Must be called with metadataMutex held.
func (srv *KubeMetadataStreamServer) processKAIPodGroupEvent(eventType workloadmeta.EventType, podGroup *workloadmeta.KubernetesKAIPodGroup) bool {
	key := podGroup.EntityID.ID
	switch eventType {
	case workloadmeta.EventTypeSet:
		entry := kaiPodGroupEntry{
			namespace:   podGroup.Namespace,
			name:        podGroup.Name,
			queueName:   podGroup.QueueName,
			labels:      podGroup.Labels,
			annotations: podGroup.Annotations,
			uid:         podGroup.UID,
		}
		if prev, exists := srv.kaiPodGroups[key]; exists && kaiPodGroupEqual(prev, entry) {
			return false
		}
		srv.kaiPodGroups[key] = entry
		return true
	case workloadmeta.EventTypeUnset:
		if _, exists := srv.kaiPodGroups[key]; exists {
			delete(srv.kaiPodGroups, key)
			return true
		}
	default:
		log.Errorf("Unknown event type %d for KAI PodGroup %s", eventType, key)
	}
	return false
}

// processKAIPodEvent updates the node index from the pod's node and KAI
// PodGroup annotation, and returns the nodes whose PodGroups may have changed.
// A pod that is not scheduled yet or has no PodGroup is not indexed. Must be
// called with metadataMutex held.
func (srv *KubeMetadataStreamServer) processKAIPodEvent(eventType workloadmeta.EventType, pod *workloadmeta.KubernetesPod) []string {
	podUID := pod.EntityID.ID
	prev, indexed := srv.kaiPods[podUID]

	var current kaiPodRef
	switch eventType {
	case workloadmeta.EventTypeSet:
		if podGroupName := pod.Annotations[kubernetes.KAIPodGroupNameAnnotationKey]; podGroupName != "" && pod.NodeName != "" {
			current = kaiPodRef{
				nodeName:    pod.NodeName,
				podGroupKey: workloadmeta.GenerateKAIPodGroupEntityID(pod.Namespace, podGroupName),
			}
		}
	case workloadmeta.EventTypeUnset:
	default:
		log.Errorf("Unknown event type %d for pod %s", eventType, podUID)
		return nil
	}

	if current == prev {
		return nil
	}

	var nodes []string
	if indexed {
		srv.removeKAIPodRef(prev)
		delete(srv.kaiPods, podUID)
		nodes = append(nodes, prev.nodeName)
	}
	if current.podGroupKey != "" {
		srv.addKAIPodRef(current)
		srv.kaiPods[podUID] = current
		nodes = append(nodes, current.nodeName)
	}
	return nodes
}

func (srv *KubeMetadataStreamServer) addKAIPodRef(ref kaiPodRef) {
	incrementNestedCount(srv.kaiNodePodGroups, ref.nodeName, ref.podGroupKey)
	incrementNestedCount(srv.kaiPodGroupNodes, ref.podGroupKey, ref.nodeName)
}

func (srv *KubeMetadataStreamServer) removeKAIPodRef(ref kaiPodRef) {
	decrementNestedCount(srv.kaiNodePodGroups, ref.nodeName, ref.podGroupKey)
	decrementNestedCount(srv.kaiPodGroupNodes, ref.podGroupKey, ref.nodeName)
}

func incrementNestedCount(counts map[string]map[string]int, outer, inner string) {
	if counts[outer] == nil {
		counts[outer] = make(map[string]int)
	}
	counts[outer][inner]++
}

func decrementNestedCount(counts map[string]map[string]int, outer, inner string) {
	counts[outer][inner]--
	if counts[outer][inner] <= 0 {
		delete(counts[outer], inner)
		if len(counts[outer]) == 0 {
			delete(counts, outer)
		}
	}
}

func kueuePodSetAssignmentEntries(assignments []workloadmeta.KueuePodSetAssignment) []kueuePodSetAssignmentEntry {
	entries := make([]kueuePodSetAssignmentEntry, 0, len(assignments))
	for _, assignment := range assignments {
		entries = append(entries, kueuePodSetAssignmentEntry{
			name:    assignment.Name,
			flavors: assignment.Flavors,
		})
	}
	return entries
}

func (srv *KubeMetadataStreamServer) notifyNamespaceSubscribers() {
	for _, channels := range srv.namespaceSubscribers {
		for _, ch := range channels {
			select {
			// Non-blocking send: if a signal is already pending, we drop it.
			// The consumer replays shared diffs since its last version, or
			// resyncs the full state if those diffs are no longer retained.
			case ch <- struct{}{}:
			default:
			}
		}
	}
}

// notifyNodeSubscribers notifies the subscribers of the given nodes. Must be
// called with metadataMutex held.
func (srv *KubeMetadataStreamServer) notifyNodeSubscribers(nodeNames map[string]struct{}) {
	for nodeName := range nodeNames {
		for _, ch := range srv.namespaceSubscribers[nodeName] {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}
}

func (srv *KubeMetadataStreamServer) subscribeToNamespaceEvents(nodeName string) <-chan struct{} {
	srv.metadataMutex.Lock()
	defer srv.metadataMutex.Unlock()

	ch := make(chan struct{}, 1)
	srv.namespaceSubscribers[nodeName] = append(srv.namespaceSubscribers[nodeName], ch)

	log.Debugf("Subscribed to namespace metadata updates for node %s (subscribers=%d)",
		nodeName,
		len(srv.namespaceSubscribers[nodeName]))

	return ch
}

func (srv *KubeMetadataStreamServer) unsubscribeFromNamespaceEvents(nodeName string, ch <-chan struct{}) {
	srv.metadataMutex.Lock()
	defer srv.metadataMutex.Unlock()

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
	return srv.buildMetadataSnapshot().namespaces
}

func (srv *KubeMetadataStreamServer) buildKueueQueuesSnapshot() map[string]kueueQueueEntry {
	return srv.buildMetadataSnapshot().kueueQueues
}

func (srv *KubeMetadataStreamServer) buildKueueResourceFlavorsSnapshot() map[string]kueueResourceFlavorEntry {
	return srv.buildMetadataSnapshot().kueueResourceFlavors
}

func (srv *KubeMetadataStreamServer) buildKueueWorkloadsSnapshot() map[string]kueueWorkloadEntry {
	return srv.buildMetadataSnapshot().kueueWorkloads
}

func (srv *KubeMetadataStreamServer) buildMetadataSnapshot() metadataSnapshot {
	snapshot, _ := srv.buildMetadataSnapshotWithVersion()
	return snapshot
}

func (srv *KubeMetadataStreamServer) buildMetadataSnapshotWithVersion() (metadataSnapshot, uint64) {
	srv.metadataMutex.RLock()
	defer srv.metadataMutex.RUnlock()

	return srv.metadata.clone(), srv.metadataVersion
}

// buildKAIPodGroupsSnapshot returns the KAI PodGroups of the pods scheduled on
// a node, keyed by workloadmeta entity ID.
func (srv *KubeMetadataStreamServer) buildKAIPodGroupsSnapshot(nodeName string) map[string]kaiPodGroupEntry {
	srv.metadataMutex.RLock()
	defer srv.metadataMutex.RUnlock()

	podGroupKeys := srv.kaiNodePodGroups[nodeName]
	snapshot := make(map[string]kaiPodGroupEntry, len(podGroupKeys))
	for key := range podGroupKeys {
		// A pod can reference a PodGroup that is not known yet, or already deleted.
		if entry, exists := srv.kaiPodGroups[key]; exists {
			snapshot[key] = entry
		}
	}
	return snapshot
}

func (s metadataSnapshot) clone() metadataSnapshot {
	snapshot := newMetadataSnapshot()
	maps.Copy(snapshot.namespaces, s.namespaces)
	maps.Copy(snapshot.kueueQueues, s.kueueQueues)
	maps.Copy(snapshot.kueueResourceFlavors, s.kueueResourceFlavors)
	maps.Copy(snapshot.kueueWorkloads, s.kueueWorkloads)
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

func kubeMetadataStreamFilter(kaiEnabled bool) *workloadmeta.Filter {
	builder := workloadmeta.NewFilterBuilder().AddKindWithEntityFilter(
		workloadmeta.KindKubernetesMetadata,
		func(entity workloadmeta.Entity) bool {
			metadata := entity.(*workloadmeta.KubernetesMetadata)
			return workloadmeta.IsNamespaceMetadata(metadata)
		},
	).AddKind(workloadmeta.KindKubernetesKueueQueue).AddKind(workloadmeta.KindKubernetesKueueResourceFlavor).AddKind(workloadmeta.KindKubernetesKueueWorkload)
	if kaiEnabled {
		// Pods map each KAI PodGroup to the nodes that need it.
		builder = builder.AddKind(workloadmeta.KindKubernetesKAIPodGroup).AddKind(workloadmeta.KindKubernetesPod)
	}
	return builder.Build()
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
// is_full_state=true containing all current mappings and metadata.
func fullStateResponse(podServices map[string]podServiceEntry, metadata metadataSnapshot, kaiPodGroups map[string]kaiPodGroupEntry) *pb.KubeMetadataStreamResponse {
	mappings := make([]*pb.PodServiceMapping, 0, len(podServices))
	for _, entry := range podServices {
		mappings = append(mappings, &pb.PodServiceMapping{
			Namespace:    entry.namespace,
			PodName:      entry.podName,
			ServiceNames: sets.List(entry.services),
			Type:         pb.KubeMetadataEventType_SET,
		})
	}

	resp := computeMetadataDiff(newMetadataSnapshot(), metadata).response(true)
	resp.Mappings = mappings
	resp.KaiPodGroups = computeKAIPodGroupsDiff(nil, kaiPodGroups)
	return resp
}

type metadataDiff struct {
	namespaces           []*pb.NamespaceMetadata
	kueueQueues          []*pb.KueueQueue
	kueueResourceFlavors []*pb.KueueResourceFlavor
	kueueWorkloads       []*pb.KueueWorkload
}

func computeMetadataDiff(old, current metadataSnapshot) metadataDiff {
	return metadataDiff{
		namespaces:           computeNamespacesDiff(old.namespaces, current.namespaces),
		kueueQueues:          computeKueueQueueDiff(old.kueueQueues, current.kueueQueues),
		kueueResourceFlavors: computeKueueResourceFlavorDiff(old.kueueResourceFlavors, current.kueueResourceFlavors),
		kueueWorkloads:       computeKueueWorkloadDiff(old.kueueWorkloads, current.kueueWorkloads),
	}
}

func (d metadataDiff) isEmpty() bool {
	return len(d.namespaces)+len(d.kueueQueues)+len(d.kueueResourceFlavors)+len(d.kueueWorkloads) == 0
}

func (d metadataDiff) response(isFullState bool) *pb.KubeMetadataStreamResponse {
	return &pb.KubeMetadataStreamResponse{
		IsFullState:          isFullState,
		NamespaceMetadata:    d.namespaces,
		KueueQueues:          d.kueueQueues,
		KueueResourceFlavors: d.kueueResourceFlavors,
		KueueWorkloads:       d.kueueWorkloads,
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

func computeKueueQueueDiff(old, current map[string]kueueQueueEntry) []*pb.KueueQueue {
	var diff []*pb.KueueQueue

	for key, cur := range current {
		prev, existed := old[key]
		if !existed || !kueueQueueEqual(prev, cur) {
			diff = append(diff, protoKueueQueue(cur, pb.KubeMetadataEventType_SET))
		}
	}

	for key, prev := range old {
		if _, exists := current[key]; !exists {
			diff = append(diff, protoKueueQueue(prev, pb.KubeMetadataEventType_UNSET))
		}
	}

	return diff
}

func computeKueueResourceFlavorDiff(old, current map[string]kueueResourceFlavorEntry) []*pb.KueueResourceFlavor {
	var diff []*pb.KueueResourceFlavor

	for key, cur := range current {
		prev, existed := old[key]
		if !existed || !kueueResourceFlavorEqual(prev, cur) {
			diff = append(diff, protoKueueResourceFlavor(cur, pb.KubeMetadataEventType_SET))
		}
	}

	for key, prev := range old {
		if _, exists := current[key]; !exists {
			diff = append(diff, protoKueueResourceFlavor(prev, pb.KubeMetadataEventType_UNSET))
		}
	}

	return diff
}

func computeKueueWorkloadDiff(old, current map[string]kueueWorkloadEntry) []*pb.KueueWorkload {
	var diff []*pb.KueueWorkload

	for key, cur := range current {
		prev, existed := old[key]
		if !existed || !kueueWorkloadEqual(prev, cur) {
			diff = append(diff, protoKueueWorkload(cur, pb.KubeMetadataEventType_SET))
		}
	}

	for key, prev := range old {
		if _, exists := current[key]; !exists {
			diff = append(diff, protoKueueWorkload(prev, pb.KubeMetadataEventType_UNSET))
		}
	}

	return diff
}

// computeKAIPodGroupsDiff compares old and new KAI PodGroup snapshots and returns set/unset events.
func computeKAIPodGroupsDiff(old, current map[string]kaiPodGroupEntry) []*pb.KAIPodGroup {
	var diff []*pb.KAIPodGroup

	for key, cur := range current {
		prev, existed := old[key]
		if !existed || !kaiPodGroupEqual(prev, cur) {
			diff = append(diff, protoKAIPodGroup(cur, pb.KubeMetadataEventType_SET))
		}
	}

	for key, prev := range old {
		if _, exists := current[key]; !exists {
			diff = append(diff, protoKAIPodGroup(prev, pb.KubeMetadataEventType_UNSET))
		}
	}

	return diff
}

func protoKueueQueue(entry kueueQueueEntry, eventType pb.KubeMetadataEventType) *pb.KueueQueue {
	return &pb.KueueQueue{
		Namespace:    entry.namespace,
		Name:         entry.name,
		QueueType:    protoKueueQueueType(entry.queueType),
		ClusterQueue: entry.clusterQueueName,
		Labels:       entry.labels,
		Annotations:  entry.annotations,
		Uid:          entry.uid,
		Type:         eventType,
	}
}

func protoKueueResourceFlavor(entry kueueResourceFlavorEntry, eventType pb.KubeMetadataEventType) *pb.KueueResourceFlavor {
	return &pb.KueueResourceFlavor{
		Name:               entry.name,
		Labels:             entry.labels,
		Annotations:        entry.annotations,
		Uid:                entry.uid,
		NodeAffinityLabels: entry.nodeAffinityLabels,
		Type:               eventType,
	}
}

func protoKueueWorkload(entry kueueWorkloadEntry, eventType pb.KubeMetadataEventType) *pb.KueueWorkload {
	return &pb.KueueWorkload{
		Namespace:         entry.namespace,
		Name:              entry.name,
		Queue:             entry.queueName,
		ClusterQueue:      entry.clusterQueueName,
		Labels:            entry.labels,
		Annotations:       entry.annotations,
		Uid:               entry.uid,
		PodSetAssignments: protoKueuePodSetAssignments(entry.podSetAssignments),
		Type:              eventType,
	}
}

func protoKAIPodGroup(entry kaiPodGroupEntry, eventType pb.KubeMetadataEventType) *pb.KAIPodGroup {
	return &pb.KAIPodGroup{
		Namespace:   entry.namespace,
		Name:        entry.name,
		Queue:       entry.queueName,
		Labels:      entry.labels,
		Annotations: entry.annotations,
		Uid:         entry.uid,
		Type:        eventType,
	}
}

func protoKueuePodSetAssignments(assignments []kueuePodSetAssignmentEntry) []*pb.KueuePodSetAssignment {
	if assignments == nil {
		return nil
	}

	protoAssignments := make([]*pb.KueuePodSetAssignment, 0, len(assignments))
	for _, assignment := range assignments {
		protoAssignments = append(protoAssignments, &pb.KueuePodSetAssignment{
			Name:    assignment.name,
			Flavors: assignment.flavors,
		})
	}
	return protoAssignments
}

func kueueQueueEqual(left, right kueueQueueEntry) bool {
	return left.namespace == right.namespace &&
		left.name == right.name &&
		left.queueType == right.queueType &&
		left.clusterQueueName == right.clusterQueueName &&
		left.uid == right.uid &&
		maps.Equal(left.labels, right.labels) &&
		maps.Equal(left.annotations, right.annotations)
}

func kueueResourceFlavorEqual(left, right kueueResourceFlavorEntry) bool {
	return left.name == right.name &&
		left.uid == right.uid &&
		maps.Equal(left.labels, right.labels) &&
		maps.Equal(left.annotations, right.annotations) &&
		maps.Equal(left.nodeAffinityLabels, right.nodeAffinityLabels)
}

func kueueWorkloadEqual(left, right kueueWorkloadEntry) bool {
	return left.namespace == right.namespace &&
		left.name == right.name &&
		left.queueName == right.queueName &&
		left.clusterQueueName == right.clusterQueueName &&
		left.uid == right.uid &&
		maps.Equal(left.labels, right.labels) &&
		maps.Equal(left.annotations, right.annotations) &&
		kueuePodSetAssignmentsEqual(left.podSetAssignments, right.podSetAssignments)
}

func kaiPodGroupEqual(left, right kaiPodGroupEntry) bool {
	return left.namespace == right.namespace &&
		left.name == right.name &&
		left.queueName == right.queueName &&
		left.uid == right.uid &&
		maps.Equal(left.labels, right.labels) &&
		maps.Equal(left.annotations, right.annotations)
}

func kueuePodSetAssignmentsEqual(left, right []kueuePodSetAssignmentEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].name != right[i].name || !maps.Equal(left[i].flavors, right[i].flavors) {
			return false
		}
	}
	return true
}

func protoKueueQueueType(queueType workloadmeta.KueueQueueType) pb.KueueQueueType {
	switch queueType {
	case workloadmeta.KueueClusterQueue:
		return pb.KueueQueueType_CLUSTER_QUEUE
	default:
		return pb.KueueQueueType_LOCAL_QUEUE
	}
}
