// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/benbjohnson/clock"
)

const (
	schedulerName                            = "configfiles-discovery"
	configCollectionQueueSize                = 128
	configCollectionBatchMaxWait             = time.Second
	configCollectionBatchMaxCollectedConfigs = 100
	configCollectionBatchMaxRawConfigBytes   = 4 * 1024 * 1024 // 4MiB

	defaultHeartbeatInterval      = time.Hour
	defaultHeartbeatJitter        = 10 * time.Minute
	maxHeartbeatJitter            = time.Hour
	defaultStartupJitter          = time.Minute
	defaultHeartbeatRetryInterval = 5 * time.Minute
	defaultHeartbeatCheckInterval = time.Minute
)

type adScheduler struct {
	resolver   targetResolver
	readers    map[RuntimeType]configReaderFactory
	collectors map[string]ConfigCollector
	sender     collectedConfigSender

	heartbeatInterval time.Duration
	heartbeatJitter   time.Duration
	// startupDelay is selected once and starts with the first valid AD config.
	startupDelay time.Duration
	// startupNotBefore is shared by configs discovered during Agent startup so
	// their first collections stay batchable while Agents spread their sends.
	startupNotBefore time.Time
	// startupTimer releases startup configs at their shared deadline.
	startupTimer *clock.Timer

	heartbeatRetryInterval time.Duration
	heartbeatCheckInterval time.Duration
	clock                  clock.Clock
	jitter                 func(time.Duration) time.Duration

	ctx             context.Context
	cancel          context.CancelFunc
	collectionQueue chan *watchedConfig
	mu              sync.Mutex
	watches         map[string]*watchedConfig
	stopOnce        sync.Once
	workerDone      sync.WaitGroup
}

var _ scheduler.Scheduler = (*adScheduler)(nil)

// watchedConfig holds the durable state needed to recollect one scheduled AD
// config until it is unscheduled. Collectors and reader factories remain in the
// scheduler-owned registries and are looked up only when collection runs.
type watchedConfig struct {
	// key is the AD config digest used for map membership and generation checks.
	key string
	// integration selects the collector and is included in emitted payloads.
	integration string
	// serviceID preserves the original AD service identifier for diagnostics.
	serviceID string
	// target identifies the runtime and entity to inspect on each collection.
	target target
	// nextCollection is the startup, heartbeat, or retry deadline.
	nextCollection time.Time
	// inFlight covers both collection and the subsequent batched send.
	inFlight bool
}

// pendingCollectedConfig holds a collected payload until its batch has been
// sent. The watch pointer lets delivery update the originating watch while
// rejecting results from a watch that has since been replaced.
type pendingCollectedConfig struct {
	watch  *watchedConfig
	config CollectedConfig
}

type collectedConfigSender interface {
	SendCollectedConfigs([]CollectedConfig) error
}

type noopCollectedConfigSender struct{}

func (noopCollectedConfigSender) SendCollectedConfigs([]CollectedConfig) error {
	return nil
}

type collectedConfigBatch struct {
	pendingConfigs []pendingCollectedConfig
	rawConfigBytes int
}

func (b *collectedConfigBatch) hasConfigs() bool {
	return len(b.pendingConfigs) > 0
}

func (b *collectedConfigBatch) add(config pendingCollectedConfig) {
	b.pendingConfigs = append(b.pendingConfigs, config)
	b.rawConfigBytes += collectedConfigRawBytes(config.config)
}

func (b *collectedConfigBatch) shouldFlush() bool {
	return len(b.pendingConfigs) >= configCollectionBatchMaxCollectedConfigs || b.rawConfigBytes >= configCollectionBatchMaxRawConfigBytes
}

func (b *collectedConfigBatch) wouldExceedByteLimit(config pendingCollectedConfig) bool {
	return b.hasConfigs() && b.rawConfigBytes+collectedConfigRawBytes(config.config) > configCollectionBatchMaxRawConfigBytes
}

func (b *collectedConfigBatch) takeConfigs() []pendingCollectedConfig {
	configs := b.pendingConfigs
	b.pendingConfigs = nil
	b.rawConfigBytes = 0
	return configs
}

func collectedConfigRawBytes(config CollectedConfig) int {
	var size int
	for _, file := range config.ConfigFiles {
		size += len(file.Content)
	}
	for _, envVar := range config.EnvVars {
		size += len(envVar.Name) + len(envVar.Value)
	}
	return size
}

type adSchedulerConfig struct {
	heartbeatInterval      time.Duration
	heartbeatJitter        time.Duration
	startupJitter          time.Duration
	heartbeatRetryInterval time.Duration
	heartbeatCheckInterval time.Duration
	clock                  clock.Clock
	jitter                 func(time.Duration) time.Duration
}

