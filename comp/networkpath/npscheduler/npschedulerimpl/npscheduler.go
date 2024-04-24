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
	"sync"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/networkpath/npscheduler"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In
	Lc          fx.Lifecycle
	EpForwarder eventplatform.Component
}

type provides struct {
	fx.Out

	Comp npscheduler.Component
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newNpScheduler),
	)
}

func newNpScheduler(deps dependencies) provides {
	scheduler := newNpSchedulerImpl(deps.EpForwarder)
	deps.Lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			scheduler.Stop()
			return nil
		},
	})
	return provides{
		Comp: scheduler,
	}
}

type npSchedulerImpl struct {
	epForwarder eventplatform.Component

	initOnce sync.Once

	workers int

	pathtestConfigIn chan pathtestConfig
	stopChan         chan struct{}
	flushLoopDone    chan struct{}
	runDone          chan struct{}
}

func (s *npSchedulerImpl) Init() {
	s.initOnce.Do(func() {
		// TODO: conn checks -> job chan
		//       job chan -> update state
		//       flush
		//         - read state
		//         - traceroute using workers
		go s.listenPathtestConfigs()
		//go s.flushLoop()
	})
}
func (s *npSchedulerImpl) listenPathtestConfigs() {
	for {
		select {
		case <-s.stopChan:
			// TODO: TESTME
			log.Info("Stop listening to traceroute commands")
			s.runDone <- struct{}{}
			return
		case command := <-s.pathtestConfigIn:
			log.Infof("Command received: %+v", command)
			//s.receivedFlowCount.Inc()
			//s.traceroute.add(flow)
		}
	}
}
func (s *npSchedulerImpl) Schedule(hostname string, port uint16) {
	// TODO: Use logger component?
	log.Debugf("Schedule traceroute for: hostname=%s port=%d", hostname, port)
	statsd.Client.Incr("datadog.network_path.scheduler.count", []string{}, 1) //nolint:errcheck

	if net.ParseIP(hostname).To4() == nil {
		// TODO: IPv6 not supported yet
		log.Debugf("Only IPv4 is currently supported. Address not supported: %s", hostname)
		return
	}
	s.pathtestConfigIn <- pathtestConfig{
		hostname: hostname,
		port:     port,
	}
}

func (s *npSchedulerImpl) runTraceroute(job pathtestConfig) {
	log.Debugf("Run Traceroute for job: %+v", job)
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
		log.Warnf("traceroute error: %+v", err)
		return
	}
	log.Debugf("Network Path: %+v", path)

	epForwarder, ok := s.epForwarder.Get()
	if ok {
		payloadBytes, err := json.Marshal(path)
		if err != nil {
			log.Errorf("json marshall error: %s", err)
		} else {

			log.Debugf("network path event: %s", string(payloadBytes))
			m := message.NewMessage(payloadBytes, nil, "", 0)
			err = epForwarder.SendEventPlatformEvent(m, eventplatform.EventTypeNetworkPath)
			if err != nil {
				log.Errorf("SendEventPlatformEvent error: %s", err)
			}
		}
	}
}

func (s *npSchedulerImpl) Stop() {
	log.Error("Stop npSchedulerImpl")
}

func newNpSchedulerImpl(epForwarder eventplatform.Component) *npSchedulerImpl {
	return &npSchedulerImpl{
		epForwarder: epForwarder,

		pathtestConfigIn: make(chan pathtestConfig),
		workers:          3, // TODO: Make it a configurable
	}
}
