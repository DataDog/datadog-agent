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
	"k8s.io/client-go/util/workqueue"

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
	keepAliveInterval       = 9 * time.Minute
	wmetaSubscriberName     = "kube-metadata-stream"
	podOwnersSubscriberName = "kube-metadata-stream-pod-owners"
	podOwnerMaxRetries      = 3
	podOwnerResolveTimeout  = 10 * time.Second
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

// PodOwnerResolver resolves the configured owners for a Pod.
type PodOwnerResolver interface {
	Resolve(context.Context, *workloadmeta.KubernetesPod) ([]workloadmeta.KubernetesMatchedOwner, error)
}

type podMatchedOwnersEntry struct {
	namespace string
	podName   string
	owners    []workloadmeta.KubernetesMatchedOwner
}

type matchedOwnersSnapshot map[string]podMatchedOwnersEntry

type metadataSnapshot struct {
	namespaces           map[string]namespaceEntry
	kueueQueues          map[string]kueueQueueEntry
	kueueResourceFlavors map[string]kueueResourceFlavorEntry
	kueueWorkloads       map[string]kueueWorkloadEntry
	matchedOwners        matchedOwnersSnapshot
}

// KubeMetadataStreamServer streams pod-to-service mappings and namespace
// labels/annotations from the DCA to node agents.
type KubeMetadataStreamServer struct {
	store    *controllers.MetaBundleStore
	wmeta    workloadmeta.Component
	resolver PodOwnerResolver

	metadataMutex sync.RWMutex
	metadata      metadataSnapshot
	matchedOwners map[string]matchedOwnersSnapshot

	podEventsMutex sync.Mutex
	podEvents      map[string]workloadmeta.Event
	podOwnerQueue  workqueue.TypedRateLimitingInterface[string]
	// namespaceSubscribers holds notification channels per node name. A node
	// can have multiple subscribers because more than one process (for example,
	// the running agent plus "agent diagnose", "agent check", etc.) may stream
	// metadata for the same node concurrently.
	namespaceSubscribers map[string][]chan struct{}
}

// NewKubeMetadataStreamServer creates a new KubeMetadataStreamServer
func NewKubeMetadataStreamServer(store *controllers.MetaBundleStore, wmeta workloadmeta.Component, resolver PodOwnerResolver) *KubeMetadataStreamServer {
	return &KubeMetadataStreamServer{
		store:         store,
		wmeta:         wmeta,
		resolver:      resolver,
		metadata:      newMetadataSnapshot(),
		matchedOwners: make(map[string]matchedOwnersSnapshot),
		podEvents:     make(map[string]workloadmeta.Event),
		podOwnerQueue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[string](),
			workqueue.TypedRateLimitingQueueConfig[string]{Name: podOwnersSubscriberName},
		),
		namespaceSubscribers: make(map[string][]chan struct{}),
	}
}

func newMetadataSnapshot() metadataSnapshot {
	return metadataSnapshot{
		namespaces:           make(map[string]namespaceEntry),
		kueueQueues:          make(map[string]kueueQueueEntry),
		kueueResourceFlavors: make(map[string]kueueResourceFlavorEntry),
		kueueWorkloads:       make(map[string]kueueWorkloadEntry),
		matchedOwners:        make(matchedOwnersSnapshot),
	}
}

// Start subscribes to workloadmeta for metadata changes and maintains state.
// It must be called before serving streams.
func (srv *KubeMetadataStreamServer) Start(ctx context.Context) {
	metadataCh := srv.wmeta.Subscribe(
		wmetaSubscriberName,
		workloadmeta.NormalPriority,
		kubeMetadataStreamFilter(),
	)

	go func() {
		defer srv.wmeta.Unsubscribe(metadataCh)
		for {
			select {
			case <-ctx.Done():
				return
			case bundle, ok := <-metadataCh:
				if !ok {
					return
				}
				bundle.Acknowledge()
				srv.processMetadataEvents(bundle.Events)
			}
		}
	}()

	if srv.resolver == nil {
		return
	}
	srv.startPodOwnerResolution(ctx)
}

func (srv *KubeMetadataStreamServer) startPodOwnerResolution(ctx context.Context) {
	podCh := srv.wmeta.Subscribe(
		podOwnersSubscriberName,
		workloadmeta.NormalPriority,
		workloadmeta.NewFilterBuilder().AddKind(workloadmeta.KindKubernetesPod).Build(),
	)

	go func() {
		defer srv.wmeta.Unsubscribe(podCh)
		for {
			select {
			case <-ctx.Done():
				return
			case bundle, ok := <-podCh:
				if !ok {
					return
				}
				bundle.Acknowledge()
				srv.enqueuePodEvents(bundle.Events)
			}
		}
	}()

	go func() {
		<-ctx.Done()
		srv.podOwnerQueue.ShutDown()
	}()

	go srv.runPodOwnerWorker(ctx)
}

