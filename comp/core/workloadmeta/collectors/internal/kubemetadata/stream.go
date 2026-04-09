// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package kubemetadata

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	grpcmeta "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	pkgapiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// This file implements a streaming alternative to the pull-based approach.
//
// The kubemetadata collector queries the Cluster Agent every minute in the
// pull-based path, which means tags derived from its data can have substantial
// lag. To fix this, the Cluster Agent exposes a grpc streaming endpoint that
// pushes kubernetes service mappings and namespace labels/annotations. This
// file contains the streaming provider, which connects to that endpoint and
// generates enriched pod events as updates arrive.
//
// The provider needs to know which pods to enrich. Instead of querying the
// kubelet directly (which would require periodic polling and could create
// inconsistencies with the kubelet workloadmeta collector that already fetches
// pod info from the kubelet), it subscribes to workloadmeta pod events. This
// has the nice property that a pod is guaranteed to have been discovered by the
// kubelet collector, which makes the code a bit easier to reason about.
//
// Unlike pull-based providers, this does not emit KubernetesMetadata
// workloadmeta events for namespaces. Pull-based providers emit them as a
// workaround for the polling lag, so it's not needed here.

const (
	initialReconnectDelay = 1 * time.Second
	maxReconnectDelay     = 30 * time.Second
	streamRecvTimeout     = 10 * time.Minute
)

// streamingProvider generates pod workloadmeta events enriched with the
// Kubernetes services each pod belongs to and, when configured, the namespace
// labels and annotations relevant to those pods.
type streamingProvider struct {
	dcaStream                   *dcaStreamClient
	wmeta                       workloadmeta.Component
	active                      atomic.Bool
	ignoreServiceReadiness      bool
	collectNamespaceLabels      bool
	collectNamespaceAnnotations bool
}

func newStreamingProvider(
	nodeName string,
	cfg configmodel.Reader,
	wmeta workloadmeta.Component,
	ignoreServiceReadiness, collectNamespaceLabels, collectNamespaceAnnotations bool,
) *streamingProvider {
	return &streamingProvider{
		dcaStream:                   newDCAStreamClient(nodeName, cfg),
		wmeta:                       wmeta,
		ignoreServiceReadiness:      ignoreServiceReadiness,
		collectNamespaceLabels:      collectNamespaceLabels,
		collectNamespaceAnnotations: collectNamespaceAnnotations,
	}
}

func (p *streamingProvider) start(ctx context.Context) {
	p.active.Store(true)
	log.Debug("kube metadata streaming active")
	go p.dcaStream.run(ctx)
	go p.run(ctx)
}

func (p *streamingProvider) isActive() bool {
	if p == nil {
		return false
	}
	return p.active.Load()
}

// run subscribes to workloadmeta pod events and reacts to updates received via
// the Cluster Agent
func (p *streamingProvider) run(ctx context.Context) {
	defer func() {
		p.active.Store(false)
		log.Debug("kube metadata streaming deactivated")
	}()

	select {
	case <-ctx.Done():
		return
	case <-p.dcaStream.ready():
	}

	if p.dcaStream.isUnimplemented() {
		return
	}

	// Notice that the source is SourceNodeOrchestrator. This means that this
	// only gets pods from the kubelet collector and not this one (source is
	// SourceClusterOrchestrator).
	podFilter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceNodeOrchestrator).
		AddKind(workloadmeta.KindKubernetesPod).
		Build()
	wmetaCh := p.wmeta.Subscribe(componentName, workloadmeta.NormalPriority, podFilter)
	defer p.wmeta.Unsubscribe(wmetaCh)

	seenPods := make(map[string]string) // "namespace/name" -> pod UID

	for {
		select {
		case <-ctx.Done():
			return

		case bundle, ok := <-wmetaCh:
			if !ok {
				return
			}
			p.handleWmetaPodEvents(bundle, seenPods)

		case <-p.dcaStream.updates():
			update := p.dcaStream.drainPendingUpdate()
			p.handleDCAStreamUpdate(update, seenPods)
		}

		if p.dcaStream.isUnimplemented() {
			log.Info("Streaming endpoint not exposed in Cluster Agent")
			return
		}
	}
}