func defaultADSchedulerConfig() adSchedulerConfig {
	return adSchedulerConfig{
		heartbeatInterval:      defaultHeartbeatInterval,
		heartbeatJitter:        defaultHeartbeatJitter,
		startupJitter:          defaultStartupJitter,
		heartbeatRetryInterval: defaultHeartbeatRetryInterval,
		heartbeatCheckInterval: defaultHeartbeatCheckInterval,
		clock:                  clock.New(),
		jitter:                 randomJitter,
	}
}

func randomJitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(2*maxJitter)+1)) - maxJitter //nolint:gosec // Jitter is for scheduling spread, not security.
}

func startupDelay(maxDelay time.Duration, jitter func(time.Duration) time.Duration) time.Duration {
	if maxDelay <= 0 {
		return 0
	}
	halfDelay := maxDelay / 2
	return halfDelay + jitter(halfDelay)
}

// newADSchedulerWithConfig builds the long-lived config watcher registered with autodiscovery.
// It collects configs when they are scheduled and retains them for periodic
// heartbeats until autodiscovery unschedules them or the scheduler stops.
func newADSchedulerWithConfig(resolver targetResolver, readers map[RuntimeType]configReaderFactory, collectors map[string]ConfigCollector, sender collectedConfigSender, cfg adSchedulerConfig) *adScheduler {
	if sender == nil {
		sender = noopCollectedConfigSender{}
	}
	cfg = normalizeADSchedulerConfig(cfg)
	initialDelay := startupDelay(cfg.startupJitter, cfg.jitter)

	ctx, cancel := context.WithCancel(context.Background())
	s := &adScheduler{
		resolver:               resolver,
		readers:                readers,
		collectors:             collectors,
		sender:                 sender,
		heartbeatInterval:      cfg.heartbeatInterval,
		heartbeatJitter:        cfg.heartbeatJitter,
		startupDelay:           initialDelay,
		heartbeatRetryInterval: cfg.heartbeatRetryInterval,
		heartbeatCheckInterval: cfg.heartbeatCheckInterval,
		clock:                  cfg.clock,
		jitter:                 cfg.jitter,
		ctx:                    ctx,
		cancel:                 cancel,
		collectionQueue:        make(chan *watchedConfig, configCollectionQueueSize),
		watches:                make(map[string]*watchedConfig),
	}
	s.workerDone.Add(2)
	go s.runCollectionWorker()
	go s.runHeartbeatWorker()
	return s
}

func normalizeADSchedulerConfig(cfg adSchedulerConfig) adSchedulerConfig {
	if cfg.heartbeatInterval <= 0 {
		cfg.heartbeatInterval = defaultHeartbeatInterval
	}
	if cfg.heartbeatJitter < 0 {
		cfg.heartbeatJitter = 0
	}
	if jitterLimit := heartbeatJitterLimit(cfg.heartbeatInterval); cfg.heartbeatJitter > jitterLimit {
		cfg.heartbeatJitter = jitterLimit
	}
	if cfg.startupJitter < 0 {
		cfg.startupJitter = 0
	}
	if cfg.heartbeatRetryInterval <= 0 {
		cfg.heartbeatRetryInterval = defaultHeartbeatRetryInterval
	}
	if cfg.heartbeatCheckInterval <= 0 {
		cfg.heartbeatCheckInterval = defaultHeartbeatCheckInterval
	}
	if cfg.clock == nil {
		cfg.clock = clock.New()
	}
	if cfg.jitter == nil {
		cfg.jitter = randomJitter
	}
	return cfg
}

// Schedule is called by autodiscovery with configs that should run for a
// service. New configs are collected outside the AD callback. Repeated
// schedules update the collection target but wait for the next heartbeat.
func (s *adScheduler) Schedule(configs []integration.Config) {
	for _, config := range configs {
		target, ok := s.resolver.Resolve(config)
		if !ok {
			continue
		}

		if _, found := s.collectors[config.Name]; !found {
			log.Debugf("config files discovery has no collector for integration %q service %q", config.Name, config.ServiceID)
			continue
		}

		if _, found := s.readers[target.runtime]; !found {
			log.Debugf("config files discovery has no config reader for integration %q service %q runtime %q", config.Name, config.ServiceID, target.runtime)
			continue
		}

		s.trackAndEnqueue(config, target)
	}
}

