// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// gnmi-fakeserver is a minimal gNMI server used by the NDM gNMI E2E suite.
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	defaultListenAddr = "0.0.0.0:57400"
	defaultUsername   = "user"
	defaultPassword   = "test-password"
	defaultInterface  = "eth0"
)

func main() {
	listenAddr := flag.String("listen", envOrDefault("GNMI_FAKESERVER_LISTEN", defaultListenAddr), "TCP address to listen on")
	username := flag.String("username", envOrDefault("GNMI_FAKESERVER_USERNAME", defaultUsername), "expected gNMI username")
	password := flag.String("password", envOrDefault("GNMI_FAKESERVER_PASSWORD", defaultPassword), "expected gNMI password")
	interfaceName := flag.String("interface", envOrDefault("GNMI_FAKESERVER_INTERFACE", defaultInterface), "interface name for synthetic counters")
	flag.Parse()

	server, err := newServer(*listenAddr, *username, *password)
	if err != nil {
		log.Fatalf("start fakeserver: %v", err)
	}
	defer server.close()

	log.Printf("gNMI fakeserver listening on %s", server.addr)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go server.publishLoop(ctx, *interfaceName)

	<-ctx.Done()
}

type streamState struct {
	id     int
	stream grpc.BidiStreamingServer[gnmipb.SubscribeRequest, gnmipb.SubscribeResponse]
}

type server struct {
	grpcServer *grpc.Server
	listener   net.Listener
	addr       string

	mu               sync.Mutex
	streams          map[int]*streamState
	nextStreamID     atomic.Int32
	subscribeEvents  chan int
	expectedUsername string
	expectedPassword string
}

func newServer(listenAddr, username, password string) (*server, error) {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	s := &server{
		listener:         listener,
		addr:             listener.Addr().String(),
		streams:          make(map[int]*streamState),
		subscribeEvents:  make(chan int, 16),
		expectedUsername: username,
		expectedPassword: password,
	}

	grpcServer := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	gnmipb.RegisterGNMIServer(grpcServer, &gnmiService{server: s})
	s.grpcServer = grpcServer

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Printf("grpc serve: %v", err)
		}
	}()

	return s, nil
}

func (s *server) close() {
	s.grpcServer.GracefulStop()
	_ = s.listener.Close()
}

func (s *server) publishLoop(ctx context.Context, interfaceName string) {
	var streamID int
	var inOctets uint64 = 42

	for {
		select {
		case <-ctx.Done():
			return
		case streamID = <-s.subscribeEvents:
			publishCounters(s, streamID, interfaceName, inOctets)
			inOctets += 10
		case <-time.After(1 * time.Second):
			if streamID == 0 {
				continue
			}
			publishCounters(s, streamID, interfaceName, inOctets)
			inOctets += 10
		}
	}
}

func publishCounters(s *server, streamID int, interfaceName string, inOctets uint64) {
	updates := []*gnmipb.Update{
		interfaceInOctetsUpdate(interfaceName, inOctets),
		interfaceOutOctetsUpdate(interfaceName, inOctets*2),
		hostnameUpdate("gnmi-router-1"),
		interfaceNameUpdate(interfaceName),
	}
	for _, update := range updates {
		if err := s.sendUpdate(streamID, update); err != nil {
			log.Printf("publish update: %v", err)
			return
		}
	}
}

func (s *server) sendUpdate(streamID int, update *gnmipb.Update) error {
	s.mu.Lock()
	stream, ok := s.streams[streamID]
	s.mu.Unlock()
	if !ok {
		return status.Error(codes.NotFound, "stream not found")
	}

	return stream.stream.Send(&gnmipb.SubscribeResponse{
		Response: &gnmipb.SubscribeResponse_Update{
			Update: &gnmipb.Notification{
				Timestamp: time.Now().UnixNano(),
				Update:    []*gnmipb.Update{update},
			},
		},
	})
}

type gnmiService struct {
	gnmipb.UnimplementedGNMIServer
	server *server
}

func (g *gnmiService) Subscribe(stream grpc.BidiStreamingServer[gnmipb.SubscribeRequest, gnmipb.SubscribeResponse]) error {
	username, password := credentialsFromContext(stream.Context())
	if username != g.server.expectedUsername || password != g.server.expectedPassword {
		return status.Error(codes.Unauthenticated, "invalid credentials")
	}

	streamID := int(g.server.nextStreamID.Add(1))
	state := &streamState{id: streamID, stream: stream}

	g.server.mu.Lock()
	g.server.streams[streamID] = state
	g.server.mu.Unlock()
	defer func() {
		g.server.mu.Lock()
		delete(g.server.streams, streamID)
		g.server.mu.Unlock()
	}()

	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}
		if req.GetSubscribe() == nil {
			continue
		}

		if err := stream.Send(&gnmipb.SubscribeResponse{
			Response: &gnmipb.SubscribeResponse_SyncResponse{SyncResponse: true},
		}); err != nil {
			return err
		}

		select {
		case g.server.subscribeEvents <- streamID:
		default:
		}
	}
}

func credentialsFromContext(ctx context.Context) (string, string) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", ""
	}

	if values := md.Get("username"); len(values) > 0 {
		username := values[0]
		password := ""
		if passwordValues := md.Get("password"); len(passwordValues) > 0 {
			password = passwordValues[0]
		}
		return username, password
	}

	for _, value := range md.Get("authorization") {
		if !strings.HasPrefix(value, "Basic ") {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "Basic "))
		if err != nil {
			continue
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			continue
		}
		return parts[0], parts[1]
	}

	return "", ""
}

func scalarUint64(value uint64) *gnmipb.TypedValue {
	return &gnmipb.TypedValue{Value: &gnmipb.TypedValue_UintVal{UintVal: value}}
}

func scalarString(value string) *gnmipb.TypedValue {
	return &gnmipb.TypedValue{Value: &gnmipb.TypedValue_StringVal{StringVal: value}}
}

func interfaceInOctetsUpdate(interfaceName string, octets uint64) *gnmipb.Update {
	return &gnmipb.Update{
		Path: &gnmipb.Path{
			Elem: []*gnmipb.PathElem{
				{Name: "interfaces"},
				{Name: "interface", Key: map[string]string{"name": interfaceName}},
				{Name: "state"},
				{Name: "counters"},
				{Name: "in-octets"},
			},
		},
		Val: scalarUint64(octets),
	}
}

func interfaceOutOctetsUpdate(interfaceName string, octets uint64) *gnmipb.Update {
	return &gnmipb.Update{
		Path: &gnmipb.Path{
			Elem: []*gnmipb.PathElem{
				{Name: "interfaces"},
				{Name: "interface", Key: map[string]string{"name": interfaceName}},
				{Name: "state"},
				{Name: "counters"},
				{Name: "out-octets"},
			},
		},
		Val: scalarUint64(octets),
	}
}

func hostnameUpdate(hostname string) *gnmipb.Update {
	return &gnmipb.Update{
		Path: &gnmipb.Path{
			Elem: []*gnmipb.PathElem{
				{Name: "system"},
				{Name: "state"},
				{Name: "hostname"},
			},
		},
		Val: scalarString(hostname),
	}
}

func interfaceNameUpdate(interfaceName string) *gnmipb.Update {
	return &gnmipb.Update{
		Path: &gnmipb.Path{
			Elem: []*gnmipb.PathElem{
				{Name: "interfaces"},
				{Name: "interface", Key: map[string]string{"name": interfaceName}},
				{Name: "state"},
				{Name: "name"},
			},
		},
		Val: scalarString(interfaceName),
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
