// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogtelextension

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	grpcauth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

const (
	taggerStreamSendTimeout = 1 * time.Minute
	streamKeepAliveInterval = 9 * time.Minute
)

// token represents a throttler token
type token string

// throttler provides tokens with throttling logic that limits the number of active tokens at the same time
type throttler interface {
	RequestToken() token
	Release(t token)
}

// limiter implements the throttler interface
type limiter struct {
	mutex          sync.RWMutex
	tokensChan     chan struct{}
	activeRequests map[token]struct{}
}

// newSyncThrottler creates and returns a new throttler
func newSyncThrottler(maxConcurrentSync uint32) throttler {
	return &limiter{
		mutex:          sync.RWMutex{},
		tokensChan:     make(chan struct{}, maxConcurrentSync),
		activeRequests: make(map[token]struct{}),
	}
}

// RequestToken implements throttler#RequestToken
func (l *limiter) RequestToken() token {
	tk := token(uuid.New().String())
	l.tokensChan <- struct{}{}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.activeRequests[tk] = struct{}{}
	return tk
}

// Release implements throttler#Release
func (l *limiter) Release(t token) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if _, found := l.activeRequests[t]; found {
		<-l.tokensChan
		delete(l.activeRequests, t)
	}
}

// telemetryStore holds telemetry counters for the tagger server
type telemetryStore struct {
	ServerStreamErrors telemetry.Counter
}

// newTelemetryStore creates a new telemetry store
func newTelemetryStore(telemetryComp telemetry.Component) *telemetryStore {
	return &telemetryStore{
		ServerStreamErrors: telemetryComp.NewCounterWithOpts(
			"tagger",
			"server_stream_errors",
			[]string{},
			"Errors when streaming out tagger events",
			telemetry.Options{NoDoubleUnderscoreSep: true},
		),
	}
}

// computeTagsEventInBytes returns the size of a tags stream event in bytes
func computeTagsEventInBytes(event *pb.StreamTagsEvent) int {
	return proto.Size(event)
}

// processChunksInPlace splits the passed slice into contiguous chunks such that the total size of each chunk is at most maxChunkSize
// and applies the consume function to each of these chunks
func processChunksInPlace(slice []*pb.StreamTagsEvent, maxChunkSize int, computeSize func(*pb.StreamTagsEvent) int, consume func([]*pb.StreamTagsEvent) error) error {
	idx := 0
	for idx < len(slice) {
		chunkSize := computeSize(slice[idx])
		j := idx + 1

		for j < len(slice) {
			eventSize := computeSize(slice[j])
			if chunkSize+eventSize > maxChunkSize {
				break
			}
			chunkSize += eventSize
			j++
		}

		if err := consume(slice[idx:j]); err != nil {
			return err
		}
		idx = j
	}
	return nil
}

// taggerServerWrapper implements pb.AgentSecureServer for tagger gRPC methods
type taggerServerWrapper struct {
	pb.UnimplementedAgentSecureServer
	ext *dogtelExtension
}

