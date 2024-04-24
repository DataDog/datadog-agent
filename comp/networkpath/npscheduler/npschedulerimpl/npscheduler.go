// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package npschedulerimpl implements the scheduler for network path
package npschedulerimpl

import (
	"encoding/json"
	"net"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"go.uber.org/atomic"
)

type npSchedulerImpl struct {
	epForwarder eventplatform.Component
	logger      log.Component

	initOnce sync.Once

	workers int

	receivedPathtestConfigCount *atomic.Uint64
	pathtestConfigState         *flowAccumulator
	pathtestConfigIn            chan *pathtestConfig
	stopChan                    chan struct{}
	flushLoopDone               chan struct{}
	runDone                     chan struct{}

	TimeNowFunction func() time.Time // Allows to mock time in tests
}

func newNpSchedulerImpl(epForwarder eventplatform.Component, logger log.Component) *npSchedulerImpl {
	return &npSchedulerImpl{
		epForwarder: epForwarder,
		logger:      logger,

		pathtestConfigState: newFlowAccumulator(10*time.Second, 60*time.Second, logger),
		pathtestConfigIn:    make(chan *pathtestConfig),
		workers:             3, // TODO: Make it a configurable

		receivedPathtestConfigCount: atomic.NewUint64(0),
		TimeNowFunction:             time.Now,
	}
}

func (s *npSchedulerImpl) Init() {
	s.initOnce.Do(func() {
		// TODO: conn checks -> job chan
		//       job chan -> update state
		//       flush
		//         - read state
		//         - traceroute using workers
		s.logger.Info("Init NpScheduler")
		go s.listenPathtestConfigs()
		go s.flushLoop()
	})
}
func (s *npSchedulerImpl) listenPathtestConfigs() {
	for {
		select {
		case <-s.stopChan:
			// TODO: TESTME
			s.logger.Info("Stop listening to traceroute commands")
			s.runDone <- struct{}{}
			return
		case pathtestConf := <-s.pathtestConfigIn:
			// TODO: TESTME
			s.logger.Infof("Command received: %+v", pathtestConf)
			s.receivedPathtestConfigCount.Inc()
			s.pathtestConfigState.add(pathtestConf)
		}
	}
}
func (s *npSchedulerImpl) Schedule(hostname string, port uint16) {
	s.logger.Debugf("Schedule traceroute for: hostname=%s port=%d", hostname, port)
	statsd.Client.Incr("datadog.network_path.scheduler.count", []string{}, 1) //nolint:errcheck

	if net.ParseIP(hostname).To4() == nil {
		// TODO: IPv6 not supported yet
		s.logger.Debugf("Only IPv4 is currently supported. Address not supported: %s", hostname)
		return
	}
	s.pathtestConfigIn <- &pathtestConfig{
		hostname: hostname,
		port:     port,
	}
}

func (s *npSchedulerImpl) runTraceroute(job pathtestConfig) {
	s.logger.Debugf("Run Traceroute for job: %+v", job)
	// TODO: RUN 3x? Configurable?
	for i := 0; i < 3; i++ {
		s.pathForConn(job.hostname, job.port)
	}
}

func (s *npSchedulerImpl) pathForConn(hostname string, port uint16) {
	cfg := traceroute.Config{
		DestHostname: hostname,
		DestPort:     uint16(port),
		MaxTTL:       24,
		TimeoutMs:    1000,
	}

	tr := traceroute.New(cfg)
	path, err := tr.Run()
	if err != nil {
		s.logger.Warnf("traceroute error: %+v", err)
		return
	}
	s.logger.Debugf("Network Path: %+v", path)

	epForwarder, ok := s.epForwarder.Get()
	if ok {
		payloadBytes, err := json.Marshal(path)
		if err != nil {
			s.logger.Errorf("json marshall error: %s", err)
		} else {

			s.logger.Debugf("network path event: %s", string(payloadBytes))
			m := message.NewMessage(payloadBytes, nil, "", 0)
			err = epForwarder.SendEventPlatformEvent(m, eventplatform.EventTypeNetworkPath)
			if err != nil {
				s.logger.Errorf("SendEventPlatformEvent error: %s", err)
			}
		}
	}
}

func (s *npSchedulerImpl) Stop() {
	s.logger.Error("Stop npSchedulerImpl")
}

func (s *npSchedulerImpl) flushLoop() {
	flushTicker := time.NewTicker(10 * time.Second)

	var lastFlushTime time.Time
	for {
		select {
		// stop sequence
		case <-s.stopChan:
			s.flushLoopDone <- struct{}{}
			flushTicker.Stop()
			return
		// automatic flush sequence
		case <-flushTicker.C:
			now := time.Now()
			if !lastFlushTime.IsZero() {
				flushInterval := now.Sub(lastFlushTime)
				statsd.Client.Gauge("datadog.network_path.scheduler.flush_interval", flushInterval.Seconds(), []string{}, 1) //nolint:errcheck
			}
			lastFlushTime = now

			flushStartTime := time.Now()
			s.flush()
			statsd.Client.Gauge("datadog.network_path.scheduler.flush_duration", time.Since(flushStartTime).Seconds(), []string{}, 1) //nolint:errcheck
		}
	}
}

func (s *npSchedulerImpl) flush() {
	flowsContexts := s.pathtestConfigState.getFlowContextCount()
	flushTime := s.TimeNowFunction()
	flowsToFlush := s.pathtestConfigState.flush()
	s.logger.Debugf("Flushing %d flows to the forwarder (flush_duration=%d, flow_contexts_before_flush=%d)", len(flowsToFlush), time.Since(flushTime).Milliseconds(), flowsContexts)

	// TODO: run traceroute here for flowsToFlush
	for _, ptConf := range flowsToFlush {
		s.logger.Tracef("flushed ptConf %s:%d", ptConf.hostname, ptConf.port)
	}
}
