// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/benbjohnson/clock"
	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/internal/fakeserver"
)

const rejectedTestPath = "/openconfig/system/state/missing"

func rejectionTestProfile() config.ProfileDefinition {
	return config.ProfileDefinition{
		Metrics: []config.MetricConfig{
			{Path: "/openconfig/interfaces/interface/state/counters/in-octets", Tags: map[string]string{"interface": "name"}},
			{Path: rejectedTestPath},
		},
		Metadata: config.MetadataConfig{Device: config.DeviceMetadataConfig{Hostname: "/openconfig/system/state/hostname"}},
	}
}

func requestHasMissing(request *gnmipb.SubscribeRequest) bool {
	for _, sub := range request.GetSubscribe().GetSubscription() {
		for _, elem := range sub.GetPath().GetElem() {
			if elem.Name == "missing" {
				return true
			}
		}
	}
	return false
}

func startRejectionTestClient(t *testing.T, server *fakeserver.Server, opts ...Option) *Client {
	t.Helper()
	host, portText, err := net.SplitHostPort(server.Addr())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	opts = append([]Option{WithReconnectDelays(time.Hour, time.Hour)}, opts...)
	c, err := New(Config{Address: host, Port: port, Username: "test", Password: "test", Profile: rejectionTestProfile()}, opts...)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, c.Close()) })
	require.NoError(t, c.Start(context.Background()))
	return c
}

func nextRejectionEvent(t *testing.T, server *fakeserver.Server) fakeserver.SubscribeEvent {
	t.Helper()
	select {
	case event := <-server.SubscribeRequests():
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("missing subscription request")
		return fakeserver.SubscribeEvent{}
	}
}

func TestRejectedPathDoesNotBlockHealthyPaths(t *testing.T) {
	for _, code := range []codes.Code{codes.InvalidArgument, codes.NotFound, codes.Unimplemented} {
		for _, inBand := range []bool{false, true} {
			t.Run(code.String()+"/inband="+strconv.FormatBool(inBand), func(t *testing.T) {
				server, err := fakeserver.New()
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, server.Close()) })
				server.SetSubscribeErrorHandler(func(request *gnmipb.SubscribeRequest) error {
					if requestHasMissing(request) {
						return status.Error(code, "schema node missing")
					}
					return nil
				}, inBand)
				c := startRejectionTestClient(t, server)
				var live fakeserver.SubscribeEvent
				for {
					event := nextRejectionEvent(t, server)
					if len(event.Request.GetSubscribe().GetSubscription()) == 2 && !requestHasMissing(event.Request) {
						live = event
						break
					}
				}
				require.NoError(t, server.SendUpdate(live.StreamID, fakeserver.InterfaceInOctetsUpdate("eth0", 42)))
				// These assertions cross a real loopback socket, not a virtual-time timer.
				require.Eventually(t, func() bool {
					entry, ok := c.Get("/openconfig/interfaces/interface/state/counters/in-octets", map[string]string{"name": "eth0"})
					return ok && entry.Value == uint64(42) && c.Synchronized() && c.StreamState() == StreamStateConnected
				}, 5*time.Second, 5*time.Millisecond)
				errors := c.ConnectionStatus().RejectedSubscriptions
				require.Len(t, errors, 1)
				require.Contains(t, errors[rejectedTestPath], code.String())
				errors[rejectedTestPath] = "mutated"
				require.Contains(t, c.ConnectionStatus().RejectedSubscriptions[rejectedTestPath], "schema node missing")
				require.NoError(t, c.Close())
				require.Eventually(t, func() bool { return server.ActiveStreamCount() == 0 && server.ConnectionCount() == 0 }, 5*time.Second, 5*time.Millisecond)
			})
		}
	}
}