func (srv *KubeMetadataStreamServer) enqueuePodEvents(events []workloadmeta.Event) {
	srv.podEventsMutex.Lock()
	defer srv.podEventsMutex.Unlock()

	for _, event := range events {
		pod := event.Entity.(*workloadmeta.KubernetesPod)
		key := pod.Namespace + "/" + pod.Name
		srv.podEvents[key] = event
		srv.podOwnerQueue.Add(key)
	}
}

func (srv *KubeMetadataStreamServer) runPodOwnerWorker(ctx context.Context) {
	for srv.processNextPodEvent(ctx) {
	}
}

func (srv *KubeMetadataStreamServer) processNextPodEvent(ctx context.Context) bool {
	key, shutdown := srv.podOwnerQueue.Get()
	if shutdown {
		return false
	}
	defer srv.podOwnerQueue.Done(key)

	srv.podEventsMutex.Lock()
	pending, found := srv.podEvents[key]
	delete(srv.podEvents, key)
	srv.podEventsMutex.Unlock()
	if !found {
		srv.podOwnerQueue.Forget(key)
		return true
	}

	pod := pending.Entity.(*workloadmeta.KubernetesPod)
	var owners []workloadmeta.KubernetesMatchedOwner
	if pending.Type == workloadmeta.EventTypeSet && pod.NodeName != "" {
		resolveCtx, cancel := context.WithTimeout(ctx, podOwnerResolveTimeout)
		resolved, err := srv.resolver.Resolve(resolveCtx, pod)
		cancel()
		owners = resolved
		if err != nil {
			if ctx.Err() != nil {
				srv.podOwnerQueue.Forget(key)
				return false
			}

			if srv.podOwnerQueue.NumRequeues(key) < podOwnerMaxRetries {
				log.Debugf("Unable to resolve workload owners for pod %s: %v", key, err)
				srv.podEventsMutex.Lock()
				_, newerEventPending := srv.podEvents[key]
				if !newerEventPending {
					srv.podEvents[key] = pending
				}
				srv.podEventsMutex.Unlock()
				if newerEventPending {
					srv.podOwnerQueue.Forget(key)
				} else {
					srv.podOwnerQueue.AddRateLimited(key)
				}
				return true
			}
			log.Warnf("Unable to resolve workload owners for pod %s after %d retries: %v", key, srv.podOwnerQueue.NumRequeues(key), err)
			// Keep the last successful owners instead of replacing them with an empty result after a transient failure.
			srv.podOwnerQueue.Forget(key)
			return true
		}
	}

	srv.applyPodEvent(pending, owners)
	srv.podOwnerQueue.Forget(key)
	return true
}

