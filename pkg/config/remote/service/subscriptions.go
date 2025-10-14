// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"slices"

	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Maximum number of responses that can be queued per subscription.
	//
	// This number is arbitrary, but should bound the memory usage and avoid
	// quadratic runtime when interacting with the queue. Generally we don't
	// expect much queueing of updates at all -- it would only happen if the
	// system-probe fell behind or couldn't keep up with the rate of updates.
	// Given the polling by clients happens on the order of seconds, this is
	// very unlikely to happen. Even if it does, the subscription will get
	// notified on the next poll.
	maxSubscriptionQueueSize = 16
)

// trackedClient holds the state for a single tracked client in a
// subscription.
type trackedClient struct {
	seenAny  bool
	products map[string]struct{}
}

// subscriptionID is a unique identifier for a subscription.
type subscriptionID int

// subscription represents a single active config subscription stream.
// All methods must be called while holding CoreAgentService.mu.
type subscription struct {
	id subscriptionID

	// trackedClients maps runtime_id -> tracked client state
	trackedClients map[string]*trackedClient
	// pendingQueue is a queue of responses waiting to be sent
	pendingQueue []*pbgo.ConfigSubscriptionResponse
	// maxQueueSize is the maximum number of responses that can be queued
	maxQueueSize int
	// updateSignal is used to notify the sender goroutine that there are
	// pending updates to send.
	updateSignal chan<- struct{}
}

// newSubscription creates a new subscription. The caller is responsible for
// starting the sender goroutine.
func newSubscription(id subscriptionID) (*subscription, <-chan struct{}) {
	updateSignal := make(chan struct{}, 1)
	return &subscription{
		id:             id,
		trackedClients: make(map[string]*trackedClient),
		maxQueueSize:   maxSubscriptionQueueSize,
		updateSignal:   updateSignal,
	}, updateSignal
}

// track adds a client to the subscription's tracked clients.
// Must be called while holding CoreAgentService.mu.
func (s *subscription) track(runtimeID string, products []string) {
	productSet := make(map[string]struct{}, len(products))
	for _, product := range products {
		productSet[product] = struct{}{}
	}
	s.trackedClients[runtimeID] = &trackedClient{
		products: productSet,
	}
}

// untrack removes a client from the subscription's tracked clients and
// removes any pending updates for that client from the queue.
// Must be called while holding CoreAgentService.mu.
func (s *subscription) untrack(runtimeID string) {
	delete(s.trackedClients, runtimeID)

	// Remove any pending updates for this runtime_id from the queue.
	s.removeFromQueue(runtimeID)
}

// removeFromQueue removes all responses for the given runtime_id from the
// pending queue.
// Must be called while holding CoreAgentService.mu.
func (s *subscription) removeFromQueue(runtimeID string) {
	s.pendingQueue = slices.DeleteFunc(s.pendingQueue, func(
		response *pbgo.ConfigSubscriptionResponse,
	) bool {
		return getRuntimeIDFromClient(response.Client) == runtimeID
	})
}

// enqueueUpdate enqueues an update for a tracked runtime_id with the provided
// files. If the runtime_id is not tracked, this is a no-op. If there's
// already a pending update for this runtime_id in the queue, it will be
// removed and replaced with the new one. If the queue is full, the update will
// be dropped.
func (s *subscription) enqueueUpdate(
	client *pbgo.Client,
	matchedConfigs []string,
	files []*pbgo.File,
) {
	runtimeID := getRuntimeIDFromClient(client)
	if runtimeID == "" {
		return
	}

	// If the client is no longer tracked, this is a no-op.
	tracked := s.trackedClients[runtimeID]
	if tracked == nil {
		return
	}

	// Remove any existing update for this runtime_id from the queue.
	s.removeFromQueue(runtimeID)

	// Check if queue is full after removing old entry.
	if len(s.pendingQueue) >= s.maxQueueSize {
		log.Warnf(
			"subscription %d: queue is full (%d), dropping update for runtime_id %s",
			s.id, s.maxQueueSize, runtimeID,
		)
		// Ensure that the client will receive all files on the next update.
		s.trackedClients[runtimeID].seenAny = false
		return
	}

	response := &pbgo.ConfigSubscriptionResponse{
		Client:         client,
		MatchedConfigs: matchedConfigs,
		TargetFiles:    files,
	}

	s.pendingQueue = append(s.pendingQueue, response)

	// Signal the sender goroutine (non-blocking).
	select {
	case s.updateSignal <- struct{}{}:
	default:
		// Already signaled, which is fine.
	}
}