func TestLateRejectionRemovesOldValueAndSkipsPathOnReconnect(t *testing.T) {
	server, err := fakeserver.New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, server.Close()) })
	c := startRejectionTestClient(t, server, WithReconnectDelays(time.Millisecond, time.Millisecond))
	initial := nextRejectionEvent(t, server)
	require.NoError(t, server.SendUpdate(initial.StreamID, &gnmipb.Update{
		Path: &gnmipb.Path{Elem: []*gnmipb.PathElem{{Name: "openconfig"}, {Name: "system"}, {Name: "state"}, {Name: "missing"}}},
		Val:  fakeserver.ScalarUint64(99),
	}))
	require.Eventually(t, func() bool {
		_, ok := c.Get(rejectedTestPath, nil)
		return ok
	}, 5*time.Second, 5*time.Millisecond)
	server.SetSubscribeErrorHandler(func(request *gnmipb.SubscribeRequest) error {
		if requestHasMissing(request) {
			return status.Error(codes.NotFound, "publisher removed")
		}
		return nil
	}, true)
	require.NoError(t, server.SendSubscriptionError(initial.StreamID, status.Error(codes.NotFound, "publisher removed")))
	var recovered fakeserver.SubscribeEvent
	for {
		event := nextRejectionEvent(t, server)
		if len(event.Request.GetSubscribe().GetSubscription()) == 2 && !requestHasMissing(event.Request) {
			recovered = event
			break
		}
	}
	require.Eventually(t, func() bool {
		_, ok := c.Get(rejectedTestPath, nil)
		return !ok && c.Synchronized()
	}, 5*time.Second, 5*time.Millisecond)
	before := server.SubscriptionCount()
	require.NoError(t, server.CloseStream(recovered.StreamID))
	reconnected := nextRejectionEvent(t, server)
	require.Len(t, reconnected.Request.GetSubscribe().GetSubscription(), 2)
	require.False(t, requestHasMissing(reconnected.Request))
	require.Equal(t, before+1, server.SubscriptionCount(), "reconnect must reuse the isolated path set")
	require.NoError(t, server.SendUpdate(reconnected.StreamID, fakeserver.InterfaceInOctetsUpdate("eth0", 7)))
	require.Eventually(t, func() bool {
		entry, ok := c.Get("/openconfig/interfaces/interface/state/counters/in-octets", map[string]string{"name": "eth0"})
		return ok && entry.Value == uint64(7)
	}, 5*time.Second, 5*time.Millisecond)
	active := c.activeSubscriptions()
	require.Len(t, active, 2)
	for _, spec := range active {
		require.NotEqual(t, rejectedTestPath, spec.Path)
	}
}

func TestConnectionFailuresDoNotQuarantinePaths(t *testing.T) {
	for _, code := range []codes.Code{codes.Unauthenticated, codes.PermissionDenied, codes.Unavailable, codes.ResourceExhausted} {
		t.Run(code.String(), func(t *testing.T) {
			server, err := fakeserver.New()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, server.Close()) })
			server.SetSubscribeErrorHandler(func(*gnmipb.SubscribeRequest) error { return status.Error(code, "connection-level failure") }, false)
			c := startRejectionTestClient(t, server)
			nextRejectionEvent(t, server)
			require.Eventually(t, func() bool { return strings.Contains(c.ConnectionStatus().LastError, code.String()) }, 5*time.Second, 5*time.Millisecond)
			require.Empty(t, c.ConnectionStatus().RejectedSubscriptions)
			require.Equal(t, 1, server.SubscriptionCount(), "connection failures must not fan out into probes")
		})
	}
}

func TestSharedEncodingErrorDoesNotDisableEveryPath(t *testing.T) {
	server, err := fakeserver.New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, server.Close()) })
	server.SetSubscribeErrorHandler(func(*gnmipb.SubscribeRequest) error { return status.Error(codes.Unimplemented, "encoding unsupported") }, false)
	c := startRejectionTestClient(t, server)
	require.Eventually(t, func() bool { return strings.Contains(c.ConnectionStatus().LastError, "without an isolated path") }, 5*time.Second, 5*time.Millisecond)
	require.Empty(t, c.ConnectionStatus().RejectedSubscriptions)
	require.LessOrEqual(t, server.SubscriptionCount(), 1+2*3)
}

func TestSilenceDoesNotDisablePaths(t *testing.T) {
	for _, delaySync := range []bool{false, true} {
		t.Run(strconv.FormatBool(delaySync), func(t *testing.T) {
			server, err := fakeserver.New()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, server.Close()) })
			server.SetDelaySync(delaySync)
			c := startRejectionTestClient(t, server)
			nextRejectionEvent(t, server)
			if !delaySync {
				require.Eventually(t, func() bool { return c.Synchronized() }, 5*time.Second, 5*time.Millisecond)
			}
			require.Empty(t, c.ConnectionStatus().RejectedSubscriptions)
			require.Empty(t, c.Snapshot())
			require.Equal(t, 1, server.SubscriptionCount())
		})
	}
}

func TestCancelDuringIsolationClosesDiagnosticRPC(t *testing.T) {
	server, err := fakeserver.New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, server.Close()) })
	server.SetDelaySync(true)
	server.SetSubscribeErrorHandler(func(r *gnmipb.SubscribeRequest) error {
		if len(r.GetSubscribe().GetSubscription()) > 1 {
			return status.Error(codes.InvalidArgument, "one path rejected")
		}
		return nil
	}, false)
	c := startRejectionTestClient(t, server)
	for {
		if len(nextRejectionEvent(t, server).Request.GetSubscribe().GetSubscription()) == 1 {
			break
		}
	}
	require.NoError(t, c.Close())
	require.Eventually(t, func() bool { return server.ActiveStreamCount() == 0 && server.ConnectionCount() == 0 }, 5*time.Second, 5*time.Millisecond)
	require.Empty(t, c.ConnectionStatus().RejectedSubscriptions)
}