// TaggerStreamEntities subscribes to added, removed, or changed entities in the Tagger
// and streams them to clients as pb.StreamTagsResponse events.
func (w *taggerServerWrapper) TaggerStreamEntities(in *pb.StreamTagsRequest, out pb.AgentSecure_TaggerStreamEntitiesServer) error {
	cardinality, err := pb2TaggerCardinality(in.GetCardinality())
	if err != nil {
		return err
	}

	ticker := time.NewTicker(streamKeepAliveInterval)
	defer ticker.Stop()

	timeoutRefreshError := make(chan error)

	go func() {
		// The remote tagger client has a timeout that closes the
		// connection after 10 minutes of inactivity. In order to avoid closing the
		// connection and having to open it again, the server will send
		// an empty message after 9 minutes of inactivity.
		for {
			select {
			case <-out.Context().Done():
				return

			case <-ticker.C:
				err = grpcutil.DoWithTimeout(func() error {
					return out.Send(&pb.StreamTagsResponse{
						Events: []*pb.StreamTagsEvent{},
					})
				}, taggerStreamSendTimeout)

				if err != nil {
					w.ext.log.Warnf("error sending tagger keep-alive: %s", err)
					w.ext.taggerTelemetry.ServerStreamErrors.Inc()
					timeoutRefreshError <- err
					return
				}
			}
		}
	}()

	filterBuilder := types.NewFilterBuilder()
	for _, prefix := range in.GetPrefixes() {
		filterBuilder = filterBuilder.Include(types.EntityIDPrefix(prefix))
	}

	filter := filterBuilder.Build(cardinality)

	streamingID := in.GetStreamingID()
	if streamingID == "" {
		streamingID = uuid.New().String()
	}
	subscriptionID := "streaming-client-" + streamingID

	// initBurst is a flag indicating if the initial sync is still in progress or not
	initBurst := true
	w.ext.log.Debugf("requesting token from server throttler for streaming id: %q", streamingID)
	tk := w.ext.taggerThrottler.RequestToken()
	defer w.ext.taggerThrottler.Release(tk)

	subscription, err := w.ext.tagger.Subscribe(subscriptionID, filter)
	w.ext.log.Debugf("tagger server has just initiated subscription for %q at time %v", subscriptionID, time.Now().Unix())
	if err != nil {
		w.ext.log.Errorf("Failed to subscribe to tagger for subscription %q", subscriptionID)
		return err
	}

	defer subscription.Unsubscribe()

	sendFunc := func(chunk []*pb.StreamTagsEvent) error {
		return grpcutil.DoWithTimeout(func() error {
			return out.Send(&pb.StreamTagsResponse{
				Events: chunk,
			})
		}, taggerStreamSendTimeout)
	}

	maxEventSize := w.ext.config.TaggerMaxMessageSize / 2

	for {
		select {
		case events, ok := <-subscription.EventsChan():
			if !ok {
				w.ext.log.Warnf("subscriber channel closed, client will reconnect")
				return errors.New("subscriber channel closed")
			}

			ticker.Reset(streamKeepAliveInterval)

			responseEvents := make([]*pb.StreamTagsEvent, 0, len(events))
			for _, event := range events {
				e, err := tagger2PbEntityEvent(event)
				if err != nil {
					w.ext.log.Warnf("can't convert tagger entity to protobuf: %s", err)
					continue
				}

				responseEvents = append(responseEvents, e)
			}

			if err := processChunksInPlace(responseEvents, maxEventSize, computeTagsEventInBytes, sendFunc); err != nil {
				w.ext.log.Warnf("error sending tagger event: %s", err)
				w.ext.taggerTelemetry.ServerStreamErrors.Inc()
				return err
			}

			if initBurst {
				initBurst = false
				w.ext.taggerThrottler.Release(tk)
				w.ext.log.Infof("tagger server has just finished initialization for subscription %q at time %v", subscriptionID, time.Now().Unix())
			}

		case <-out.Context().Done():
			return nil

		case err = <-timeoutRefreshError:
			return err
		}
	}
}

// TaggerFetchEntity fetches an entity from the Tagger with the desired cardinality tags.
func (w *taggerServerWrapper) TaggerFetchEntity(_ context.Context, in *pb.FetchEntityRequest) (*pb.FetchEntityResponse, error) {
	if in.Id == nil {
		return nil, status.Errorf(codes.InvalidArgument, `missing "id" parameter`)
	}

	entityID := types.NewEntityID(types.EntityIDPrefix(in.Id.Prefix), in.Id.Uid)
	cardinality, err := pb2TaggerCardinality(in.GetCardinality())
	if err != nil {
		return nil, err
	}

	tags, err := w.ext.tagger.Tag(entityID, cardinality)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err)
	}

	return &pb.FetchEntityResponse{
		Id:          in.Id,
		Cardinality: in.GetCardinality(),
		Tags:        tags,
	}, nil
}