// subscriptions manages all active config subscriptions.
// All methods must be called while holding CoreAgentService.mu.
type subscriptions struct {
	idAlloc subscriptionID
	subs    map[subscriptionID]*subscription
}

// newSubscriptions creates a new subscriptions manager.
func newSubscriptions() *subscriptions {
	return &subscriptions{
		idAlloc: 0,
		subs:    make(map[subscriptionID]*subscription),
	}
}

// add adds a subscription to the manager and returns it.
// Must be called while holding CoreAgentService.mu.
func (s *subscriptions) newSubscription() (_ subscriptionID, updateSignal <-chan struct{}) {
	s.idAlloc++
	id := s.idAlloc
	sub, updateSignal := newSubscription(id)
	s.subs[id] = sub
	return id, updateSignal
}

// remove removes a subscription from the manager.
// Must be called while holding CoreAgentService.mu.
func (s *subscriptions) remove(id subscriptionID) {
	delete(s.subs, id)
}

// interestedSubscriptions returns the IDs of subscriptions tracking the given
// runtime_id along with the set of products that still require a full payload
// (i.e. the subscriptions have never seen this client and need all files for
// those products). Must be called while holding CoreAgentService.mu.
func (s *subscriptions) interestedSubscriptions(
	client *pbgo.Client,
) (interestedSubs []subscriptionID, needCompleteProducts map[string]struct{}) {
	runtimeID := getRuntimeIDFromClient(client)
	if runtimeID == "" {
		return nil, nil
	}

	var needProducts map[string]struct{}
	for subID, sub := range s.subs {
		// Check if this subscription is tracking this runtime_id.
		tracked := sub.trackedClients[runtimeID]
		if tracked == nil {
			// Not tracking this runtime_id.
			continue
		}

		// This subscription is interested.
		interestedSubs = append(interestedSubs, subID)

		// If this subscription hasn't seen this client before, it needs the
		// complete set of configs for the tracked products.
		if !tracked.seenAny {
			if needProducts == nil {
				needProducts = make(map[string]struct{}, len(tracked.products))
			}
			for product := range tracked.products {
				needProducts[product] = struct{}{}
			}
		}
	}

	return interestedSubs, needProducts
}

// notifySubscriptions notifies the given subscriptions with updates for the
// client. Each subscription receives only the configs and files matching its
// tracked products.
func (s *subscriptions) notify(
	toNotify []subscriptionID,
	client *pbgo.Client,
	matchedClientConfigs []string,
	responseFiles, allFiles []*pbgo.File,
) {
	runtimeID := getRuntimeIDFromClient(client)
	if runtimeID == "" {
		return
	}

	for _, id := range toNotify {
		sub := s.subs[id]
		if sub == nil {
			continue
		}

		tracked := sub.trackedClients[runtimeID]
		if tracked == nil {
			continue
		}

		// If this subscription hasn't seen this client before, it needs all
		// files, not just the new target files.
		files := responseFiles
		if !tracked.seenAny {
			files = allFiles
		}
		files = filtered(files, func(file *pbgo.File) bool {
			return contains(tracked.products, productFromPath(file.Path))
		})
		configs := filtered(matchedClientConfigs, func(config string) bool {
			return contains(tracked.products, productFromPath(config))
		})
		sub.enqueueUpdate(client, configs, files)
	}
}

// popUpdate pops one update from the subscription's queue.
func (s *subscriptions) popUpdate(
	id subscriptionID,
) *pbgo.ConfigSubscriptionResponse {
	sub := s.subs[id]
	if sub == nil || len(sub.pendingQueue) == 0 {
		return nil
	}
	for len(sub.pendingQueue) > 0 {
		update := sub.pendingQueue[0]
		sub.pendingQueue[0], sub.pendingQueue = nil, sub.pendingQueue[1:]
		runtimeID := getRuntimeIDFromClient(update.Client)
		tracked := sub.trackedClients[runtimeID]
		if tracked == nil {
			continue
		}
		tracked.seenAny = true
		return update
	}
	return nil
}

// filtered returns a new slice containing only the items that satisfy the
// predicate.
func filtered[T any](slice []T, predicate func(T) bool) []T {
	var filtered []T
	for _, item := range slice {
		if predicate(item) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func contains[M ~map[K]V, K comparable, V any](m M, key K) bool {
	_, ok := m[key]
	return ok
}

func getRuntimeIDFromClient(client *pbgo.Client) string {
	if !client.IsTracer || client.ClientTracer == nil {
		return ""
	}
	return client.ClientTracer.RuntimeId
}

func productFromPath(path string) string {
	parsed, err := rdata.ParseConfigPath(path)
	if err != nil {
		return ""
	}
	return parsed.Product
}
