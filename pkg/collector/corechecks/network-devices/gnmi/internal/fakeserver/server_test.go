// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package fakeserver_test

import (
	"context"
	"sync"
	"testing"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/internal/fakeserver"
)

func startServer(t *testing.T) *fakeserver.Server {
	server, err := fakeserver.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	return server
}

func dialGNMI(t *testing.T, addr string, _ string, _ string) gnmipb.GNMIClient {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})
	return gnmipb.NewGNMIClient(conn)
}

func subscribeRequest() *gnmipb.SubscribeRequest {
	return &gnmipb.SubscribeRequest{
		Request: &gnmipb.SubscribeRequest_Subscribe{
			Subscribe: &gnmipb.SubscriptionList{
				Mode:     gnmipb.SubscriptionList_STREAM,
				Encoding: gnmipb.Encoding_PROTO,
				Subscription: []*gnmipb.Subscription{
					{
						Path: &gnmipb.Path{
							Elem: []*gnmipb.PathElem{
								{Name: "interfaces"},
								{Name: "interface", Key: map[string]string{"name": "*"}},
								{Name: "state"},
								{Name: "counters"},
								{Name: "in-octets"},
							},
						},
						Mode: gnmipb.SubscriptionMode_SAMPLE,
					},
				},
			},
		},
	}
}

func openSubscribeStream(t *testing.T, client gnmipb.GNMIClient, username, password string) grpc.BidiStreamingClient[gnmipb.SubscribeRequest, gnmipb.SubscribeResponse] {
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"username", username,
		"password", password,
	))
	stream, err := client.Subscribe(ctx)
	require.NoError(t, err)
	return stream
}

func waitForActiveStreams(t *testing.T, server *fakeserver.Server, count int) {
	require.Eventually(t, func() bool {
		return server.ActiveStreamCount() == count
	}, 2*time.Second, 10*time.Millisecond)
}

func TestSubscribeSyncAndUpdate(t *testing.T) {
	server := startServer(t)
	server.SetExpectedCredentials("user", "pass")

	client := dialGNMI(t, server.Addr(), "user", "pass")
	stream := openSubscribeStream(t, client, "user", "pass")

	require.NoError(t, stream.Send(subscribeRequest()))

	event := waitSubscribeEvent(t, server)
	require.Equal(t, "user", event.Username)
	require.Equal(t, "pass", event.Password)
	require.NotNil(t, event.Request.GetSubscribe())
	require.Equal(t, gnmipb.Encoding_PROTO, event.Request.GetSubscribe().GetEncoding())

	syncResp, err := stream.Recv()
	require.NoError(t, err)
	require.True(t, syncResp.GetSyncResponse())

	update := fakeserver.InterfaceInOctetsUpdate("eth0", 42)
	require.NoError(t, server.SendUpdate(event.StreamID, update))

	resp, err := stream.Recv()
	require.NoError(t, err)
	notification := resp.GetUpdate()
	require.NotNil(t, notification)
	require.Len(t, notification.GetUpdate(), 1)
	require.Equal(t, uint64(42), notification.GetUpdate()[0].GetVal().GetUintVal())
}

func TestRejectCredentials(t *testing.T) {
	server := startServer(t)
	server.SetExpectedCredentials("user", "pass")
	server.SetRejectCredentials(true)

	client := dialGNMI(t, server.Addr(), "user", "pass")
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"username", "user",
		"password", "pass",
	))
	stream, err := client.Subscribe(ctx)
	if err != nil {
		require.Equal(t, codes.Unauthenticated, status.Code(err))
		return
	}

	_, err = stream.Recv()
	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestDelaySync(t *testing.T) {
	server := startServer(t)
	server.SetDelaySync(true)

	client := dialGNMI(t, server.Addr(), "user", "pass")
	stream := openSubscribeStream(t, client, "user", "pass")
	require.NoError(t, stream.Send(subscribeRequest()))

	event := waitSubscribeEvent(t, server)

	// Recv is called from a single goroutine so that, once the receive is
	// pending, no other goroutine touches the stream concurrently: gRPC does
	// not allow concurrent RecvMsg calls on the same stream.
	pending := recvAsync(stream)
	select {
	case <-pending:
		t.Fatal("expected sync response to be delayed")
	case <-time.After(100 * time.Millisecond):
	}

	require.NoError(t, server.SendSyncResponse(event.StreamID))

	select {
	case res := <-pending:
		require.NoError(t, res.err)
		require.True(t, res.resp.GetSyncResponse())
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sync response")
	}
}

func TestStopPublishing(t *testing.T) {
	server := startServer(t)

	client := dialGNMI(t, server.Addr(), "user", "pass")
	stream := openSubscribeStream(t, client, "user", "pass")
	require.NoError(t, stream.Send(subscribeRequest()))

	event := waitSubscribeEvent(t, server)
	_, err := stream.Recv()
	require.NoError(t, err)

	require.NoError(t, server.StopPublishing(event.StreamID))
	err = server.SendUpdate(event.StreamID, fakeserver.InterfaceInOctetsUpdate("eth0", 1))
	require.Error(t, err)
}

