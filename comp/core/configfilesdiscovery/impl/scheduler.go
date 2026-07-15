// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	schedulerName                            = "configfiles-discovery"
	configCollectionQueueSize                = 128
	configCollectionBatchMaxWait             = time.Second
	configCollectionBatchMaxCollectedConfigs = 100
	configCollectionBatchMaxRawConfigBytes   = 4 * 1024 * 1024 // 4MiB
)

type adScheduler struct {
	resolver   targetResolver
	readers    map[RuntimeType]configReaderFactory
	collectors map[string]ConfigCollector
	sender     collectedConfigSender

	ctx             context.Context
	cancel          context.CancelFunc
	collectionQueue chan configCollectionWork
	stopOnce        sync.Once
	workerDone      sync.WaitGroup
}

var _ scheduler.Scheduler = (*adScheduler)(nil)

type configCollectionWork struct {
	config        integration.Config
	target        target
	collector     ConfigCollector
	readerFactory configReaderFactory
}

type collectedConfig struct {
	Integration string
	Runtime     RuntimeType
	RuntimeID   string
	ConfigFiles []ConfigFile
	EnvVars     []ConfigEnvVar
}

type collectedConfigSender interface {
	SendCollectedConfigs([]collectedConfig) error
}

type noopCollectedConfigSender struct{}

func (noopCollectedConfigSender) SendCollectedConfigs([]collectedConfig) error {
	return nil
}

type collectedConfigBatch struct {
	configs        []collectedConfig
	rawConfigBytes int
}

func (b *collectedConfigBatch) hasConfigs() bool {
	return len(b.configs) > 0
}

func (b *collectedConfigBatch) add(config collectedConfig) {
	b.configs = append(b.configs, config)
	b.rawConfigBytes += collectedConfigRawBytes(config)
}

func (b *collectedConfigBatch) shouldFlush() bool {
	return len(b.configs) >= configCollectionBatchMaxCollectedConfigs || b.rawConfigBytes >= configCollectionBatchMaxRawConfigBytes
}

func (b *collectedConfigBatch) wouldExceedByteLimit(config collectedConfig) bool {
	return b.hasConfigs() && b.rawConfigBytes+collectedConfigRawBytes(config) > configCollectionBatchMaxRawConfigBytes
}

func (b *collectedConfigBatch) takeConfigs() []collectedConfig {
	configs := make([]collectedConfig, len(b.configs))
	copy(configs, b.configs)
	b.configs = nil
	b.rawConfigBytes = 0
	return configs
}

func collectedConfigRawBytes(config collectedConfig) int {
	var size int
	for _, file := range config.ConfigFiles {
		size += len(file.Content)
	}
	return size
}

// newADScheduler builds the object registered with autodiscovery.
// Autodiscovery calls this scheduler when integration configs appear or
// disappear; this component only uses the scheduled configs as triggers for
// one-shot config collection.
func newADScheduler(resolver targetResolver, readers map[RuntimeType]configReaderFactory, collectors map[string]ConfigCollector, sender collectedConfigSender) *adScheduler {
	if sender == nil {
		sender = noopCollectedConfigSender{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &adScheduler{
		resolver:        resolver,
		readers:         readers,
		collectors:      collectors,
		sender:          sender,
		ctx:             ctx,
		cancel:          cancel,
		collectionQueue: make(chan configCollectionWork, configCollectionQueueSize),
	}
	s.workerDone.Add(1)
	go s.runCollectionWorker()
	return s
}

// Schedule is called by autodiscovery with configs that should run for a
// service. For each config, it normalizes the AD service information into an
// internal target, selects the collector and runtime-specific reader factory,
// and enqueues one-shot collection work outside the AD scheduler callback.
func (s *adScheduler) Schedule(configs []integration.Config) {
	for _, config := range configs {
		target, ok := s.resolver.Resolve(config)
		if !ok {
			continue
		}

		collector, ok := s.collectors[config.Name]
		if !ok {
			log.Debugf("config files discovery has no collector for integration %q service %q", config.Name, config.ServiceID)
			continue
		}

		readerFactory, ok := s.readers[target.runtime]
		if !ok {
			log.Debugf("config files discovery has no config reader for integration %q service %q runtime %q", config.Name, config.ServiceID, target.runtime)
			continue
		}

		work := configCollectionWork{
			config:        config,
			target:        target,
			collector:     collector,
			readerFactory: readerFactory,
		}

		select {
		case <-s.ctx.Done():
			return
		case s.collectionQueue <- work:
		default:
			log.Warnf("config files discovery collection queue is full, dropping integration %q service %q runtime %q", config.Name, config.ServiceID, target.runtime)
		}
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
		configs := batch.takeConfigs()
		if err := s.sender.SendCollectedConfigs(configs); err != nil {
			select {
			case <-s.ctx.Done():
				return false
			default:
				log.Warnf("failed to send collected config files batch with %d collected configs: %v", len(configs), err)
			}
		}
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
		case work := <-s.collectionQueue:
			config, ok := s.runCollection(work)
			if !ok {
				continue
			}
			if batch.wouldExceedByteLimit(config) && !flushBatch() {
				return
			}
			batch.add(config)
			startFlushTimer()
			if batch.shouldFlush() && !flushBatch() {
				return
			}
		}
	}
}

// runCollection executes one queued config collection. Returns a collected
// config and true when collection succeeds and produces at least one config
// file. Returns an empty collected config and false when there is nothing to add
// to the batch.
func (s *adScheduler) runCollection(work configCollectionWork) (collectedConfig, bool) {
	reader, err := work.readerFactory(work.target)
	if err != nil {
		log.Warnf("failed to build config reader for integration %q service %q runtime %q: %v", work.config.Name, work.config.ServiceID, work.target.runtime, err)
		return collectedConfig{}, false
	}
	defer reader.Close()

	files, err := work.collector.Collect(s.ctx, reader)
	if err != nil {
		select {
		case <-s.ctx.Done():
			return collectedConfig{}, false
		default:
			log.Warnf("failed to collect config files for integration %q service %q: %v", work.config.Name, work.config.ServiceID, err)
			return collectedConfig{}, false
		}
	}

	if len(files) == 0 {
		return collectedConfig{}, false
	}

	config := collectedConfig{
		Integration: work.config.Name,
		Runtime:     work.target.runtime,
		RuntimeID:   work.target.entityID,
		ConfigFiles: files,
	}

	for _, file := range files {
		log.Debugf("config files discovery collected config file: integration %q path %q size_bytes %d truncated %t", work.config.Name, file.Path, len(file.Content), file.Truncated)
	}
	return config, true
}

// Unschedule is required by the autodiscovery scheduler interface. Config file
// discovery does not keep a long-running collection tied to a scheduled AD
// config, so there is nothing to tear down when AD unschedules it.
func (s *adScheduler) Unschedule(_ []integration.Config) {}

// Stop is required by the autodiscovery scheduler interface. The component
// unregisters this scheduler from autodiscovery during shutdown and cancels any
// in-flight collection.
func (s *adScheduler) Stop() {
	s.stopOnce.Do(func() {
		s.cancel()
		s.workerDone.Wait()
	})
}