func (s *adScheduler) trackAndEnqueue(config integration.Config, target target) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := watchKey(config)
	watch, ok := s.watches[key]
	if s.startupNotBefore.IsZero() {
		s.startupNotBefore = s.clock.Now().Add(s.startupDelay)
		if s.startupDelay > 0 {
			s.startupTimer = s.clock.AfterFunc(s.startupDelay, s.enqueueDueCollections)
		}
	}
	if !ok {
		watch = &watchedConfig{
			key: key,
		}
		s.watches[key] = watch
	}
	watch.integration = config.Name
	watch.serviceID = config.ServiceID
	watch.target = target
	if !ok && s.clock.Now().Before(s.startupNotBefore) {
		watch.nextCollection = s.startupNotBefore
		return
	}

	if ok && (watch.inFlight || !watch.nextCollection.IsZero()) {
		return
	}
	s.enqueueCollectionLocked(watch)
}

func watchKey(config integration.Config) string {
	return config.Digest()
}

func (s *adScheduler) isActiveWatchLocked(watch *watchedConfig) bool {
	return watch != nil && s.watches[watch.key] == watch
}

func (s *adScheduler) isActiveWatch(watch *watchedConfig) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isActiveWatchLocked(watch)
}

func (s *adScheduler) enqueueCollectionLocked(watch *watchedConfig) {
	if watch.inFlight {
		return
	}

	watch.inFlight = true

	select {
	case <-s.ctx.Done():
		watch.inFlight = false
	case s.collectionQueue <- watch:
	default:
		watch.inFlight = false
		watch.nextCollection = s.clock.Now().Add(s.nextRetryDelay())
		log.Warnf("config files discovery collection queue is full, retrying integration %q service %q runtime %q later", watch.integration, watch.serviceID, watch.target.runtime)
	}
}

func (s *adScheduler) runCollectionWorker() {
	defer s.workerDone.Done()

	var batch collectedConfigBatch
	var flushTimer *time.Timer
	var flushTimerC <-chan time.Time

	stopFlushTimer := func() {
		if flushTimer == nil {
			return
		}
		if !flushTimer.Stop() {
			select {
			case <-flushTimer.C:
			default:
			}
		}
		flushTimer = nil
		flushTimerC = nil
	}

	startFlushTimer := func() {
		if flushTimer != nil {
			return
		}
		flushTimer = time.NewTimer(configCollectionBatchMaxWait)
		flushTimerC = flushTimer.C
	}

	flushBatch := func() bool {
		if !batch.hasConfigs() {
			return true
		}
		stopFlushTimer()
		pendingConfigs := batch.takeConfigs()
		configs := collectedConfigsFromPending(pendingConfigs)
		if err := s.sender.SendCollectedConfigs(configs); err != nil {
			select {
			case <-s.ctx.Done():
				return false
			default:
				log.Warnf("failed to send collected config batch with %d collected configs: %v", len(configs), err)
				s.finishSend(pendingConfigs, false)
				return true
			}
		}
		s.finishSend(pendingConfigs, true)
		return true
	}

	for {
		select {
		case <-s.ctx.Done():
			flushBatch()
			return
		case <-flushTimerC:
			flushTimer = nil
			flushTimerC = nil
			if !flushBatch() {
				return
			}
		case watch := <-s.collectionQueue:
			pendingConfig, ok := s.runCollection(watch)
			if !ok {
				continue
			}
			if batch.wouldExceedByteLimit(pendingConfig) && !flushBatch() {
				return
			}
			batch.add(pendingConfig)
			startFlushTimer()
			if batch.shouldFlush() && !flushBatch() {
				return
			}
		}
	}
}

func collectedConfigsFromPending(pendingConfigs []pendingCollectedConfig) []CollectedConfig {
	configs := make([]CollectedConfig, 0, len(pendingConfigs))
	for _, pendingConfig := range pendingConfigs {
		configs = append(configs, pendingConfig.config)
	}
	return configs
}

func (s *adScheduler) runHeartbeatWorker() {
	defer s.workerDone.Done()

	ticker := s.clock.Ticker(s.heartbeatCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.enqueueDueCollections()
		}
	}
}

func (s *adScheduler) enqueueDueCollections() {
	now := s.clock.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, watch := range s.watches {
		if watch.inFlight || !watch.hasDueCollection(now) {
			continue
		}
		s.enqueueCollectionLocked(watch)
	}
}

func (w *watchedConfig) hasDueCollection(now time.Time) bool {
	return !w.nextCollection.IsZero() && !w.nextCollection.After(now)
}