// TaggerGenerateContainerIDFromOriginInfo requests the Tagger to generate a container ID from the given OriginInfo.
func (w *taggerServerWrapper) TaggerGenerateContainerIDFromOriginInfo(_ context.Context, in *pb.GenerateContainerIDFromOriginInfoRequest) (*pb.GenerateContainerIDFromOriginInfoResponse, error) {
	generatedContainerID, err := w.ext.tagger.GenerateContainerIDFromOriginInfo(origindetection.OriginInfo{
		LocalData: origindetection.LocalData{
			ProcessID:   *in.LocalData.ProcessID,
			ContainerID: *in.LocalData.ContainerID,
			Inode:       *in.LocalData.Inode,
			PodUID:      *in.LocalData.PodUID,
		},
		ExternalData: origindetection.ExternalData{
			Init:          *in.ExternalData.Init,
			ContainerName: *in.ExternalData.ContainerName,
			PodUID:        *in.ExternalData.PodUID,
		},
	})
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err)
	}

	return &pb.GenerateContainerIDFromOriginInfoResponse{
		ContainerID: generatedContainerID,
	}, nil
}

// startTaggerServer starts the minimal tagger gRPC server
func (e *dogtelExtension) startTaggerServer() error {
	// 1. Create listener
	addr := fmt.Sprintf("%s:%d", e.config.TaggerServerAddr, e.config.TaggerServerPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to create listener on %s: %w", addr, err)
	}
	e.taggerListener = lis

	// Store the actual port (useful when auto-assigning with port 0)
	e.taggerServerPort = lis.Addr().(*net.TCPAddr).Port

	// 2. Initialize telemetry and throttler
	e.taggerTelemetry = newTelemetryStore(e.telemetry)
	e.taggerThrottler = newSyncThrottler(uint32(e.config.TaggerMaxConcurrentSync))

	// 3. Setup gRPC server with authentication
	var grpcOpts []grpc.ServerOption

	// Get TLS credentials from IPC component
	tlsConf := e.ipc.GetTLSServerConfig()
	if tlsConf != nil {
		creds := credentials.NewTLS(tlsConf)
		grpcOpts = append(grpcOpts, grpc.Creds(creds))
		e.log.Debug("Tagger server: TLS enabled")
	} else {
		e.log.Warn("Tagger server: TLS not configured, running without TLS")
	}

	// Add auth interceptor from IPC
	authInterceptor := grpcauth.UnaryServerInterceptor(grpcutil.StaticAuthInterceptor(e.ipc.GetAuthToken()))
	grpcOpts = append(grpcOpts, grpc.UnaryInterceptor(authInterceptor))

	// Add stream auth interceptor
	streamAuthInterceptor := grpcauth.StreamServerInterceptor(grpcutil.StaticAuthInterceptor(e.ipc.GetAuthToken()))
	grpcOpts = append(grpcOpts, grpc.StreamInterceptor(streamAuthInterceptor))

	// Set max message size
	grpcOpts = append(grpcOpts,
		grpc.MaxRecvMsgSize(e.config.TaggerMaxMessageSize),
		grpc.MaxSendMsgSize(e.config.TaggerMaxMessageSize),
	)

	// 4. Create gRPC server
	e.taggerServer = grpc.NewServer(grpcOpts...)

	// 5. Register tagger service
	pb.RegisterAgentSecureServer(e.taggerServer, &taggerServerWrapper{
		ext: e,
	})

	// 6. Start serving in goroutine
	go func() {
		e.log.Infof("Starting tagger gRPC server on %s (port %d)", addr, e.taggerServerPort)
		if err := e.taggerServer.Serve(lis); err != nil {
			e.log.Errorf("Tagger server error: %v", err)
		}
	}()

	return nil
}

// stopTaggerServer stops the tagger gRPC server gracefully
func (e *dogtelExtension) stopTaggerServer() {
	if e.taggerServer != nil {
		e.log.Info("Stopping tagger gRPC server")
		e.taggerServer.GracefulStop()
		e.taggerServer = nil
	}
	if e.taggerListener != nil {
		e.taggerListener.Close()
		e.taggerListener = nil
	}
}
