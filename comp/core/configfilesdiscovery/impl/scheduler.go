// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	schedulerName             = "configfiles-discovery"
	configCollectionQueueSize = 128
)

type adScheduler struct {
	resolver targetResolver

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
	collector     configCollector
	readerFactory configReaderFactory
}

// newADScheduler builds the object registered with autodiscovery.
// Autodiscovery calls this scheduler when integration configs appear or
// disappear; this component only uses the scheduled configs as triggers for
// one-shot config collection.
func newADScheduler(resolver targetResolver) *adScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	s := &adScheduler{
		resolver:        resolver,
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

		collector, ok := configCollectors[config.Name]
		if !ok {
			log.Debugf("config files discovery has no collector for integration %q service %q", config.Name, config.ServiceID)
			continue
		}

		readerFactory, ok := configReaders[target.runtime]
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

	for {
		select {
		case <-s.ctx.Done():
			return
		case work := <-s.collectionQueue:
			s.runCollection(work)
		}
	}
}

func (s *adScheduler) runCollection(work configCollectionWork) {
	reader, err := work.readerFactory(work.target)
	if err != nil {
		log.Warnf("failed to build config reader for integration %q service %q runtime %q: %v", work.config.Name, work.config.ServiceID, work.target.runtime, err)
		return
	}

	if err := work.collector.Run(s.ctx, reader); err != nil {
		select {
		case <-s.ctx.Done():
			return
		default:
			log.Warnf("failed to run config files discovery for integration %q service %q: %v", work.config.Name, work.config.ServiceID, err)
			return
		}
	}
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