func TestSendReplaceAndDelete(t *testing.T) {
	server := startServer(t)

	client := dialGNMI(t, server.Addr(), "user", "pass")
	stream := openSubscribeStream(t, client, "user", "pass")
	require.NoError(t, stream.Send(subscribeRequest()))

	event := waitSubscribeEvent(t, server)
	_, err := stream.Recv()
	require.NoError(t, err)

	replace := &gnmipb.Notification{
		Timestamp: time.Now().UnixNano(),
		Update: []*gnmipb.Update{
			fakeserver.InterfaceInOctetsUpdate("eth1", 100),
			fakeserver.InterfaceOutOctetsUpdate("eth1", 200),
		},
	}
	require.NoError(t, server.SendReplace(event.StreamID, replace))

	resp, err := stream.Recv()
	require.NoError(t, err)
	require.Len(t, resp.GetUpdate().GetUpdate(), 2)

	deletePath := &gnmipb.Path{
		Elem: []*gnmipb.PathElem{
			{Name: "interfaces"},
			{Name: "interface", Key: map[string]string{"name": "eth1"}},
		},
	}
	require.NoError(t, server.SendDelete(event.StreamID, deletePath))

	resp, err = stream.Recv()
	require.NoError(t, err)
	require.Len(t, resp.GetUpdate().GetDelete(), 1)
}

func TestCloseStreamAndCounts(t *testing.T) {
	server := startServer(t)

	client := dialGNMI(t, server.Addr(), "user", "pass")
	stream := openSubscribeStream(t, client, "user", "pass")
	waitForActiveStreams(t, server, 1)
	require.GreaterOrEqual(t, server.ConnectionCount(), 1)

	require.NoError(t, stream.Send(subscribeRequest()))
	event := waitSubscribeEvent(t, server)
	require.Equal(t, 1, server.SubscriptionCount())

	_, err := stream.Recv()
	require.NoError(t, err)

	require.NoError(t, server.CloseStream(event.StreamID))

	_, err = stream.Recv()
	require.Error(t, err)

	deadline := time.Now().Add(2 * time.Second)
	for server.ActiveStreamCount() > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	require.Equal(t, 0, server.ActiveStreamCount())
}

func TestConcurrentCloseStopsActiveStream(t *testing.T) {
	server := startServer(t)

	client := dialGNMI(t, server.Addr(), "user", "pass")
	stream := openSubscribeStream(t, client, "user", "pass")
	require.NoError(t, stream.Send(subscribeRequest()))
	waitSubscribeEvent(t, server)
	_, err := stream.Recv()
	require.NoError(t, err)

	const closeCount = 8
	closeErrors := make(chan error, closeCount)
	var wg sync.WaitGroup
	for range closeCount {
		wg.Go(func() {
			closeErrors <- server.Close()
		})
	}

	closed := make(chan struct{})
	go func() {
		wg.Wait()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out closing server with an active stream")
	}
	close(closeErrors)
	for err := range closeErrors {
		require.NoError(t, err)
	}

	_, err = stream.Recv()
	require.Error(t, err)
	require.Eventually(t, func() bool {
		return server.ActiveStreamCount() == 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestConcurrentSendUpdate(t *testing.T) {
	server := startServer(t)

	client := dialGNMI(t, server.Addr(), "user", "pass")
	stream := openSubscribeStream(t, client, "user", "pass")
	require.NoError(t, stream.Send(subscribeRequest()))
	event := waitSubscribeEvent(t, server)

	const updateCount = 16
	sendErrors := make(chan error, updateCount)
	var wg sync.WaitGroup
	for value := range updateCount {
		wg.Go(func() {
			sendErrors <- server.SendUpdate(event.StreamID, fakeserver.InterfaceInOctetsUpdate("eth0", uint64(value)))
		})
	}

	syncResp, err := stream.Recv()
	require.NoError(t, err)
	require.True(t, syncResp.GetSyncResponse())

	values := make(map[uint64]struct{}, updateCount)
	for range updateCount {
		resp, err := stream.Recv()
		require.NoError(t, err)
		require.Len(t, resp.GetUpdate().GetUpdate(), 1)
		values[resp.GetUpdate().GetUpdate()[0].GetVal().GetUintVal()] = struct{}{}
	}
	require.Len(t, values, updateCount)

	wg.Wait()
	close(sendErrors)
	for err := range sendErrors {
		require.NoError(t, err)
	}
}

func TestValueHelpers(t *testing.T) {
	require.Equal(t, uint64(7), fakeserver.ScalarUint64(7).GetUintVal())
	require.Equal(t, int64(-3), fakeserver.ScalarInt64(-3).GetIntVal())
	require.Equal(t, "open", fakeserver.ScalarString("open").GetStringVal())

	update := fakeserver.InterfaceInOctetsUpdate("ge-0/0/0", 999)
	require.Equal(t, "ge-0/0/0", update.GetPath().GetElem()[1].GetKey()["name"])
	require.Equal(t, uint64(999), update.GetVal().GetUintVal())
}

func waitSubscribeEvent(t *testing.T, server *fakeserver.Server) fakeserver.SubscribeEvent {
	select {
	case event := <-server.SubscribeRequests():
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for subscribe event")
		return fakeserver.SubscribeEvent{}
	}
}

type recvResult struct {
	resp *gnmipb.SubscribeResponse
	err  error
}

func recvAsync(stream grpc.BidiStreamingClient[gnmipb.SubscribeRequest, gnmipb.SubscribeResponse]) <-chan recvResult {
	ch := make(chan recvResult, 1)
	go func() {
		resp, err := stream.Recv()
		ch <- recvResult{resp: resp, err: err}
	}()
	return ch
}
