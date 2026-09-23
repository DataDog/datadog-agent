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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
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

type metadataSnapshot struct {
	namespaces           map[string]namespaceEntry
	kueueQueues          map[string]kueueQueueEntry
	kueueResourceFlavors map[string]kueueResourceFlavorEntry
	kueueWorkloads       map[string]kueueWorkloadEntry
	// workloadAutoscalers holds the autoscaler kinds acting on each workload
	// that has at least one. The sets are never mutated once stored, only
	// replaced, so a shallow copy of the map is a safe snapshot.
	workloadAutoscalers map[kubernetes.WorkloadTarget]sets.Set[string]
}

// KubeMetadataStreamServer streams pod-to-service mappings and namespace
// labels/annotations from the DCA to node agents.
type KubeMetadataStreamServer struct {
	store *controllers.MetaBundleStore
	wmeta workloadmeta.Component

	metadataMutex sync.RWMutex
	metadata      metadataSnapshot
	// namespaceSubscribers holds notification channels per node name. A node
	// can have multiple subscribers because more than one process (for example,
	// the running agent plus "agent diagnose", "agent check", etc.) may stream
	// metadata for the same node concurrently.
	namespaceSubscribers map[string][]chan struct{}

	// Per-node workload filtering, see WithPerNodeWorkloadFiltering.
	//
	// workloadNodes counts, for every workload, its pods bound to each node.
	// It covers all workloads, not only autoscaled ones, so that a workload
	// gaining an autoscaler notifies exactly the nodes already hosting it.
	// Guarded by metadataMutex, like podPlacements.
	waitPodPlacementSynced func(context.Context) bool
	filterByNode           atomic.Bool
	podPlacements          map[string]podPlacement // pod UID -> placement
	workloadNodes          map[kubernetes.WorkloadTarget]map[string]int
}

// podPlacement is where a pod runs and which workload owns it.
type podPlacement struct {
	node   string
	target kubernetes.WorkloadTarget
}

// KubeMetadataStreamServerOption configures a KubeMetadataStreamServer.
type KubeMetadataStreamServerOption func(*KubeMetadataStreamServer)

// WithPerNodeWorkloadFiltering makes the server send each node only the
// autoscalers of workloads that have a pod bound to that node, like pod to
// service mappings, instead of every autoscaled workload in the cluster.
//
// Filtering needs to know where every pod runs, so it only starts once
// waitSynced reports the Cluster Agent's pod collection has synced. Before
// that, and without this option, workload autoscalers are sent cluster-wide:
// filtering on incomplete placement data would starve nodes.
func WithPerNodeWorkloadFiltering(waitSynced func(context.Context) bool) KubeMetadataStreamServerOption {
	return func(srv *KubeMetadataStreamServer) {
		srv.waitPodPlacementSynced = waitSynced
	}
}

// NewKubeMetadataStreamServer creates a new KubeMetadataStreamServer
func NewKubeMetadataStreamServer(store *controllers.MetaBundleStore, wmeta workloadmeta.Component, opts ...KubeMetadataStreamServerOption) *KubeMetadataStreamServer {
	srv := &KubeMetadataStreamServer{
		store:                store,
		wmeta:                wmeta,
		metadata:             newMetadataSnapshot(),
		namespaceSubscribers: make(map[string][]chan struct{}),
		podPlacements:        make(map[string]podPlacement),
		workloadNodes:        make(map[kubernetes.WorkloadTarget]map[string]int),
	}
	for _, opt := range opts {
		opt(srv)
	}
	return srv
}

func newMetadataSnapshot() metadataSnapshot {
	return metadataSnapshot{
		namespaces:           make(map[string]namespaceEntry),
		kueueQueues:          make(map[string]kueueQueueEntry),
		kueueResourceFlavors: make(map[string]kueueResourceFlavorEntry),
		kueueWorkloads:       make(map[string]kueueWorkloadEntry),
		workloadAutoscalers:  make(map[kubernetes.WorkloadTarget]sets.Set[string]),
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

	if srv.waitPodPlacementSynced != nil {
		srv.startPodPlacementTracking(ctx)
	}
}

// startPodPlacementTracking follows where pods run, and switches to per-node
// filtering once the pod collection has synced.
func (srv *KubeMetadataStreamServer) startPodPlacementTracking(ctx context.Context) {
	// Only the Kubernetes API server source: it carries the node and the
	// owners. The autoscaler collector also writes pods, but only their kinds.
	ch := srv.wmeta.Subscribe(
		wmetaSubscriberName+"-pods",
		workloadmeta.NormalPriority,
		workloadmeta.NewFilterBuilder().
			SetSource(workloadmeta.SourceKubeAPIServer).
			AddKind(workloadmeta.KindKubernetesPod).
			Build(),
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
				srv.processPodEvents(bundle.Events)
			}
		}
	}()

	go func() {
		if !srv.waitPodPlacementSynced(ctx) {
			return
		}
		srv.metadataMutex.Lock()
		defer srv.metadataMutex.Unlock()
		srv.filterByNode.Store(true)
		log.Infof("Pod collection synced: sending each node only the autoscalers of its own workloads")
		// Every stream re-diffs against what it last sent: the workloads of
		// other nodes go out as UNSETs, so no full state is needed.
		srv.notifyNamespaceSubscribers()
	}()
}

