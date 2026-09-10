// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fakeserver provides a controllable in-process gNMI server for tests.
package fakeserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

// SubscribeEvent captures a Subscribe RPC request and the credentials presented on the stream.
type SubscribeEvent struct {
	StreamID int
	Request  *gnmipb.SubscribeRequest
	Username string
	Password string
}

type streamState struct {
	stream         grpc.BidiStreamingServer[gnmipb.SubscribeRequest, gnmipb.SubscribeResponse]
	delaySync      bool
	sendMu         sync.Mutex
	stopPublishing bool
	subscribeOnce  sync.Once
	subscribeCh    chan struct{}
	closeOnce      sync.Once
	closeCh        chan struct{}
}

func newStreamState(stream grpc.BidiStreamingServer[gnmipb.SubscribeRequest, gnmipb.SubscribeResponse], delaySync bool) *streamState {
	return &streamState{
		stream:      stream,
		delaySync:   delaySync,
		subscribeCh: make(chan struct{}),
		closeCh:     make(chan struct{}),
	}
}

func (s *streamState) close() {
	s.closeOnce.Do(func() {
		close(s.closeCh)
	})
}

func (s *streamState) signalSubscribe() {
	s.subscribeOnce.Do(func() {
		close(s.subscribeCh)
	})
}

type connStatsHandler struct {
	activeConns atomic.Int32
}

func (h *connStatsHandler) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	return ctx
}

func (h *connStatsHandler) HandleRPC(_ context.Context, _ stats.RPCStats) {}

func (h *connStatsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

func (h *connStatsHandler) HandleConn(_ context.Context, s stats.ConnStats) {
	switch s.(type) {
	case *stats.ConnBegin:
		h.activeConns.Add(1)
	case *stats.ConnEnd:
		h.activeConns.Add(-1)
	}
}

// Server is a loopback gNMI server with deterministic controls for Subscribe testing.
type Server struct {
	grpcServer *grpc.Server
	addr       string

	mu           sync.RWMutex
	streams      map[int]*streamState
	nextStreamID atomic.Int32

	subscribeRequests chan SubscribeEvent

	expectedUsername  string
	expectedPassword  string
	rejectCredentials bool
	delaySyncDefault  bool

	subscriptionCount atomic.Int32
	activeStreams     atomic.Int32
	connStats         connStatsHandler

	closeOnce sync.Once
}

// New starts a gNMI server on a random loopback TCP port.
func New() (*Server, error) {
	return NewOn("127.0.0.1:0")
}

// NewOn starts a gNMI server bound to the given TCP address.
func NewOn(listenAddr string) (*Server, error) {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	s := &Server{
		addr:              listener.Addr().String(),
		streams:           make(map[int]*streamState),
		subscribeRequests: make(chan SubscribeEvent, 16),
	}

	grpcServer := grpc.NewServer(
		grpc.Creds(insecure.NewCredentials()),
		grpc.StatsHandler(&s.connStats),
	)
	gnmipb.RegisterGNMIServer(grpcServer, &gnmiService{server: s})
	s.grpcServer = grpcServer

	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			panic(fmt.Sprintf("fakeserver: serve failed: %v", err))
		}
	}()

	return s, nil
}

// Addr returns the loopback address (host:port) of the running server.
func (s *Server) Addr() string {
	return s.addr
}

// SubscribeRequests returns a channel of Subscribe RPC events for inspection in tests.
func (s *Server) SubscribeRequests() <-chan SubscribeEvent {
	return s.subscribeRequests
}

// ConnectionCount returns the number of active gRPC connections.
func (s *Server) ConnectionCount() int {
	return int(s.connStats.activeConns.Load())
}

// SubscriptionCount returns the number of Subscribe messages received.
func (s *Server) SubscriptionCount() int {
	return int(s.subscriptionCount.Load())
}

// ActiveStreamCount returns the number of open Subscribe streams.
func (s *Server) ActiveStreamCount() int {
	return int(s.activeStreams.Load())
}

// SetExpectedCredentials configures credentials that must match unless rejection is enabled.
func (s *Server) SetExpectedCredentials(username, password string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expectedUsername = username
	s.expectedPassword = password
}

// SetRejectCredentials configures whether all credentials are rejected regardless of value.
func (s *Server) SetRejectCredentials(reject bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejectCredentials = reject
}

// SetDelaySync configures whether new streams wait for SendSyncResponse before auto-syncing.
func (s *Server) SetDelaySync(delay bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delaySyncDefault = delay
}

// StopPublishing prevents further updates from being sent on the stream.
func (s *Server) StopPublishing(streamID int) error {
	s.mu.RLock()
	stream, ok := s.streams[streamID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("stream %d not found", streamID)
	}

	stream.sendMu.Lock()
	defer stream.sendMu.Unlock()
	stream.stopPublishing = true
	return nil
}

// SendSyncResponse sends a sync_response on the given stream.
func (s *Server) SendSyncResponse(streamID int) error {
	return s.sendResponse(streamID, &gnmipb.SubscribeResponse{
		Response: &gnmipb.SubscribeResponse_SyncResponse{
			SyncResponse: true,
		},
	})
}

// SendUpdate sends a single-path update notification on the stream.
func (s *Server) SendUpdate(streamID int, update *gnmipb.Update) error {
	return s.sendNotification(streamID, &gnmipb.Notification{
		Timestamp: time.Now().UnixNano(),
		Update:    []*gnmipb.Update{update},
	})
}

