// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package npschedulerimpl implements the scheduler for network path
package npschedulerimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/network/bogon"
	"github.com/DataDog/datadog-agent/pkg/networkpath/metricsender"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/telemetry"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"go.uber.org/atomic"
)

type npSchedulerImpl struct {
	epForwarder eventplatform.Component
	logger      log.Component

	enabled bool

	workers int

	excludeIPManager *bogon.Bogon
	metricSender     metricsender.MetricSender

	receivedPathtestConfigCount *atomic.Uint64
	pathtestStore               *pathtestStore
	pathtestInputChan           chan *pathtest
	pathtestProcessChan         chan *pathtestContext
	stopChan                    chan struct{}
	flushLoopDone               chan struct{}
	runDone                     chan struct{}

	TimeNowFunction func() time.Time // Allows to mock time in tests
}

func newNoopNpSchedulerImpl() *npSchedulerImpl {
	return &npSchedulerImpl{enabled: false}
}

func newNpSchedulerImpl(epForwarder eventplatform.Component, logger log.Component, sysprobeYamlConfig config.Reader) *npSchedulerImpl {
	workers := sysprobeYamlConfig.GetInt("network_path.workers")
	pathtestInputChanSize := sysprobeYamlConfig.GetInt("network_path.input_chan_size")
	pathtestProcessChanSize := sysprobeYamlConfig.GetInt("network_path.process_chan_size")
	pathtestTTL := sysprobeYamlConfig.GetDuration("network_path.pathtest_ttl")
	pathtestInterval := sysprobeYamlConfig.GetDuration("network_path.pathtest_interval")

	logger.Infof("New NpScheduler (workers=%d input_chan_size=%d pathtest_ttl=%s pathtest_interval=%s)",
		workers,
		pathtestInputChanSize,
		pathtestTTL.String(),
		pathtestInterval.String())

	return &npSchedulerImpl{
		enabled:     true,
		epForwarder: epForwarder,
		logger:      logger,

		pathtestStore:       newPathtestStore(DefaultFlushTickerInterval, pathtestTTL, pathtestInterval, logger),
		pathtestInputChan:   make(chan *pathtest, pathtestInputChanSize),
		pathtestProcessChan: make(chan *pathtestContext, pathtestProcessChanSize),
		workers:             workers,

		metricSender: metricsender.NewMetricSenderStatsd(),

		receivedPathtestConfigCount: atomic.NewUint64(0),
		TimeNowFunction:             time.Now,

		stopChan:      make(chan struct{}),
		runDone:       make(chan struct{}),
		flushLoopDone: make(chan struct{}),
	}
}

func (s *npSchedulerImpl) Enabled() bool {
	return s.enabled
}

// Schedule schedules pathtests.
// It shouldn't block, if the input channel is full, an error is returned.
func (s *npSchedulerImpl) Schedule(hostname string, port uint16) error {
	if s.pathtestInputChan == nil {
		return errors.New("no input channel, please check that network path is enabled")
	}
	s.logger.Debugf("Schedule traceroute for: hostname=%s port=%d", hostname, port)

	if net.ParseIP(hostname).To4() == nil {
		// TODO: IPv6 not supported yet
		s.logger.Debugf("Only IPv4 is currently supported. Address not supported: %s", hostname)
		return nil
	}

	if s.excludeIPManager != nil {
		isExcluded, _ := s.excludeIPManager.Is(hostname)
		if isExcluded {
			s.logger.Debugf("Excluded IP hostname=%s", hostname)
			return nil
		}
	}

	ptest := &pathtest{
		hostname: hostname,
		port:     port,
	}
	select {
	case s.pathtestInputChan <- ptest:
		return nil
	default:
		return fmt.Errorf("scheduler input channel is full (channel capacity is %d)", cap(s.pathtestInputChan))
	}
}
func (s *npSchedulerImpl) start() {
	s.logger.Info("Start NpScheduler")
	go s.listenPathtestConfigs()
	go s.flushLoop()
	s.startWorkers()
}