// processPodEvents maintains workloadNodes, and notifies the nodes whose view
// of autoscaled workloads changed.
func (srv *KubeMetadataStreamServer) processPodEvents(events []workloadmeta.Event) {
	srv.metadataMutex.Lock()
	defer srv.metadataMutex.Unlock()

	affectedNodes := sets.New[string]()
	for _, event := range events {
		pod, ok := event.Entity.(*workloadmeta.KubernetesPod)
		if !ok {
			continue
		}

		// A pod counts once it is bound to a node and owned by a workload.
		var next podPlacement
		placed := false
		if event.Type == workloadmeta.EventTypeSet && pod.NodeName != "" {
			if target, found := util.PodWorkloadTarget(pod); found {
				next, placed = podPlacement{node: pod.NodeName, target: target}, true
			}
		}

		previous, known := srv.podPlacements[pod.ID]
		if known && placed && previous == next {
			continue
		}
		if known {
			delete(srv.podPlacements, pod.ID)
			if srv.removePlacementLocked(previous) {
				affectedNodes.Insert(previous.node)
			}
		}
		if placed {
			srv.podPlacements[pod.ID] = next
			if srv.addPlacementLocked(next) {
				affectedNodes.Insert(next.node)
			}
		}
	}

	if srv.filterByNode.Load() {
		srv.notifyNodeSubscribersLocked(affectedNodes)
	}
}

// addPlacementLocked records a pod of a workload on a node. It reports whether
// that node's view changed: the workload is autoscaled and was not on it yet.
func (srv *KubeMetadataStreamServer) addPlacementLocked(p podPlacement) bool {
	nodes := srv.workloadNodes[p.target]
	if nodes == nil {
		nodes = make(map[string]int)
		srv.workloadNodes[p.target] = nodes
	}
	nodes[p.node]++
	_, autoscaled := srv.metadata.workloadAutoscalers[p.target]
	return nodes[p.node] == 1 && autoscaled
}

// removePlacementLocked forgets a pod of a workload on a node. It reports
// whether that node's view changed: the workload is autoscaled and this was
// its last pod there.
func (srv *KubeMetadataStreamServer) removePlacementLocked(p podPlacement) bool {
	nodes := srv.workloadNodes[p.target]
	if nodes[p.node] == 0 {
		return false
	}
	nodes[p.node]--
	if nodes[p.node] > 0 {
		return false
	}
	delete(nodes, p.node)
	if len(nodes) == 0 {
		delete(srv.workloadNodes, p.target)
	}
	_, autoscaled := srv.metadata.workloadAutoscalers[p.target]
	return autoscaled
}

