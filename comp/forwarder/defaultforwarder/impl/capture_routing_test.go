// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package defaultforwarderimpl

import (
	"bytes"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/internal/retry"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

type captureRoute struct {
	payloadID                       uint64
	ordinals                        []uint64
	endpoint, protocol, destination string
	at                              time.Time
}

type routeObserver struct {
	routes []captureRoute
	panics bool
}

func (o *routeObserver) ObserveRoute(id uint64, ordinals []uint64, endpoint, protocol, destination string, at time.Time) {
	o.routes = append(o.routes, captureRoute{id, slices.Clone(ordinals), endpoint, protocol, destination, at})
	if o.panics {
		panic("capture worker failed")
	}
}

func newCaptureRoutingForwarder(t *testing.T) *DefaultForwarder {
	t.Helper()
	config := configmock.New(t)
	config.SetInTest("dd_url", "https://primary.example")
	log := logmock.New(t)
	resolvers, err := resolver.NewSingleDomainResolvers(map[string][]utils.APIKeys{
		"https://primary.example": {utils.NewAPIKeys("api_key", "primary-private-key")},
		"https://extra.example":   {utils.NewAPIKeys("additional_endpoints", "extra-private-key-1", "extra-private-key-2")},
	})
	require.NoError(t, err)
	f := NewDefaultForwarder(config, log, NewOptionsWithResolvers(config, log, resolvers))
	// Exercise the normal queue boundary with no workers or network activity.
	f.internalState.Store(Started)
	for _, df := range f.domainForwarders {
		df.highPrio = make(chan transaction.Transaction, 8)
		df.internalState = Started
	}
	return f
}

func TestCaptureRoutesPreserveDeliveryAndExcludeRetries(t *testing.T) {
	for _, panics := range []bool{false, true} {
		t.Run(map[bool]string{false: "observed", true: "observer panics"}[panics], func(t *testing.T) {
			f := newCaptureRoutingForwarder(t)
			observer := &routeObserver{panics: panics}
			payload := transaction.NewBytesPayload([]byte("unchanged encoded series"), 2)
			payload.SetCapture(&transaction.CaptureMetadata{SessionID: "capture-private-session", CycleID: 10, PayloadID: 4, Ordinals: []uint64{2, 9}, Observer: observer})
			headers := http.Header{"Content-Type": {"application/x-protobuf"}, "Content-Encoding": {"deflate"}}
			before := time.Now()
			require.NoError(t, f.SubmitSeries(transaction.BytesPayloads{payload}, headers))
			require.Len(t, observer.routes, 3)
			destinations := make([]string, 0, 3)
			for _, route := range observer.routes {
				require.Equal(t, uint64(4), route.payloadID)
				require.Equal(t, []uint64{2, 9}, route.ordinals)
				require.Equal(t, "/api/v2/series", route.endpoint)
				require.Equal(t, "v2", route.protocol)
				require.False(t, route.at.Before(before))
				destinations = append(destinations, route.destination)
			}
			require.ElementsMatch(t, []string{"primary/1", "additional-1/1", "additional-1/2"}, destinations)
			for _, df := range f.domainForwarders {
				count := len(df.highPrio)
				queued := make([]*transaction.HTTPTransaction, 0, count)
				for range count {
					queued = append(queued, (<-df.highPrio).(*transaction.HTTPTransaction))
				}
				for _, txn := range queued {
					require.Same(t, payload, txn.Payload)
					require.Equal(t, []byte("unchanged encoded series"), txn.Payload.GetContent())
					require.Equal(t, headers.Get("Content-Type"), txn.Headers.Get("Content-Type"))
					require.Equal(t, headers.Get("Content-Encoding"), txn.Headers.Get("Content-Encoding"))
					for key, values := range txn.Headers {
						require.NotContains(t, strings.ToLower(key), "capture")
						require.NotContains(t, strings.Join(values, " "), "capture-private-session")
					}
					// Requeue below the initial boundary, as retries do, while
					// correlation is still attached: no extra observation occurs.
					df.sendHTTPTransactions(txn)
					require.Same(t, txn, <-df.highPrio)
				}
			}
			require.Len(t, observer.routes, 3)
			payload.ClearCapture()
			require.Nil(t, payload.Capture())
		})
	}
}

func TestCaptureMetadataNeverEntersRetryStorage(t *testing.T) {
	f := newCaptureRoutingForwarder(t)
	payload := transaction.NewBytesPayload([]byte("original series body"), 2)
	payload.SetCapture(&transaction.CaptureMetadata{SessionID: "capture-private-session", CycleID: 22, PayloadID: 17, Ordinals: []uint64{11}, Observer: &routeObserver{}})
	txns := f.createHTTPTransactions(endpoints.SeriesEndpoint, transaction.BytesPayloads{payload}, transaction.Series, nil)
	require.NotEmpty(t, txns)
	txn := txns[0]
	serializer := retry.NewHTTPTransactionsSerializer(f.log, txn.Resolver.(resolver.DomainResolver))
	require.NoError(t, serializer.Add(txn))
	encoded, err := serializer.GetBytesAndReset()
	require.NoError(t, err)
	require.False(t, bytes.Contains(encoded, []byte("capture-private-session")))
	decoded, failures, err := serializer.Deserialize(encoded)
	require.NoError(t, err)
	require.Zero(t, failures)
	require.Len(t, decoded, 1)
	replayed := decoded[0].(*transaction.HTTPTransaction)
	require.Nil(t, replayed.Payload.Capture())
	require.Equal(t, payload.GetContent(), replayed.Payload.GetContent())
	require.Equal(t, payload.GetPointCount(), replayed.Payload.GetPointCount())
}

func TestCaptureRouteRejectsUnknownEndpointWithoutExportingIt(t *testing.T) {
	f := newCaptureRoutingForwarder(t)
	observer := &routeObserver{}
	payload := transaction.NewBytesPayloadWithoutMetaData([]byte("body"))
	payload.SetCapture(&transaction.CaptureMetadata{Observer: observer})
	txns := f.createHTTPTransactions(transaction.Endpoint{Route: "https://private.example/path?api_key=secret", Name: "custom"}, transaction.BytesPayloads{payload}, transaction.Series, nil)
	require.NoError(t, f.sendHTTPTransactions(txns))
	require.Len(t, observer.routes, 3)
	for _, route := range observer.routes {
		require.Empty(t, route.endpoint)
		require.Empty(t, route.protocol)
		require.NotContains(t, route.destination, "example")
	}
}
