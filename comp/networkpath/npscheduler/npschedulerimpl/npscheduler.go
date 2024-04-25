// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package npschedulerimpl implements the scheduler for network path
package npschedulerimpl

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/utils"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"go.uber.org/atomic"
)

type npSchedulerImpl struct {
	epForwarder eventplatform.Component
	logger      log.Component

	initOnce sync.Once

	workers int

	receivedPathtestConfigCount *atomic.Uint64
	pathtestStore               *pathtestStore
	pathtestIn                  chan *pathtest
	stopChan                    chan struct{}
	flushLoopDone               chan struct{}
	runDone                     chan struct{}

	TimeNowFunction func() time.Time // Allows to mock time in tests
}

func newNpSchedulerImpl(epForwarder eventplatform.Component, logger log.Component) *npSchedulerImpl {
	return &npSchedulerImpl{
		epForwarder: epForwarder,
		logger:      logger,

		pathtestStore: newPathtestStore(DefaultFlushTickerInterval, DefaultPathtestRunDurationFromDiscovery, DefaultPathtestRunInterval, logger),
		pathtestIn:    make(chan *pathtest),
		workers:       3, // TODO: Make it a configurable

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
		case ptest := <-s.pathtestIn:
			// TODO: TESTME
			s.logger.Debugf("Pathtest received: %+v", ptest)
			s.receivedPathtestConfigCount.Inc()
			s.pathtestStore.add(ptest)
		}
	}
}
func (s *npSchedulerImpl) Schedule(hostname string, port uint16) {
	s.logger.Debugf("Schedule traceroute for: hostname=%s port=%d", hostname, port)

	if net.ParseIP(hostname).To4() == nil {
		// TODO: IPv6 not supported yet
		s.logger.Debugf("Only IPv4 is currently supported. Address not supported: %s", hostname)
		return
	}
	s.pathtestIn <- &pathtest{
		hostname: hostname,
		port:     port,
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

	tr := traceroute.New(cfg)
	path, err := tr.Run()
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

func (s *npSchedulerImpl) Stop() {
	s.logger.Infof("Stop npSchedulerImpl")
	close(s.stopChan)
	<-s.flushLoopDone
	<-s.runDone
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
	flowsContexts := s.pathtestStore.getPathtestContextCount()
	flushTime := s.TimeNowFunction()
	flowsToFlush := s.pathtestStore.flush()
	s.logger.Debugf("Flushing %d flows to the forwarder (flush_duration=%d, flow_contexts_before_flush=%d)", len(flowsToFlush), time.Since(flushTime).Milliseconds(), flowsContexts)

	for _, ptConf := range flowsToFlush {
		s.logger.Tracef("flushed ptConf %s:%d", ptConf.pathtest.hostname, ptConf.pathtest.port)
		s.runTraceroute(ptConf)
	}
}

func (s *npSchedulerImpl) sendTelemetry(path traceroute.NetworkPath, startTime time.Time, ptest *pathtestContext) {
	// TODO: Factor Network Path telemetry from Network Path Integration and use the code
	// TODO: Factor Network Path telemetry from Network Path Integration and use the code
	// TODO: Factor Network Path telemetry from Network Path Integration and use the code
	// TODO: Factor Network Path telemetry from Network Path Integration and use the code
	tags := s.getTelemetryTags(path)

	checkDuration := time.Since(startTime)
	statsd.Client.Gauge("datadog.network_path.check_duration", checkDuration.Seconds(), tags, 1) //nolint:errcheck

	if ptest.lastFlushInterval > 0 {
		statsd.Client.Gauge("datadog.network_path.check_interval", ptest.lastFlushInterval.Seconds(), tags, 1) //nolint:errcheck
	}

	statsd.Client.Gauge("datadog.network_path.path.monitored", float64(1), tags, 1) //nolint:errcheck
	if len(path.Hops) > 0 {
		lastHop := path.Hops[len(path.Hops)-1]
		if lastHop.Success {
			statsd.Client.Gauge("datadog.network_path.path.hops", float64(len(path.Hops)), tags, 1) //nolint:errcheck
		}
		statsd.Client.Gauge("datadog.network_path.path.reachable", float64(utils.BoolToFloat64(lastHop.Success)), tags, 1)    //nolint:errcheck
		statsd.Client.Gauge("datadog.network_path.path.unreachable", float64(utils.BoolToFloat64(!lastHop.Success)), tags, 1) //nolint:errcheck
	}
}

func (s *npSchedulerImpl) getTelemetryTags(path traceroute.NetworkPath) []string {
	var tags []string
	agentHost, err := hostname.Get(context.TODO())
	if err != nil {
		s.logger.Warnf("Error getting the hostname: %v", err)
	} else {
		tags = append(tags, "agent_host:"+agentHost)
	}
	tags = append(tags, utils.GetAgentVersionTag())

	destPortTag := "unspecified"
	if path.Destination.Port > 0 {
		destPortTag = strconv.Itoa(int(path.Destination.Port))
	}
	tags = append(tags, []string{
		"protocol:udp", // TODO: Update to protocol from config when we support tcp/icmp
		"destination_hostname:" + path.Destination.Hostname,
		"destination_port:" + destPortTag,
	}...)
	return tags
}
