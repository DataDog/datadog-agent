// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// fakeConfigFetcher answers every poll with an empty config set and records the
// products each poll asked for.
type fakeConfigFetcher struct {
	mu       sync.Mutex
	requests [][]string
}

func (f *fakeConfigFetcher) ClientGetConfigs(_ context.Context, req *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req.Client.GetProducts())
	return &pbgo.ClientGetConfigsResponse{}, nil
}

func (f *fakeConfigFetcher) polledProducts() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

// recordingSubscriber captures the updates delivered to a subscription.
type recordingSubscriber struct {
	mu      sync.Mutex
	updates []map[string]state.RawConfig
}

func (r *recordingSubscriber) onUpdate(updates map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = append(r.updates, updates)
}

func (r *recordingSubscriber) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.updates)
}

// TestWithInitialUpdate_DeliversEmptyConfigSet covers the guarantee a plain
// subscription does not give: a poll that returned nothing for the product still
// reaches the subscriber, so it can tell an empty backend answer from a cache
// that was never filled.
func TestWithInitialUpdate_DeliversEmptyConfigSet(t *testing.T) {
	c, err := NewClient(&fakeConfigFetcher{}, WithoutTufVerification())
	require.NoError(t, err)

	sub := &recordingSubscriber{}
	c.SubscribeAll(state.ProductApmPolicies, NewUpdateListener(sub.onUpdate), WithInitialUpdate())

	require.NoError(t, c.update())
	require.Equal(t, 1, sub.count())
	require.Empty(t, sub.updates[0])

	// The guarantee is one-shot: later polls that change nothing stay silent.
	require.NoError(t, c.update())
	require.Equal(t, 1, sub.count())
}

// TestSubscribe_DoesNotDeliverEmptyConfigSet documents the default behaviour
// that makes WithInitialUpdate necessary.
func TestSubscribe_DoesNotDeliverEmptyConfigSet(t *testing.T) {
	c, err := NewClient(&fakeConfigFetcher{}, WithoutTufVerification())
	require.NoError(t, err)

	sub := &recordingSubscriber{}
	c.Subscribe(state.ProductApmPolicies, sub.onUpdate)

	require.NoError(t, c.update())
	require.Zero(t, sub.count())
}

// TestWithInitialUpdate_WaitsForAPollCarryingTheProduct is the property callers
// gate behaviour on: polls that completed before the subscription never asked for
// the product, so they cannot serve as its initial update.
func TestWithInitialUpdate_WaitsForAPollCarryingTheProduct(t *testing.T) {
	fetcher := &fakeConfigFetcher{}
	c, err := NewClient(fetcher, WithProducts(state.ProductAgentConfig), WithoutTufVerification())
	require.NoError(t, err)

	// A poll completes before anyone subscribes to APM_POLICIES.
	require.NoError(t, c.update())

	sub := &recordingSubscriber{}
	c.SubscribeAll(state.ProductApmPolicies, NewUpdateListener(sub.onUpdate), WithInitialUpdate())
	require.Zero(t, sub.count())

	require.NoError(t, c.update())
	require.Equal(t, 1, sub.count())

	polled := fetcher.polledProducts()
	require.Equal(t, []string{state.ProductAgentConfig}, polled[0])
	require.Contains(t, polled[1], state.ProductApmPolicies)
}

// TestWithInitialUpdate_LeavesOtherSubscribersAlone checks the opt-in is per
// subscription: a subscriber to the same product that did not ask for an initial
// update keeps seeing changes only.
func TestWithInitialUpdate_LeavesOtherSubscribersAlone(t *testing.T) {
	c, err := NewClient(&fakeConfigFetcher{}, WithoutTufVerification())
	require.NoError(t, err)

	optedIn, plain := &recordingSubscriber{}, &recordingSubscriber{}
	c.SubscribeAll(state.ProductApmPolicies, NewUpdateListener(optedIn.onUpdate), WithInitialUpdate())
	c.Subscribe(state.ProductApmPolicies, plain.onUpdate)

	require.NoError(t, c.update())
	require.Equal(t, 1, optedIn.count())
	require.Zero(t, plain.count())
}

// TestHasSyncedProduct_OnlyForProductsOnTheWire covers the signal callers use to
// read GetConfigs as an answer: a poll answers for the products it requested and
// for nothing else.
func TestHasSyncedProduct_OnlyForProductsOnTheWire(t *testing.T) {
	c, err := NewClient(&fakeConfigFetcher{}, WithProducts(state.ProductAgentConfig), WithoutTufVerification())
	require.NoError(t, err)

	require.False(t, c.hasSyncedProduct(state.ProductAgentConfig), "no poll has completed yet")

	require.NoError(t, c.update())
	require.True(t, c.hasSyncedProduct(state.ProductAgentConfig))
	require.False(t, c.hasSyncedProduct(state.ProductApmPolicies), "that poll never asked for it")

	// Subscribing does not backdate the sync: it takes another poll.
	c.Subscribe(state.ProductApmPolicies, func(map[string]state.RawConfig, func(string, state.ApplyStatus)) {})
	require.False(t, c.hasSyncedProduct(state.ProductApmPolicies))

	require.NoError(t, c.update())
	require.True(t, c.hasSyncedProduct(state.ProductApmPolicies))
}

// TestWithInitialUpdate_DeliversDuringSubscribeWhenAlreadySynced covers the case
// the cluster-agent actually hits: the client has already polled by the time the
// subscriber shows up, so the answer it holds must not wait for another poll.
func TestWithInitialUpdate_DeliversDuringSubscribeWhenAlreadySynced(t *testing.T) {
	c, err := NewClient(&fakeConfigFetcher{}, WithProducts(state.ProductApmPolicies), WithoutTufVerification())
	require.NoError(t, err)
	require.NoError(t, c.update())

	sub := &recordingSubscriber{}
	c.SubscribeAll(state.ProductApmPolicies, NewUpdateListener(sub.onUpdate), WithInitialUpdate())

	require.Equal(t, 1, sub.count(), "delivery should happen during Subscribe, not on the next poll")

	// And it stays a one-shot.
	require.NoError(t, c.update())
	require.Equal(t, 1, sub.count())
}