func (p *streamingProvider) handleWmetaPodEvents(bundle workloadmeta.EventBundle, seenPods map[string]string) {
	// Nothing depends on this processing, so ack as soon as possible
	bundle.Acknowledge()

	var events []workloadmeta.CollectorEvent

	for _, event := range bundle.Events {
		pod, ok := event.Entity.(*workloadmeta.KubernetesPod)
		if !ok {
			log.Warn("Unexpected entity type")
			continue
		}

		switch event.Type {
		case workloadmeta.EventTypeSet:
			namespacedName := pod.Namespace + "/" + pod.Name
			seenPods[namespacedName] = pod.EntityID.ID

			// We emit a pod event even when the DCA stream cache has no
			// services for this pod. This means the pod may briefly have
			// incomplete data: the DCA only reports pods that have at least one
			// service, so we cannot distinguish "no services yet" from "no
			// services."
			events = append(events, p.buildPodEvent(pod))

		case workloadmeta.EventTypeUnset:
			for namespacedName, uid := range seenPods {
				if uid == pod.EntityID.ID {
					delete(seenPods, namespacedName)
					break
				}
			}
			events = append(events, createUnsetEvent(pod.EntityID))
		default:
			log.Warn("Unexpected event type")
		}
	}

	if len(events) > 0 {
		p.wmeta.Notify(events)
	}
}

func (p *streamingProvider) handleDCAStreamUpdate(update streamUpdate, seenPods map[string]string) {
	var events []workloadmeta.CollectorEvent

	if update.updateIsFullState {
		for _, uid := range seenPods {
			if podEvent, ok := p.buildPodEventFromUID(uid); ok {
				events = append(events, podEvent)
			}
		}
	} else {
		for namespacedName := range update.updatedPods {
			uid, ok := seenPods[namespacedName]
			if !ok {
				continue
			}
			if podEvent, ok := p.buildPodEventFromUID(uid); ok {
				events = append(events, podEvent)
			}
		}

		// Re-enrich pods in updated namespaces so they pick up the new
		// namespace labels/annotations.
		for ns := range update.updatedNamespaces {
			for namespacedName, uid := range seenPods {
				if strings.HasPrefix(namespacedName, ns+"/") {
					if podEvent, ok := p.buildPodEventFromUID(uid); ok {
						events = append(events, podEvent)
					}
				}
			}
		}
	}

	if len(events) > 0 {
		p.wmeta.Notify(events)
	}
}

func (p *streamingProvider) buildPodEvent(pod *workloadmeta.KubernetesPod) workloadmeta.CollectorEvent {
	services := []string{}
	if p.ignoreServiceReadiness || pod.Ready {
		if svcs, found := p.dcaStream.getServices(pod.Namespace, pod.Name); found {
			services = svcs
		}
	}

	nsLabels, nsAnnotations := p.getNamespaceMetadata(pod.Namespace)

	return workloadmeta.CollectorEvent{
		Source: workloadmeta.SourceClusterOrchestrator,
		Type:   workloadmeta.EventTypeSet,
		Entity: &workloadmeta.KubernetesPod{
			EntityID: pod.EntityID,
			EntityMeta: workloadmeta.EntityMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
				// Labels and annotations are omitted because they're filled by
				// the kubelet collector
			},
			KubeServices:         services,
			NamespaceLabels:      nsLabels,
			NamespaceAnnotations: nsAnnotations,
		},
	}
}

func (p *streamingProvider) buildPodEventFromUID(uid string) (workloadmeta.CollectorEvent, bool) {
	pod, err := p.wmeta.GetKubernetesPod(uid)
	if err != nil {
		return workloadmeta.CollectorEvent{}, false
	}

	return p.buildPodEvent(pod), true
}

func (p *streamingProvider) getNamespaceMetadata(ns string) (labels, annotations map[string]string) {
	nsLabels, nsAnnotations, found := p.dcaStream.getNamespaceMetadata(ns)
	if !found {
		return nil, nil
	}

	metadata := namespaceMetadata{labels: nsLabels, annotations: nsAnnotations}
	return selectNamespaceMetadata(metadata, p.collectNamespaceLabels, p.collectNamespaceAnnotations)
}

type streamUpdate struct {
	updateIsFullState bool
	updatedPods       map[string]struct{} // keys are "namespace/name"
	updatedNamespaces map[string]struct{}
}

