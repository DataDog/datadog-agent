// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Ensure in compile time that the RingStore satisfies the Store interface
var _ Store = &RingStore{}

// Store defines an interface to store process events in-memory
type Store interface {
	// Run starts the store
	Run()
	// Stop stops the store
	Stop()
	// Push sends an event to be stored. An optional channel can be passed to acknowledge when the event is successfully written
	Push(*model.ProcessEvent, chan bool) error
	// Pull fetches all events in the store that haven't been consumed yet
	Pull(context.Context, time.Duration) ([]*model.ProcessEvent, error)
}

// ringNode represents an event stored in the RingStore buffer
// It can eventually hold more data as lastUpdate, event hash etc
type ringNode struct {
	data *model.ProcessEvent
}

// pushRequest is a request sent to the internal RingBuffer routine to add an event to its buffer
type pushRequest struct {
	event *model.ProcessEvent
	done  chan bool
}

// pullRequest is a request sent to the internal RingBuffer routine to fetch events from its buffer
type pullRequest struct {
	results chan []*model.ProcessEvent
}

// RingStore implements the Store interface using a ring buffer
// The buffer is accessed by a single routine so it doesn't need to be protected by a mutex
// It holds two pointers, head and tail, to access the ring buffer
// head points to the oldest event in the buffer, where data should be consumed from
// tail points to the node where the next event will be inserted into
// head = tail if
//   - the store is empty, in which case the underlying ringNode doesn't have any data
//   - the store is full. Subsequent Push operations override the data pointed by head and move both head and tail
//     to the next position
type RingStore struct {
	head   int
	tail   int
	buffer []ringNode

	dropHandler EventHandler // applied to an event before it's dropped. Used for test's purposes
	pushReq     chan *pushRequest
	pullReq     chan *pullRequest

	wg   sync.WaitGroup
	exit chan struct{}

	statsdClient  statsd.ClientInterface
	statsInterval time.Duration
	expiredInput  *atomic.Int64 // how many events have been dropped due to a full pushReq channel
	expiredBuffer *atomic.Int64 // how many events have been dropped due to a full buffer
}

// readPositiveInt reads a config stored in the given key and asserts that it's a positive value
func readPositiveInt(cfg config.ConfigReader, key string) (int, error) {
	i := cfg.GetInt(key)
	if i <= 0 {
		return 0, fmt.Errorf("invalid setting. %s must be > 0", key)
	}

	return i, nil
}

// NewRingStore creates a new RingStore to store process events
func NewRingStore(cfg config.ConfigReader, client statsd.ClientInterface) (Store, error) {
	maxItems, err := readPositiveInt(cfg, "process_config.event_collection.store.max_items")
	if err != nil {
		return nil, err
	}

	maxPushes, err := readPositiveInt(cfg, "process_config.event_collection.store.max_pending_pushes")
	if err != nil {
		return nil, err
	}

	maxPulls, err := readPositiveInt(cfg, "process_config.event_collection.store.max_pending_pulls")
	if err != nil {
		return nil, err
	}

	statsInterval, err := readPositiveInt(cfg, "process_config.event_collection.store.stats_interval")
	if err != nil {
		return nil, err
	}

	return &RingStore{
		buffer:        make([]ringNode, maxItems),
		head:          0,
		tail:          0,
		pushReq:       make(chan *pushRequest, maxPushes),
		pullReq:       make(chan *pullRequest, maxPulls),
		exit:          make(chan struct{}),
		statsdClient:  client,
		statsInterval: time.Duration(statsInterval) * time.Second,
		expiredInput:  atomic.NewInt64(0),
		expiredBuffer: atomic.NewInt64(0),
	}, nil
}

// Run starts the RingStore. A go routine is created to serve push and pull requests in order to protect the underlying
// storage from concurrent access
func (s *RingStore) Run() {
	log.Info("Starting the RingStore")
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.run()
	}()
}

// Stop stops the RingStore's internal routine
func (s *RingStore) Stop() {
	log.Info("Stopping the RingStore")
	close(s.exit)
	s.wg.Wait()
	log.Info("RingStore stopped")
}