// runCollection executes one queued config collection. Returns a pending config
// and true when collection produces config data to send.
// Returns an empty config and false when there is nothing to add to the batch.
func (s *adScheduler) runCollection(watch *watchedConfig) (pendingCollectedConfig, bool) {
	integration, serviceID, target, ok := s.snapshotWatch(watch)
	if !ok {
		return pendingCollectedConfig{}, false
	}

	readerFactory := s.readers[target.runtime]
	reader, err := readerFactory(target)
	if err != nil {
		log.Warnf("failed to build config reader for integration %q service %q runtime %q: %v", integration, serviceID, target.runtime, err)
		s.finishCollectionWithError(watch)
		return pendingCollectedConfig{}, false
	}
	defer reader.Close()

	collector := s.collectors[integration]
	collected, err := collector.Collect(s.ctx, reader)
	if err != nil {
		select {
		case <-s.ctx.Done():
			return pendingCollectedConfig{}, false
		default:
			log.Warnf("failed to collect config data for integration %q service %q: %v", integration, serviceID, err)
			s.finishCollectionWithError(watch)
			return pendingCollectedConfig{}, false
		}
	}

	if len(collected.ConfigFiles) == 0 && len(collected.EnvVars) == 0 {
		s.finishCollection(watch, s.clock.Now().Add(s.nextHeartbeatDelay()))
		return pendingCollectedConfig{}, false
	}
	if !s.isActiveWatch(watch) {
		return pendingCollectedConfig{}, false
	}

	collected.Integration = integration
	collected.Runtime = target.runtime
	collected.RuntimeID = target.entityID
	pendingConfig := pendingCollectedConfig{
		watch:  watch,
		config: collected,
	}

	for _, file := range collected.ConfigFiles {
		log.Debugf("config files discovery collected config file: integration %q path %q size_bytes %d truncated %t", pendingConfig.config.Integration, file.Path, len(file.Content), file.Truncated)
	}
	return pendingConfig, true
}

func (s *adScheduler) snapshotWatch(watch *watchedConfig) (string, string, target, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isActiveWatchLocked(watch) {
		return "", "", target{}, false
	}
	return watch.integration, watch.serviceID, watch.target, true
}

func (s *adScheduler) finishCollection(watch *watchedConfig, nextCollection time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isActiveWatchLocked(watch) {
		return
	}
	watch.nextCollection = nextCollection
	watch.inFlight = false
}

func (s *adScheduler) finishCollectionWithError(watch *watchedConfig) {
	s.finishCollection(watch, s.clock.Now().Add(s.nextRetryDelay()))
}

// finishSend assigns every active watch in a batch the same heartbeat or retry
// deadline so subsequent collections can remain batched.
func (s *adScheduler) finishSend(pendingConfigs []pendingCollectedConfig, success bool) {
	now := s.clock.Now()
	var nextDelay time.Duration
	if success {
		nextDelay = s.nextHeartbeatDelay()
	} else {
		nextDelay = s.nextRetryDelay()
	}
	nextCollection := now.Add(nextDelay)

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pendingConfig := range pendingConfigs {
		watch := pendingConfig.watch
		if !s.isActiveWatchLocked(watch) {
			continue
		}

		watch.nextCollection = nextCollection
		watch.inFlight = false
	}
}

func (s *adScheduler) nextHeartbeatDelay() time.Duration {
	return s.heartbeatInterval + s.jitter(s.heartbeatJitter)
}

func heartbeatJitterLimit(interval time.Duration) time.Duration {
	limit := interval / 2
	if limit > maxHeartbeatJitter {
		return maxHeartbeatJitter
	}
	return limit
}

func (s *adScheduler) nextRetryDelay() time.Duration {
	retryJitter := s.heartbeatJitter
	maxRetryJitter := s.heartbeatRetryInterval / 2
	if retryJitter > maxRetryJitter {
		retryJitter = maxRetryJitter
	}
	return s.heartbeatRetryInterval + s.jitter(retryJitter)
}

// Unschedule is called when autodiscovery removes integration configs. Remove
// the corresponding watches so future heartbeat ticks do not re-collect
// configs for services that no longer match.
func (s *adScheduler) Unschedule(configs []integration.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, config := range configs {
		delete(s.watches, watchKey(config))
	}
}

// Stop is required by the autodiscovery scheduler interface. The component
// unregisters this scheduler from autodiscovery during shutdown and cancels any
// in-flight collection.
func (s *adScheduler) Stop() {
	s.stopOnce.Do(func() {
		s.cancel()
		s.mu.Lock()
		if s.startupTimer != nil {
			s.startupTimer.Stop()
		}
		s.mu.Unlock()
		s.workerDone.Wait()
	})
}