func (s *npSchedulerImpl) stop() {
	s.logger.Infof("Stop NpScheduler")
	close(s.stopChan)
	<-s.flushLoopDone
	<-s.runDone
}

func (s *npSchedulerImpl) listenPathtestConfigs() {
	for {
		select {
		case <-s.stopChan:
			// TODO: TESTME
			s.logger.Info("Stop listening to traceroute commands")
			s.runDone <- struct{}{}
			return
		case ptest := <-s.pathtestInputChan:
			// TODO: TESTME
			s.logger.Debugf("Pathtest received: %+v", ptest)
			s.receivedPathtestConfigCount.Inc()
			s.pathtestStore.add(ptest)
		}
	}
}

func (s *npSchedulerImpl) runTraceroute(ptest *pathtestContext) {
	s.logger.Debugf("Run Traceroute for ptest: %+v", ptest)
	s.pathForConn(ptest)
}

func (s *npSchedulerImpl) pathForConn(ptest *pathtestContext) {
	startTime := time.Now()
	cfg := traceroute.Config{
		DestHostname: ptest.pathtest.hostname,
		DestPort:     uint16(ptest.pathtest.port),
		MaxTTL:       24,
		TimeoutMs:    1000,
	}

	tr, err := traceroute.New(cfg)
	if err != nil {
		s.logger.Warnf("traceroute error: %+v", err)
		return
	}
	path, err := tr.Run(context.TODO())
	if err != nil {
		s.logger.Warnf("traceroute error: %+v", err)
		return
	}
	s.logger.Debugf("Network Path: %+v", path)

	s.sendTelemetry(path, startTime, ptest)

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
	// TODO: Remove workers metric?
	statsd.Client.Gauge("datadog.network_path.scheduler.workers", float64(s.workers), []string{}, 1) //nolint:errcheck

	flowsContexts := s.pathtestStore.getPathtestContextCount()
	statsd.Client.Gauge("datadog.network_path.scheduler.pathtest_store_size", float64(flowsContexts), []string{}, 1) //nolint:errcheck
	flushTime := s.TimeNowFunction()
	flowsToFlush := s.pathtestStore.flush()
	statsd.Client.Gauge("datadog.network_path.scheduler.pathtest_flushed_count", float64(len(flowsToFlush)), []string{}, 1) //nolint:errcheck
	s.logger.Debugf("Flushing %d flows to the forwarder (flush_duration=%d, flow_contexts_before_flush=%d)", len(flowsToFlush), time.Since(flushTime).Milliseconds(), flowsContexts)

	for _, ptConf := range flowsToFlush {
		s.logger.Tracef("flushed ptConf %s:%d", ptConf.pathtest.hostname, ptConf.pathtest.port)
		// TODO: FLUSH TO CHANNEL + WORKERS EXECUTE
		s.pathtestProcessChan <- ptConf
	}
}

func (s *npSchedulerImpl) sendTelemetry(path payload.NetworkPath, startTime time.Time, ptest *pathtestContext) {
	checkInterval := ptest.lastFlushInterval
	checkDuration := time.Since(startTime)
	telemetry.SubmitNetworkPathTelemetry(
		s.metricSender,
		path,
		telemetry.CollectorTypeNetworkPathScheduler,
		checkDuration,
		checkInterval,
		[]string{},
	)
}

func (s *npSchedulerImpl) startWorkers() {
	// TODO: TESTME
	for w := 0; w < s.workers; w++ {
		go s.startWorker(w)
	}
}

func (s *npSchedulerImpl) startWorker(workerID int) {
	// TODO: TESTME
	s.logger.Debugf("Starting worker #%d", workerID)
	for {
		select {
		case <-s.stopChan:
			s.logger.Debugf("[worker%d] Stopping worker", workerID)
			return
		case pathtestCtx := <-s.pathtestProcessChan:
			s.logger.Debugf("[worker%d] Handling pathtest hostname=%s, port=%d", workerID, pathtestCtx.pathtest.hostname, pathtestCtx.pathtest.port)
			s.runTraceroute(pathtestCtx)
		}
	}
}