// Push adds an event to the RingStore. If the store is full, the oldest event is dropped to make space for the new one
// The done channel is optional. It's used to signal if the event has been successfully written to the Store
func (s *RingStore) Push(e *model.ProcessEvent, done chan bool) error {
	r := &pushRequest{
		event: e,
		done:  done,
	}

	select {
	case s.pushReq <- r:
	default:
		log.Trace("RingStore input channel is full, dropping event")
		s.expiredInput.Inc()

		if done != nil {
			done <- false
		}

		return errors.New("too many pending push requests")
	}

	return nil
}

// Pull returns all events stored in the RingStore
func (s *RingStore) Pull(ctx context.Context, timeout time.Duration) ([]*model.ProcessEvent, error) {
	q := &pullRequest{
		results: make(chan []*model.ProcessEvent),
	}

	select {
	case s.pullReq <- q:
	default:
		log.Warn("Can't Pull RingStore: too many pending requests")
		return nil, errors.New("too many pending pull requests")
	}

	var batch []*model.ProcessEvent
	timer := time.NewTimer(timeout)
	select {
	case batch = <-q.results:
		timer.Stop()
		break
	case <-timer.C:
		log.Warn("Timed out while fetching events from the RingStore")
		return nil, errors.New("pull request timed out")
	}

	return batch, nil
}

// size returns how many events are currently stored in the RingStore
// This function is not thread-safe and should only be called internally by the RingStore routine or during tests
func (s *RingStore) size() int {
	// When buffer is full, tail points to a node whose data that hasn't been consumed yet
	if s.buffer[s.tail].data != nil {
		return len(s.buffer)
	}

	if s.head <= s.tail {
		return s.tail - s.head
	}

	return (len(s.buffer) - s.head) + s.tail
}

// push adds an event to the RingStore buffer
// This function is not thread-safe and should only be called internally by the RingStore routine or during tests
func (s *RingStore) push(e *model.ProcessEvent) {
	if old := s.buffer[s.tail].data; old != nil {
		if s.dropHandler != nil {
			s.dropHandler(old)
		}

		// When store is full, tail = head. Head is moved since the pointed data will be dropped to make space for the new event
		s.head = (s.head + 1) % len(s.buffer)
		log.Tracef("Dropping %s event with PID %d and exe %s", old.EventType, old.Pid, old.Exe)
		s.expiredBuffer.Inc()
	}

	s.buffer[s.tail].data = e
	s.tail = (s.tail + 1) % len(s.buffer)
}

// pull returns all events stored in the RingStore buffer
// This function is not thread-safe and should only be called internally by the RingStore routine or during tests
func (s *RingStore) pull() []*model.ProcessEvent {
	size := s.size()
	batch := make([]*model.ProcessEvent, 0, size)

	// Iterate though the buffer consuming all nodes that still have data
	for ; s.buffer[s.head].data != nil; s.head = (s.head + 1) % len(s.buffer) {
		batch = append(batch, s.buffer[s.head].data)
		s.buffer[s.head].data = nil // do not hold more reference to the data so it can be GC'ed.
	}

	return batch
}

func (s *RingStore) sendStats() {
	inputCount := s.expiredInput.Swap(0)
	bufferCount := s.expiredBuffer.Swap(0)

	if err := s.statsdClient.Count("datadog.process.events.expired", inputCount, []string{"type:input_full"}, 1.0); err != nil {
		log.Debug(err)
	}

	if err := s.statsdClient.Count("datadog.process.events.expired", bufferCount, []string{"type:buffer_full"}, 1.0); err != nil {
		log.Debug(err)
	}
}

// run listens for requests sent to the RingStore channels
func (s *RingStore) run() {
	statsTicker := time.NewTicker(s.statsInterval)
	defer statsTicker.Stop()

	for {
		select {
		case req := <-s.pushReq:
			s.push(req.event)
			if req.done != nil {
				req.done <- true
			}
		case req := <-s.pullReq:
			req.results <- s.pull()
		case <-statsTicker.C:
			s.sendStats()
		case <-s.exit:
			return
		}
	}
}