// dcaStreamClient manages a gRPC streaming connection to the DCA for
// pod-to-service and namespace metadata. It keeps a local cache.
type dcaStreamClient struct {
	nodeName string
	cfg      configmodel.Reader

	mu            sync.RWMutex
	podServices   map[string][]string          // "namespace/podName" -> services
	namespaces    map[string]namespaceMetadata // namespace name -> labels/annotations
	initialized   bool
	unimplemented bool
	pendingUpdate streamUpdate

	updateCh chan struct{} // signals that pendingUpdate has new data

	// readyCh is closed once the stream has received its first full state or
	// is detected as unimplemented.
	readyCh   chan struct{}
	readyOnce sync.Once
}

func newDCAStreamClient(nodeName string, cfg configmodel.Reader) *dcaStreamClient {
	return &dcaStreamClient{
		nodeName:    nodeName,
		cfg:         cfg,
		podServices: make(map[string][]string),
		namespaces:  make(map[string]namespaceMetadata),
		readyCh:     make(chan struct{}),
		updateCh:    make(chan struct{}, 1),
	}
}

// run manages the streaming connection with reconnection and exponential
// backoff. It falls back permanently if the DCA returns gRPC Unimplemented.
func (sc *dcaStreamClient) run(ctx context.Context) {
	delay := initialReconnectDelay
	for {
		err := sc.streamOnce(ctx)
		if err == nil {
			return
		}

		if statusWithErr, ok := status.FromError(err); ok && statusWithErr.Code() == codes.Unimplemented {
			log.Infof("DCA does not support kube metadata streaming")
			sc.mu.Lock()
			sc.unimplemented = true
			sc.mu.Unlock()
			sc.signalReady()
			return
		}

		sc.mu.Lock()
		wasInitialized := sc.initialized
		sc.initialized = false
		sc.mu.Unlock()

		if wasInitialized {
			delay = initialReconnectDelay
		}

		log.Warnf("Kube metadata stream disconnected: %v, reconnecting in %v", err, delay)

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		delay = min(delay*2, maxReconnectDelay)
	}
}

func (sc *dcaStreamClient) getServices(namespace, podName string) ([]string, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	key := namespace + "/" + podName
	svcs, ok := sc.podServices[key]
	return svcs, ok
}

func (sc *dcaStreamClient) getNamespaceMetadata(namespace string) (labels, annotations map[string]string, found bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	ns, ok := sc.namespaces[namespace]
	if !ok {
		return nil, nil, false
	}
	return ns.labels, ns.annotations, true
}

func (sc *dcaStreamClient) isUnimplemented() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.unimplemented
}

// signalReady closes readyCh exactly once, indicating the stream has either
// received its first full state or is unimplemented.
func (sc *dcaStreamClient) signalReady() {
	sc.readyOnce.Do(func() { close(sc.readyCh) })
}

// Ready returns a channel that is closed when the stream is ready (first full
// state received) or permanently unavailable (unimplemented).
func (sc *dcaStreamClient) ready() <-chan struct{} {
	return sc.readyCh
}

// updates returns a channel that receives a signal whenever the stream cache
// has been updated. Use drainPendingUpdate to retrieve accumulated changes.
func (sc *dcaStreamClient) updates() <-chan struct{} {
	return sc.updateCh
}

// drainPendingUpdate returns and clears the accumulated update state.
func (sc *dcaStreamClient) drainPendingUpdate() streamUpdate {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	update := sc.pendingUpdate
	sc.pendingUpdate = streamUpdate{}
	return update
}

// streamOnce establishes a single gRPC streaming connection and processes
// events until the connection is lost or the context is canceled.
func (sc *dcaStreamClient) streamOnce(ctx context.Context) error {
	conn, err := dialDCA(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to DCA: %w", err)
	}
	defer conn.Close()

	client := pb.NewAgentSecureClient(conn)

	authToken, err := security.GetClusterAgentAuthToken(sc.cfg)
	if err != nil {
		return fmt.Errorf("could not get auth token: %w", err)
	}

	streamCtx := grpcmeta.NewOutgoingContext(ctx, grpcmeta.MD{
		"authorization": []string{"Bearer " + authToken},
	})

	stream, err := client.StreamKubeMetadata(streamCtx, &pb.KubeMetadataStreamRequest{
		NodeName: sc.nodeName,
	})
	if err != nil {
		return fmt.Errorf("failed to start stream: %w", err)
	}

	log.Debugf("Kube metadata stream established for node %s", sc.nodeName)

	for {
		var resp *pb.KubeMetadataStreamResponse
		err = grpcutil.DoWithTimeout(func() error {
			var recvErr error
			resp, recvErr = stream.Recv()
			return recvErr
		}, streamRecvTimeout)
		if err != nil {
			return fmt.Errorf("stream recv error: %w", err)
		}

		sc.applyResponse(resp)
	}
}

