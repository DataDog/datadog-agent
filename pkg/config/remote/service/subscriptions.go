// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"slices"
	"time"

	"golang.org/x/time/rate"

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
	defaultMaxSubscriptionQueueSize = 16

	// The maximum number of config subscriptions that may be active at the
	// same time. We only expect there to ever be one in a real production
	// setting: the system-probe, but perhaps for testing or debugging use
	// cases it may be beneficial to allow for an additional subscription.
	defaultMaxConcurrentSubscriptions = 2

	// The maximum number of runtime IDs that may be tracked per subscription.
	// This limit exists to bound the memory use due to a subscription -- at the
	// cost of breaking subscriptions if there are too many such processes.
	//
	// Note that the memory usage per tracked runtime ID is roughly 36 bytes
	// for the runtime ID string bytes, 16 bytes for the string header, and
	// 8 bytes for the trackedClient value so 60 bytes per runtime ID and then
	// some additional overhead for the map. Call it 100 bytes max.
	defaultMaxTrackedRuntimeIDsPerSubscription = 16 << 10 // 16K
)

// trackedClient holds the state for a single tracked client in a
// subscription.
type trackedClient struct {
	seenAny  bool
	products pbgo.ConfigSubscriptionProducts
}

// subscriptionID is a unique identifier for a subscription.
type subscriptionID int

// subscription represents a single active config subscription stream.
// All methods must be called while holding CoreAgentService.mu.
type subscription struct {
	id subscriptionID

	// trackedClients maps runtime_id -> tracked client state
	//
	// Note that while it might be more convenient to use a pointer value, we
	// save memory by using a value type. This makes updates a bit trickier, but
	// this map is the largest in memory object in the subscription, so it's
	// worth optimizing for memory.
	trackedClients map[string]trackedClient
	// pendingQueue is a queue of responses waiting to be sent.
	pendingQueue []*pbgo.ConfigSubscriptionResponse
	// maxQueueSize is the maximum number of responses that can be queued.
	// This bounds the memory usage of the subscription in the face of a
	// stuck or slow client.
	maxQueueSize int
	// updateSignal is used to notify the sender goroutine that there are
	// pending updates to send.
	updateSignal chan<- struct{}
}

// newSubscription creates a new subscription. The caller is responsible for
// starting the sender goroutine.
func newSubscription(id subscriptionID, maxQueueSize int) (*subscription, <-chan struct{}) {
	updateSignal := make(chan struct{}, 1)
	return &subscription{
		id:             id,
		trackedClients: make(map[string]trackedClient),
		maxQueueSize:   maxQueueSize,
		updateSignal:   updateSignal,
	}, updateSignal
}

// track adds a client to the subscription's tracked clients.
// Must be called while holding CoreAgentService.mu.
func (s *subscription) track(runtimeID string, products pbgo.ConfigSubscriptionProducts) {
	s.trackedClients[runtimeID] = trackedClient{
		seenAny:  false,
		products: products,
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

// If we have a stuck subscription client, rate limit logging about it to avoid
// spamming the logs.
var queueFullLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)

// enqueueUpdate enqueues an update for a tracked runtime_id with the provided
// files. If the runtime_id is not tracked, this is a no-op.  If the queue is
// full, the update, and all pending updates for this runtime_id will be
// dropped. Subsequent polls for this runtime_id will resend all the files.
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
	tracked, ok := s.trackedClients[runtimeID]
	if !ok {
		return
	}

	// Check if queue is full, and if so, we're going to drop the update.
	if len(s.pendingQueue) >= s.maxQueueSize {
		if queueFullLogLimiter.Allow() {
			log.Warnf(
				"subscription %d: queue is full (%d), dropping update for runtime_id %s",
				s.id, s.maxQueueSize, runtimeID,
			)
		} else {
			log.Debugf(
				"subscription %d: queue is full (%d), dropping update for runtime_id %s",
				s.id, s.maxQueueSize, runtimeID,
			)
		}
		// Remove any existing update for this runtime_id from the queue. The
		// next poll that occurs for this client will resend all the files.
		s.removeFromQueue(runtimeID)
		tracked.seenAny = false
		s.trackedClients[runtimeID] = tracked
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
	idAlloc                  subscriptionID
	subs                     map[subscriptionID]*subscription
	productsMappings         productsMappings
	maxSubscriptionQueueSize int
}

type productSet = map[rdata.Product]struct{}

type productsMappings = map[pbgo.ConfigSubscriptionProducts]productSet

// newSubscriptions creates a new subscriptions manager.
func newSubscriptions(pm productsMappings, maxSubscriptionQueueSize int) *subscriptions {
	return &subscriptions{
		idAlloc:                  0,
		subs:                     make(map[subscriptionID]*subscription),
		productsMappings:         pm,
		maxSubscriptionQueueSize: maxSubscriptionQueueSize,
	}
}

// add adds a subscription to the manager and returns it.
// Must be called while holding CoreAgentService.mu.
func (s *subscriptions) newSubscription() (
	_ subscriptionID, updateSignal <-chan struct{},
) {
	s.idAlloc++
	id := s.idAlloc
	sub, updateSignal := newSubscription(id, s.maxSubscriptionQueueSize)
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
) (interestedSubs []subscriptionID, needCompleteProducts productSet) {
	runtimeID := getRuntimeIDFromClient(client)
	if runtimeID == "" {
		return nil, nil
	}

	var needProducts productSet
	for subID, sub := range s.subs {
		// Check if this subscription is tracking this runtime_id.
		tracked, ok := sub.trackedClients[runtimeID]
		if !ok {
			// Not tracking this runtime_id.
			continue
		}

		// This subscription is interested.
		interestedSubs = append(interestedSubs, subID)

		// If this subscription hasn't seen this client before, it needs the
		// complete set of configs for the tracked products.
		if !tracked.seenAny {
			if needProducts == nil {
				needProducts = make(productSet)
			}
			for product := range s.productsMappings[tracked.products] {
				needProducts[product] = struct{}{}
			}
		}
	}

	return interestedSubs, needProducts
}

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

		tracked, ok := sub.trackedClients[runtimeID]
		if !ok {
			continue
		}

		// If this subscription hasn't seen this client before, it needs all
		// files, not just the new target files.
		files := responseFiles
		if !tracked.seenAny {
			files = allFiles
			tracked.seenAny = true
			sub.trackedClients[runtimeID] = tracked
		}
		products := s.productsMappings[tracked.products]
		files = filtered(files, func(file *pbgo.File) bool {
			return contains(products, productFromPath(file.Path))
		})
		configs := filtered(matchedClientConfigs, func(config string) bool {
			return contains(products, productFromPath(config))
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
		if _, ok := sub.trackedClients[runtimeID]; ok {
			return update
		}
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

func productFromPath(path string) rdata.Product {
	parsed, err := rdata.ParseConfigPath(path)
	if err != nil {
		return ""
	}
	return rdata.Product(parsed.Product)
}
