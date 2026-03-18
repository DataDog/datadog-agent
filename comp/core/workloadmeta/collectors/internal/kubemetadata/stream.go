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
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	grpcmeta "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	pkgapiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	initialReconnectDelay = 1 * time.Second
	maxReconnectDelay     = 30 * time.Second
	streamRecvTimeout     = 10 * time.Minute
)

type namespaceMetadata struct {
	labels      map[string]string
	annotations map[string]string
}

// streamClient manages a gRPC streaming connection to the DCA for
// pod-to-service and namespace metadata. It keeps a local cache.
type streamClient struct {
	nodeName string
	cfg      configmodel.Reader

	mu          sync.RWMutex
	podServices map[string][]string          // "namespace/podName" -> services
	namespaces  map[string]namespaceMetadata // namespace name -> labels/annotations
	active      bool
}

func newStreamClient(nodeName string, cfg configmodel.Reader) *streamClient {
	return &streamClient{
		nodeName:    nodeName,
		cfg:         cfg,
		podServices: make(map[string][]string),
		namespaces:  make(map[string]namespaceMetadata),
	}
}

// run manages the streaming connection with reconnection and exponential
// backoff. It falls back permanently if the DCA returns gRPC Unimplemented.
func (sc *streamClient) run(ctx context.Context) {
	delay := initialReconnectDelay
	for {
		err := sc.streamOnce(ctx)
		if err == nil {
			return
		}

		if statusWithErr, ok := status.FromError(err); ok && statusWithErr.Code() == codes.Unimplemented {
			log.Infof("DCA does not support kube metadata streaming")
			sc.mu.Lock()
			sc.active = false
			sc.mu.Unlock()
			return
		}

		sc.mu.Lock()
		wasActive := sc.active
		sc.active = false
		sc.mu.Unlock()

		if wasActive {
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

func (sc *streamClient) getServices(namespace, podName string) ([]string, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if !sc.active {
		return nil, false
	}

	key := namespace + "/" + podName
	svcs, ok := sc.podServices[key]
	return svcs, ok
}

func (sc *streamClient) getNamespaceMetadata(namespace string) (labels, annotations map[string]string, found bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if !sc.active {
		return nil, nil, false
	}

	ns, ok := sc.namespaces[namespace]
	if !ok {
		return nil, nil, false
	}
	return ns.labels, ns.annotations, true
}

func (sc *streamClient) isActive() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.active
}

// streamOnce establishes a single gRPC streaming connection and processes
// events until the connection is lost or the context is canceled.
func (sc *streamClient) streamOnce(ctx context.Context) error {
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
// to a streaming response.
func (sc *streamClient) applyResponse(resp *pb.KubeMetadataStreamResponse) {
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

		sc.active = true
		return
	}

	if !sc.active && (len(resp.Mappings) > 0 || len(resp.NamespaceMetadata) > 0) {
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
		}
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
		}
	}
}