func dialDCA(ctx context.Context) (*grpc.ClientConn, error) {
	target, err := configutils.GetClusterAgentEndpoint()
	if err != nil {
		return nil, fmt.Errorf("could not get DCA endpoint: %w", err)
	}
	target = strings.TrimPrefix(target, "https://")

	var tlsConfig *tls.Config
	tlsConfig, err = pkgapiutil.GetCrossNodeClientTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("could not get TLS config: %w", err)
	}

	return grpc.DialContext( //nolint:staticcheck // TODO: use NewClient when ready
		ctx,
		target,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
}

// applyResponse updates pod-service mappings and namespace metadata according
// to a streaming response. It accumulates pods and namespaces that need to be
// updated in pendingUpdate and signals that there are updates pending.
func (sc *dcaStreamClient) applyResponse(resp *pb.KubeMetadataStreamResponse) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if resp.IsFullState {
		newPodServices := make(map[string][]string, len(resp.Mappings))
		for _, mapping := range resp.Mappings {
			key := mapping.Namespace + "/" + mapping.PodName
			newPodServices[key] = mapping.ServiceNames
		}
		sc.podServices = newPodServices

		newNamespaces := make(map[string]namespaceMetadata, len(resp.NamespaceMetadata))
		for _, ns := range resp.NamespaceMetadata {
			newNamespaces[ns.Namespace] = namespaceMetadata{
				labels:      ns.Labels,
				annotations: ns.Annotations,
			}
		}
		sc.namespaces = newNamespaces

		sc.initialized = true
		sc.pendingUpdate.updateIsFullState = true
		sc.notifyUpdate()
		sc.signalReady()
		return
	}

	if !sc.initialized && (len(resp.Mappings) > 0 || len(resp.NamespaceMetadata) > 0) {
		log.Errorf("Received incremental kube metadata update before full state, ignoring")
		return
	}

	for _, mapping := range resp.Mappings {
		key := mapping.Namespace + "/" + mapping.PodName
		switch mapping.Type {
		case pb.KubeMetadataEventType_SET:
			sc.podServices[key] = mapping.ServiceNames
		case pb.KubeMetadataEventType_UNSET:
			delete(sc.podServices, key)
		default:
			log.Errorf("Unknown event type %d for pod-service mapping %s", mapping.Type, key)
			continue
		}
		if sc.pendingUpdate.updatedPods == nil {
			sc.pendingUpdate.updatedPods = make(map[string]struct{})
		}
		sc.pendingUpdate.updatedPods[key] = struct{}{}
	}

	for _, ns := range resp.NamespaceMetadata {
		switch ns.Type {
		case pb.KubeMetadataEventType_SET:
			sc.namespaces[ns.Namespace] = namespaceMetadata{
				labels:      ns.Labels,
				annotations: ns.Annotations,
			}
		case pb.KubeMetadataEventType_UNSET:
			delete(sc.namespaces, ns.Namespace)
		default:
			log.Errorf("Unknown event type %d for namespace metadata %s", ns.Type, ns.Namespace)
			continue
		}
		if sc.pendingUpdate.updatedNamespaces == nil {
			sc.pendingUpdate.updatedNamespaces = make(map[string]struct{})
		}
		sc.pendingUpdate.updatedNamespaces[ns.Namespace] = struct{}{}
	}

	if len(resp.Mappings) > 0 || len(resp.NamespaceMetadata) > 0 {
		sc.notifyUpdate()
	}
}

// notifyUpdate sends signal on updateCh. Must be called with sc.mu held.
func (sc *dcaStreamClient) notifyUpdate() {
	select {
	case sc.updateCh <- struct{}{}:
	default: // signal already pending
	}
}