// SendReplace sends a full notification (replace semantics) on the stream.
func (s *Server) SendReplace(streamID int, notification *gnmipb.Notification) error {
	return s.sendNotification(streamID, notification)
}

// SendDelete sends a delete notification on the stream.
func (s *Server) SendDelete(streamID int, path *gnmipb.Path) error {
	return s.sendNotification(streamID, &gnmipb.Notification{
		Timestamp: time.Now().UnixNano(),
		Delete:    []*gnmipb.Path{path},
	})
}

// CloseStream closes the server side of a Subscribe stream.
func (s *Server) CloseStream(streamID int) error {
	s.mu.RLock()
	stream, ok := s.streams[streamID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("stream %d not found", streamID)
	}
	stream.close()
	return nil
}

// CloseConnections stops the gRPC server and closes all connections.
func (s *Server) CloseConnections() {
	s.Close()
}

// Close stops the server and releases resources.
func (s *Server) Close() error {
	s.closeOnce.Do(s.grpcServer.Stop)
	return nil
}

// AwaitSubscribe waits until the stream receives its first Subscribe message or the context is done.
func (s *Server) AwaitSubscribe(ctx context.Context, streamID int) error {
	s.mu.RLock()
	stream, ok := s.streams[streamID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("stream %d not found", streamID)
	}

	select {
	case <-stream.subscribeCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) registerStream(stream grpc.BidiStreamingServer[gnmipb.SubscribeRequest, gnmipb.SubscribeResponse], delaySync bool) int {
	id := int(s.nextStreamID.Add(1))
	state := newStreamState(stream, delaySync)

	s.mu.Lock()
	s.streams[id] = state
	s.mu.Unlock()

	s.activeStreams.Add(1)
	return id
}

func (s *Server) unregisterStream(id int) {
	s.mu.Lock()
	delete(s.streams, id)
	s.mu.Unlock()
	s.activeStreams.Add(-1)
}

func (s *Server) credentialsAllowed(username, password string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.rejectCredentials {
		return false
	}
	if s.expectedUsername == "" && s.expectedPassword == "" {
		return true
	}
	return username == s.expectedUsername && password == s.expectedPassword
}

func (s *Server) sendResponse(streamID int, response *gnmipb.SubscribeResponse) error {
	s.mu.RLock()
	stream, ok := s.streams[streamID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("stream %d not found", streamID)
	}

	stream.sendMu.Lock()
	defer stream.sendMu.Unlock()
	if stream.stopPublishing {
		return fmt.Errorf("stream %d is not publishing", streamID)
	}
	return stream.stream.Send(response)
}

func (s *Server) sendNotification(streamID int, notification *gnmipb.Notification) error {
	return s.sendResponse(streamID, &gnmipb.SubscribeResponse{
		Response: &gnmipb.SubscribeResponse_Update{
			Update: notification,
		},
	})
}

type gnmiService struct {
	gnmipb.UnimplementedGNMIServer
	server *Server
}

func (g *gnmiService) Subscribe(stream grpc.BidiStreamingServer[gnmipb.SubscribeRequest, gnmipb.SubscribeResponse]) error {
	username, password := credentialsFromContext(stream.Context())
	if !g.server.credentialsAllowed(username, password) {
		return status.Error(codes.Unauthenticated, "invalid credentials")
	}

	g.server.mu.RLock()
	delaySync := g.server.delaySyncDefault
	g.server.mu.RUnlock()

	streamID := g.server.registerStream(stream, delaySync)
	defer g.server.unregisterStream(streamID)

	g.server.mu.RLock()
	state := g.server.streams[streamID]
	g.server.mu.RUnlock()
	if state == nil {
		return status.Error(codes.Internal, "stream state missing")
	}

	reqCh := make(chan *gnmipb.SubscribeRequest)
	errCh := make(chan error, 1)
	go func() {
		for {
			req, err := stream.Recv()
			if err != nil {
				select {
				case errCh <- err:
				case <-stream.Context().Done():
				}
				return
			}
			select {
			case reqCh <- req:
			case <-stream.Context().Done():
				return
			}
		}
	}()

	for {
		select {
		case <-state.closeCh:
			return status.Error(codes.Canceled, "stream closed by server")
		case <-stream.Context().Done():
			return stream.Context().Err()
		case err := <-errCh:
			return err
		case req := <-reqCh:
			if err := g.handleSubscribeRequest(streamID, state, username, password, req); err != nil {
				return err
			}
		}
	}
}

func (g *gnmiService) handleSubscribeRequest(streamID int, state *streamState, username, password string, req *gnmipb.SubscribeRequest) error {
	switch req.GetRequest().(type) {
	case *gnmipb.SubscribeRequest_Subscribe:
		g.server.subscriptionCount.Add(1)
		state.signalSubscribe()

		event := SubscribeEvent{
			StreamID: streamID,
			Request:  req,
			Username: username,
			Password: password,
		}
		if !state.delaySync {
			if err := g.server.SendSyncResponse(streamID); err != nil {
				return err
			}
		}

		select {
		case g.server.subscribeRequests <- event:
		default:
		}
	case *gnmipb.SubscribeRequest_Poll:
		// Poll requests are accepted; tests drive responses explicitly.
	default:
		return status.Error(codes.InvalidArgument, "unsupported subscribe request type")
	}
	return nil
}