func (srv *KubeMetadataStreamServer) applyPodEvent(event workloadmeta.Event, owners []workloadmeta.KubernetesMatchedOwner) {
	pod := event.Entity.(*workloadmeta.KubernetesPod)
	if pod.NodeName == "" {
		return
	}

	key := pod.Namespace + "/" + pod.Name
	srv.metadataMutex.Lock()
	defer srv.metadataMutex.Unlock()

	switch event.Type {
	case workloadmeta.EventTypeUnset:
		if snapshot := srv.matchedOwners[pod.NodeName]; snapshot != nil {
			delete(snapshot, key)
		}
	case workloadmeta.EventTypeSet:
		if srv.matchedOwners[pod.NodeName] == nil {
			srv.matchedOwners[pod.NodeName] = make(matchedOwnersSnapshot)
		}
		srv.matchedOwners[pod.NodeName][key] = podMatchedOwnersEntry{
			namespace: pod.Namespace,
			podName:   pod.Name,
			owners:    owners,
		}
	default:
		log.Errorf("Unknown event type %d for pod %s", event.Type, key)
		return
	}

	srv.notifyNodeSubscribers(pod.NodeName)
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
	lastSentMetadataState := srv.buildMetadataSnapshot(nodeName)
	initialResp := fullStateResponse(lastSentPodServicesState, lastSentMetadataState)
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
			currentMetadataState := srv.buildMetadataSnapshot(nodeName)
			metadataDiff := computeMetadataDiff(lastSentMetadataState, currentMetadataState)
			if metadataDiff.isEmpty() {
				continue
			}
			resp := metadataDiff.response(false)
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

func (srv *KubeMetadataStreamServer) processMetadataEvents(events []workloadmeta.Event) {
	changedMetadata := false
	for _, event := range events {
		switch entity := event.Entity.(type) {
		case *workloadmeta.KubernetesMetadata:
			srv.metadataMutex.Lock()
			if srv.metadata.processNamespaceEvent(event.Type, entity) {
				changedMetadata = true
			}
			srv.metadataMutex.Unlock()
		case *workloadmeta.KubernetesKueueQueue:
			srv.metadataMutex.Lock()
			if srv.metadata.processKueueQueueEvent(event.Type, entity) {
				changedMetadata = true
			}
			srv.metadataMutex.Unlock()
		case *workloadmeta.KubernetesKueueResourceFlavor:
			srv.metadataMutex.Lock()
			if srv.metadata.processKueueResourceFlavorEvent(event.Type, entity) {
				changedMetadata = true
			}
			srv.metadataMutex.Unlock()
		case *workloadmeta.KubernetesKueueWorkload:
			srv.metadataMutex.Lock()
			if srv.metadata.processKueueWorkloadEvent(event.Type, entity) {
				changedMetadata = true
			}
			srv.metadataMutex.Unlock()
		default:
			log.Errorf("Unexpected workloadmeta entity %T in kube metadata stream", event.Entity)
		}
	}

	srv.metadataMutex.Lock()
	defer srv.metadataMutex.Unlock()
	if changedMetadata {
		srv.notifyNamespaceSubscribers()
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

func (srv *KubeMetadataStreamServer) notifyNodeSubscribers(nodeName string) {
	for _, ch := range srv.namespaceSubscribers[nodeName] {
		select {
		case ch <- struct{}{}:
		default:
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
	return srv.buildMetadataSnapshot("").namespaces
}

func (srv *KubeMetadataStreamServer) buildKueueQueuesSnapshot() map[string]kueueQueueEntry {
	return srv.buildMetadataSnapshot("").kueueQueues
}

func (srv *KubeMetadataStreamServer) buildKueueResourceFlavorsSnapshot() map[string]kueueResourceFlavorEntry {
	return srv.buildMetadataSnapshot("").kueueResourceFlavors
}

func (srv *KubeMetadataStreamServer) buildKueueWorkloadsSnapshot() map[string]kueueWorkloadEntry {
	return srv.buildMetadataSnapshot("").kueueWorkloads
}

func (srv *KubeMetadataStreamServer) buildMetadataSnapshot(nodeName string) metadataSnapshot {
	srv.metadataMutex.RLock()
	defer srv.metadataMutex.RUnlock()

	snapshot := newMetadataSnapshot()
	maps.Copy(snapshot.namespaces, srv.metadata.namespaces)
	maps.Copy(snapshot.kueueQueues, srv.metadata.kueueQueues)
	maps.Copy(snapshot.kueueResourceFlavors, srv.metadata.kueueResourceFlavors)
	maps.Copy(snapshot.kueueWorkloads, srv.metadata.kueueWorkloads)
	for key, entry := range srv.matchedOwners[nodeName] {
		entry.owners = slices.Clone(entry.owners)
		snapshot.matchedOwners[key] = entry
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
	).AddKind(workloadmeta.KindKubernetesKueueQueue).AddKind(workloadmeta.KindKubernetesKueueResourceFlavor).AddKind(workloadmeta.KindKubernetesKueueWorkload).Build()
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
	matchedOwners        []*pb.PodMatchedOwners
}

func computeMetadataDiff(old, current metadataSnapshot) metadataDiff {
	return metadataDiff{
		namespaces:           computeNamespacesDiff(old.namespaces, current.namespaces),
		kueueQueues:          computeKueueQueueDiff(old.kueueQueues, current.kueueQueues),
		kueueResourceFlavors: computeKueueResourceFlavorDiff(old.kueueResourceFlavors, current.kueueResourceFlavors),
		kueueWorkloads:       computeKueueWorkloadDiff(old.kueueWorkloads, current.kueueWorkloads),
		matchedOwners:        computeMatchedOwnersDiff(old.matchedOwners, current.matchedOwners),
	}
}

func (d metadataDiff) isEmpty() bool {
	return len(d.namespaces)+len(d.kueueQueues)+len(d.kueueResourceFlavors)+len(d.kueueWorkloads)+len(d.matchedOwners) == 0
}

func (d metadataDiff) response(isFullState bool) *pb.KubeMetadataStreamResponse {
	return &pb.KubeMetadataStreamResponse{
		IsFullState:          isFullState,
		NamespaceMetadata:    d.namespaces,
		KueueQueues:          d.kueueQueues,
		KueueResourceFlavors: d.kueueResourceFlavors,
		KueueWorkloads:       d.kueueWorkloads,
		MatchedOwners:        d.matchedOwners,
	}
}

func computeMatchedOwnersDiff(old, current matchedOwnersSnapshot) []*pb.PodMatchedOwners {
	var diff []*pb.PodMatchedOwners
	for key, entry := range current {
		previous, found := old[key]
		if found && slices.Equal(previous.owners, entry.owners) {
			continue
		}
		diff = append(diff, protoPodMatchedOwners(entry, pb.KubeMetadataEventType_SET))
	}
	for key, entry := range old {
		if _, found := current[key]; !found {
			diff = append(diff, protoPodMatchedOwners(entry, pb.KubeMetadataEventType_UNSET))
		}
	}
	return diff
}

func protoPodMatchedOwners(entry podMatchedOwnersEntry, eventType pb.KubeMetadataEventType) *pb.PodMatchedOwners {
	owners := make([]*pb.MatchedOwner, 0, len(entry.owners))
	for _, target := range entry.owners {
		owners = append(owners, &pb.MatchedOwner{
			Group:     target.Group,
			Version:   target.Version,
			Kind:      target.Kind,
			Namespace: target.Namespace,
			Name:      target.Name,
		})
	}
	return &pb.PodMatchedOwners{
		Namespace: entry.namespace,
		PodName:   entry.podName,
		Owners:    owners,
		Type:      eventType,
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
