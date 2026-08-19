// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"cmp"
	"errors"
	"slices"
	"sync"
	"time"

	"golang.org/x/time/rate"

	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// errMaxSubscriptionsReached is returned by newSubscription when the maximum
// number of concurrent subscriptions has already been reached.
var errMaxSubscriptionsReached = errors.New("maximum number of subscriptions reached")

// errMaxTrackedRuntimeIDsReached is returned by track when a subscription has
// already reached the maximum number of tracked runtime IDs.
var errMaxTrackedRuntimeIDsReached = errors.New("maximum number of runtime IDs per subscription reached")

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
// It has no locking of its own: all of its methods are internal helpers only
// ever called from subscriptions methods, which take the subscriptions'
// mutex before touching any subscription value.
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
// Must be called while holding the owning subscriptions' mu.
func (s *subscription) track(runtimeID string, products pbgo.ConfigSubscriptionProducts) {
	s.trackedClients[runtimeID] = trackedClient{
		seenAny:  false,
		products: products,
	}
}

// untrack removes a client from the subscription's tracked clients and
// removes any pending updates for that client from the queue.
// Must be called while holding the owning subscriptions' mu.
func (s *subscription) untrack(runtimeID string) {
	delete(s.trackedClients, runtimeID)

	// Remove any pending updates for this runtime_id from the queue.
	s.removeFromQueue(runtimeID)
}

// removeFromQueue removes all responses for the given runtime_id from the
// pending queue.
// Must be called while holding the owning subscriptions' mu.
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
// Must be called while holding the owning subscriptions' mu.
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
//
// It has its own internal locking (mu) independent of CoreAgentService.mu, so
// that ClientGetConfigs's read path -- which calls interestedSubscriptions and
// notify on every single poll -- never needs to take CoreAgentService.mu.
// interestedSubscriptions takes a read lock, since it runs on every poll and
// must not serialize concurrent pollers against each other; every method that
// mutates subscription state (track, untrack, newSubscription, remove,
// notify, popUpdate) takes the write lock.
type subscriptions struct {
	mu sync.RWMutex

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

// newSubscription creates and registers a new subscription, returning its ID
// and update channel. It returns errMaxSubscriptionsReached if the manager
// already has maxConcurrent subscriptions registered. Safe for concurrent
// use.
func (s *subscriptions) newSubscription(maxConcurrent int) (
	_ subscriptionID, updateSignal <-chan struct{}, err error,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.subs) >= maxConcurrent {
		return 0, nil, errMaxSubscriptionsReached
	}
	s.idAlloc++
	id := s.idAlloc
	sub, updateSignal := newSubscription(id, s.maxSubscriptionQueueSize)
	s.subs[id] = sub
	return id, updateSignal, nil
}

// remove removes a subscription from the manager. Safe for concurrent use.
func (s *subscriptions) remove(id subscriptionID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, id)
}

// count returns the number of currently registered subscriptions. Safe for
// concurrent use.
func (s *subscriptions) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subs)
}

// interestedSubscriptions returns the IDs of subscriptions tracking the given
// runtime_id along with the set of products that still require a full payload
// (i.e. the subscriptions have never seen this client and need all files for
// those products). Safe for concurrent use; takes only a read lock since this
// runs on every single ClientGetConfigs poll and must not serialize pollers
// against each other.
func (s *subscriptions) interestedSubscriptions(
	client *pbgo.Client,
) (interestedSubs []subscriptionID, needCompleteProducts productSet) {
	runtimeID := getRuntimeIDFromClient(client)
	if runtimeID == "" {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

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

// notify enqueues updates for the subscriptions in toNotify that are tracking
// client. Each subscription receives only the configs and files matching its
// tracked products. Safe for concurrent use.
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

	s.mu.Lock()
	defer s.mu.Unlock()

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

// popUpdate pops one update from the subscription's queue. Safe for
// concurrent use.
func (s *subscriptions) popUpdate(
	id subscriptionID,
) *pbgo.ConfigSubscriptionResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
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

// track adds runtimeID to the tracked clients of subscription id, returning
// the new total number of tracked clients across all subscriptions (for
// telemetry). It returns errMaxTrackedRuntimeIDsReached if the subscription
// has already reached maxTrackedPerSub tracked runtime IDs. If the
// subscription no longer exists (e.g. concurrently removed), this is a no-op.
// Safe for concurrent use.
func (s *subscriptions) track(
	id subscriptionID, runtimeID string, products pbgo.ConfigSubscriptionProducts, maxTrackedPerSub int,
) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub := s.subs[id]
	if sub == nil {
		return s.totalTrackedClientsLocked(), nil
	}
	if len(sub.trackedClients) >= maxTrackedPerSub {
		return 0, errMaxTrackedRuntimeIDsReached
	}
	sub.track(runtimeID, products)
	return s.totalTrackedClientsLocked(), nil
}

// untrack removes runtimeID from the tracked clients of subscription id,
// returning the new total number of tracked clients across all subscriptions
// (for telemetry). If the subscription no longer exists, this is a no-op.
// Safe for concurrent use.
func (s *subscriptions) untrack(id subscriptionID, runtimeID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sub := s.subs[id]; sub != nil {
		sub.untrack(runtimeID)
	}
	return s.totalTrackedClientsLocked()
}

// totalTrackedClientsLocked returns the total number of tracked clients
// across all subscriptions. Must be called while holding s.mu (read or
// write).
func (s *subscriptions) totalTrackedClientsLocked() int {
	n := 0
	for _, sub := range s.subs {
		n += len(sub.trackedClients)
	}
	return n
}

// states returns a snapshot of the tracked-client state of every currently
// registered subscription, for use by ConfigGetState. Safe for concurrent
// use.
func (s *subscriptions) states() []*pbgo.ConfigSubscriptionState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var states []*pbgo.ConfigSubscriptionState
	for _, sub := range s.subs {
		trackedClients := make([]*pbgo.ConfigSubscriptionState_TrackedClient, 0, len(sub.trackedClients))
		for runtimeID, trackedClient := range sub.trackedClients {
			trackedClients = append(trackedClients, &pbgo.ConfigSubscriptionState_TrackedClient{
				RuntimeId: runtimeID,
				SeenAny:   trackedClient.seenAny,
				Products:  trackedClient.products,
			})
		}
		slices.SortFunc(trackedClients, func(a, b *pbgo.ConfigSubscriptionState_TrackedClient) int {
			return cmp.Compare(a.RuntimeId, b.RuntimeId)
		})
		states = append(states, &pbgo.ConfigSubscriptionState{
			TrackedClients: trackedClients,
		})
	}
	return states
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