// notifyNodeSubscribersLocked signals the streams of the given nodes only.
func (srv *KubeMetadataStreamServer) notifyNodeSubscribersLocked(nodes sets.Set[string]) {
	for node := range nodes {
		for _, ch := range srv.namespaceSubscribers[node] {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}
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
	lastSentMetadataState := srv.buildMetadataSnapshotForNode(nodeName)
	log.Debugf("Sending kube metadata full state to node %s: %d workload autoscalers (filtered by node: %t)",
		nodeName, len(lastSentMetadataState.workloadAutoscalers), srv.filterByNode.Load())
	initialResp := fullStateResponse(lastSentPodServicesState, lastSentMetadataState)
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
			sendSpan := tracer.StartSpan("cluster_agent.metadata_stream.send_diff",
				tracer.ResourceName("sendDiff"),
				tracer.Tag(ext.SpanKind, ext.SpanKindServer),
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
			currentMetadataState := srv.buildMetadataSnapshotForNode(nodeName)
			metadataDiff := computeMetadataDiff(lastSentMetadataState, currentMetadataState)
			if metadataDiff.isEmpty() {
				continue
			}
			resp := metadataDiff.response(false)
			sendSpan := tracer.StartSpan("cluster_agent.metadata_stream.send_diff",
				tracer.ResourceName("sendDiff"),
				tracer.Tag(ext.SpanKind, ext.SpanKindServer),
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
			lastSentMetadataState = currentMetadataState
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
	srv.metadataMutex.Lock()
	defer srv.metadataMutex.Unlock()

	// Namespace and Kueue changes concern every node. Workload autoscaler
	// changes only concern the nodes hosting the workload, once filtering by
	// node is on.
	changed := false
	var changedWorkloads []kubernetes.WorkloadTarget
	for _, event := range events {
		switch entity := event.Entity.(type) {
		case *workloadmeta.KubernetesMetadata:
			// Namespaces and workloads represented as generic metadata
			// (StatefulSets, Argo Rollouts) share the entity kind.
			if target, isWorkload := util.WorkloadTargetFromMetadata(entity); isWorkload {
				if srv.metadata.processWorkloadAutoscalersEvent(event.Type, target, entity.AutoscalerKinds) {
					changedWorkloads = append(changedWorkloads, target)
				}
			} else if srv.metadata.processNamespaceEvent(event.Type, entity) {
				changed = true
			}
		case *workloadmeta.KubernetesKueueQueue:
			if srv.metadata.processKueueQueueEvent(event.Type, entity) {
				changed = true
			}
		case *workloadmeta.KubernetesKueueResourceFlavor:
			if srv.metadata.processKueueResourceFlavorEvent(event.Type, entity) {
				changed = true
			}
		case *workloadmeta.KubernetesKueueWorkload:
			if srv.metadata.processKueueWorkloadEvent(event.Type, entity) {
				changed = true
			}
		case *workloadmeta.KubernetesDeployment:
			if target, workloadChanged := srv.metadata.processDeploymentEvent(event.Type, entity); workloadChanged {
				changedWorkloads = append(changedWorkloads, target)
			}
		default:
			log.Errorf("Unexpected workloadmeta entity %T in kube metadata stream", event.Entity)
		}
	}

	switch {
	case changed || (len(changedWorkloads) > 0 && !srv.filterByNode.Load()):
		srv.notifyNamespaceSubscribers()
	case len(changedWorkloads) > 0:
		nodes := sets.New[string]()
		for _, target := range changedWorkloads {
			for node := range srv.workloadNodes[target] {
				nodes.Insert(node)
			}
		}
		srv.notifyNodeSubscribersLocked(nodes)
	}
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

// processDeploymentEvent tracks the autoscaler kinds acting on a Deployment.
func (s *metadataSnapshot) processDeploymentEvent(eventType workloadmeta.EventType, deployment *workloadmeta.KubernetesDeployment) (kubernetes.WorkloadTarget, bool) {
	// Taken from the entity ID ("namespace/name"), not from EntityMeta: an
	// Unset for a deleted Deployment carries only its ID, and a Deployment
	// known only through its autoscalers has no metadata at all.
	namespace, name, ok := strings.Cut(deployment.EntityID.ID, "/")
	if !ok {
		log.Errorf("Unexpected Deployment entity ID %q in kube metadata stream", deployment.EntityID.ID)
		return kubernetes.WorkloadTarget{}, false
	}
	target := kubernetes.WorkloadTarget{Kind: kubernetes.DeploymentKind, Namespace: namespace, Name: name}
	return target, s.processWorkloadAutoscalersEvent(eventType, target, deployment.AutoscalerKinds)
}

// processWorkloadAutoscalersEvent tracks the autoscaler kinds acting on a
// workload, whichever entity kind carries them.
//
// Unlike the other entity kinds here, it reports a change only when the set of
// kinds actually changed: workloads are far more numerous than Kueue objects,
// and every reported change wakes up the stream of every node.
func (s *metadataSnapshot) processWorkloadAutoscalersEvent(eventType workloadmeta.EventType, target kubernetes.WorkloadTarget, kinds sets.Set[string]) bool {
	switch eventType {
	case workloadmeta.EventTypeSet:
	case workloadmeta.EventTypeUnset:
		kinds = nil
	default:
		log.Errorf("Unknown event type %d for workload %s/%s/%s", eventType, target.Kind, target.Namespace, target.Name)
		return false
	}

	previous, exists := s.workloadAutoscalers[target]
	if kinds.Len() == 0 {
		if !exists {
			return false
		}
		delete(s.workloadAutoscalers, target)
		return true
	}
	if exists && previous.Equal(kinds) {
		return false
	}
	s.workloadAutoscalers[target] = kinds.Clone()
	return true
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
			// This is safe because the consumer re-reads the full state from
			// the store on each signal.
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

// buildMetadataSnapshotForNode is what a node's stream sends: the cluster-wide
// snapshot, with workload autoscalers restricted to the workloads that have a
// pod on the node once filtering by node is on.
func (srv *KubeMetadataStreamServer) buildMetadataSnapshotForNode(nodeName string) metadataSnapshot {
	snapshot := srv.buildMetadataSnapshot()
	if !srv.filterByNode.Load() {
		return snapshot
	}

	srv.metadataMutex.RLock()
	defer srv.metadataMutex.RUnlock()
	for target := range snapshot.workloadAutoscalers {
		if srv.workloadNodes[target][nodeName] == 0 {
			delete(snapshot.workloadAutoscalers, target)
		}
	}
	return snapshot
}

func (srv *KubeMetadataStreamServer) buildMetadataSnapshot() metadataSnapshot {
	srv.metadataMutex.RLock()
	defer srv.metadataMutex.RUnlock()

	snapshot := newMetadataSnapshot()
	maps.Copy(snapshot.namespaces, srv.metadata.namespaces)
	maps.Copy(snapshot.kueueQueues, srv.metadata.kueueQueues)
	maps.Copy(snapshot.kueueResourceFlavors, srv.metadata.kueueResourceFlavors)
	maps.Copy(snapshot.kueueWorkloads, srv.metadata.kueueWorkloads)
	maps.Copy(snapshot.workloadAutoscalers, srv.metadata.workloadAutoscalers)
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
			// Workloads are recognised by resource, never by "has autoscaler
			// kinds": that would drop exactly the event that clears them.
			_, isWorkload := util.WorkloadTargetFromMetadata(metadata)
			return workloadmeta.IsNamespaceMetadata(metadata) || isWorkload
		},
	).AddKind(workloadmeta.KindKubernetesKueueQueue).AddKind(workloadmeta.KindKubernetesKueueResourceFlavor).AddKind(workloadmeta.KindKubernetesKueueWorkload).
		// No entity filter on Deployments: filtering on "has autoscaler kinds"
		// would drop exactly the event that clears them.
		AddKind(workloadmeta.KindKubernetesDeployment).Build()
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
func fullStateResponse(podServices map[string]podServiceEntry, metadata metadataSnapshot) *pb.KubeMetadataStreamResponse {
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
	return resp
}

type metadataDiff struct {
	namespaces           []*pb.NamespaceMetadata
	kueueQueues          []*pb.KueueQueue
	kueueResourceFlavors []*pb.KueueResourceFlavor
	kueueWorkloads       []*pb.KueueWorkload
	workloadAutoscalers  []*pb.WorkloadAutoscalers
}

func computeMetadataDiff(old, current metadataSnapshot) metadataDiff {
	return metadataDiff{
		namespaces:           computeNamespacesDiff(old.namespaces, current.namespaces),
		kueueQueues:          computeKueueQueueDiff(old.kueueQueues, current.kueueQueues),
		kueueResourceFlavors: computeKueueResourceFlavorDiff(old.kueueResourceFlavors, current.kueueResourceFlavors),
		kueueWorkloads:       computeKueueWorkloadDiff(old.kueueWorkloads, current.kueueWorkloads),
		workloadAutoscalers:  computeWorkloadAutoscalersDiff(old.workloadAutoscalers, current.workloadAutoscalers),
	}
}

func (d metadataDiff) isEmpty() bool {
	return len(d.namespaces)+len(d.kueueQueues)+len(d.kueueResourceFlavors)+len(d.kueueWorkloads)+len(d.workloadAutoscalers) == 0
}

func (d metadataDiff) response(isFullState bool) *pb.KubeMetadataStreamResponse {
	return &pb.KubeMetadataStreamResponse{
		IsFullState:          isFullState,
		NamespaceMetadata:    d.namespaces,
		KueueQueues:          d.kueueQueues,
		KueueResourceFlavors: d.kueueResourceFlavors,
		KueueWorkloads:       d.kueueWorkloads,
		WorkloadAutoscalers:  d.workloadAutoscalers,
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

func computeWorkloadAutoscalersDiff(old, current map[kubernetes.WorkloadTarget]sets.Set[string]) []*pb.WorkloadAutoscalers {
	var diff []*pb.WorkloadAutoscalers

	for target, kinds := range current {
		if prev, existed := old[target]; !existed || !prev.Equal(kinds) {
			diff = append(diff, protoWorkloadAutoscalers(target, kinds, pb.KubeMetadataEventType_SET))
		}
	}

	for target := range old {
		if _, exists := current[target]; !exists {
			diff = append(diff, protoWorkloadAutoscalers(target, nil, pb.KubeMetadataEventType_UNSET))
		}
	}

	return diff
}

func protoWorkloadAutoscalers(target kubernetes.WorkloadTarget, kinds sets.Set[string], eventType pb.KubeMetadataEventType) *pb.WorkloadAutoscalers {
	return &pb.WorkloadAutoscalers{
		Namespace: target.Namespace,
		Kind:      target.Kind,
		Name:      target.Name,
		// Sorted, so the same set always serializes the same way.
		AutoscalerKinds: sets.List(kinds),
		Type:            eventType,
	}
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