func TestIsolationOnlyRecordsConfirmedSingletonRejections(t *testing.T) {
	specs := []SubscriptionSpec{{Path: "/good"}, {Path: "/missing"}, {Path: "/silent"}, {Path: "/also-missing"}}
	calls := 0
	result, err := isolateSubscriptions(context.Background(), specs, func(_ context.Context, group []SubscriptionSpec) error {
		calls++
		for _, spec := range group {
			if strings.Contains(spec.Path, "missing") {
				return status.Error(codes.NotFound, "missing")
			}
		}
		if group[0].Path == "/silent" {
			return status.Error(codes.DeadlineExceeded, "no sync")
		}
		return nil
	})
	require.NoError(t, err)
	require.True(t, result.accepted)
	require.Len(t, result.rejected, 2)
	require.Contains(t, result.rejected, subscriptionID(SubscriptionSpec{Path: "/missing"}))
	require.Contains(t, result.rejected, subscriptionID(SubscriptionSpec{Path: "/also-missing"}))
	require.LessOrEqual(t, calls, 2*len(specs))
}

func TestConnectionFailureDuringIsolationDiscardsPathDecisions(t *testing.T) {
	for _, code := range []codes.Code{codes.PermissionDenied, codes.Unavailable, codes.ResourceExhausted} {
		t.Run(code.String(), func(t *testing.T) {
			specs := []SubscriptionSpec{{Path: "/good"}, {Path: "/missing"}, {Path: "/connection-failure"}}
			result, err := isolateSubscriptions(context.Background(), specs, func(_ context.Context, group []SubscriptionSpec) error {
				if len(group) > 1 {
					return status.Error(codes.NotFound, "split batch")
				}
				switch group[0].Path {
				case "/missing":
					return status.Error(codes.NotFound, "missing")
				case "/connection-failure":
					return status.Error(code, "connection failed during isolation")
				default:
					return nil
				}
			})
			require.Equal(t, code, status.Code(err))
			require.Empty(t, result.rejected)
			require.False(t, result.accepted)
		})
	}
}

func TestIsolationHasBoundedDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		result, err := isolateSubscriptions(ctx, []SubscriptionSpec{{Path: "/a"}, {Path: "/b"}}, func(ctx context.Context, _ []SubscriptionSpec) error {
			<-ctx.Done()
			return ctx.Err()
		})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Empty(t, result.rejected)
		require.False(t, result.accepted)
	})
}

func TestRejectionsExpireOnReconnectAndAreClientLocal(t *testing.T) {
	clk := clock.NewMock()
	c := &Client{cfg: Config{Profile: rejectionTestProfile()}, opt: options{clock: clk}, rejectedSubscriptions: map[string]rejectedSubscription{
		subscriptionID(SubscriptionSpec{Path: rejectedTestPath}): {path: rejectedTestPath, err: "NotFound", retryAfter: clk.Now().Add(rejectionRetryInterval)},
	}}
	require.Len(t, c.activeSubscriptions(), 2)
	other := &Client{cfg: c.cfg, opt: c.opt}
	require.Len(t, other.activeSubscriptions(), 3)
	clk.Add(rejectionRetryInterval)
	require.Len(t, c.activeSubscriptions(), 3)
	require.Empty(t, c.rejectedSubscriptions)
}

func TestRejectionIdentityPreservesOriginAndKeyOwnership(t *testing.T) {
	legacy := subscriptionID(SubscriptionSpec{Path: "/openconfig/system/state/hostname"})
	explicit := subscriptionID(SubscriptionSpec{Path: "openconfig:/system/state/hostname"})
	require.Equal(t, legacy, explicit)
	require.NotEqual(t, explicit, subscriptionID(SubscriptionSpec{Path: "native:/system/state/hostname"}))
	require.NotEqual(t,
		subscriptionID(SubscriptionSpec{Path: "native:/a[name=x]/b[name=same]"}),
		subscriptionID(SubscriptionSpec{Path: "native:/a[name=y]/b[name=same]"}),
	)
}

func TestInBandSubscriptionErrorsPreserveCodeAndMessage(t *testing.T) {
	for _, code := range []uint32{uint32(codes.NotFound), uint32(codes.Unauthenticated), 0, 999} {
		response := &gnmipb.SubscribeResponse{Response: &gnmipb.SubscribeResponse_Error{Error: &gnmipb.Error{Code: code, Message: "device explanation"}}}
		_, err := (&Client{}).handleSubscribeResponse(newCache(), response)
		require.ErrorContains(t, err, "device explanation")
		expected := codes.Code(code)
		if expected == codes.OK || expected > codes.Unauthenticated {
			expected = codes.Unknown
		}
		require.Equal(t, expected, status.Code(err))
	}
	_, err := (&Client{}).handleSubscribeResponse(newCache(), &gnmipb.SubscribeResponse{Response: &gnmipb.SubscribeResponse_Error{}})
	require.Equal(t, codes.Unknown, status.Code(err))
}
