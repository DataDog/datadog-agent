// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syntheticstestschedulerimpl

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/common"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
)

// runWorkers starts the configured number of worker goroutines and waits for them.
func (s *SyntheticsTestScheduler) runWorkers() {
	s.log.Debugf("starting workers (%d)", s.workers)

	var wg sync.WaitGroup
	for w := 0; w < s.workers; w++ {
		wg.Add(1)
		s.log.Debugf("starting worker #%d", w)
		go func() {
			defer wg.Done()
			s.runWorker(w)
		}()
	}
	wg.Wait()
	s.workersDone <- struct{}{}
}

// flushLoop periodically enqueues tests that are due.
func (s *SyntheticsTestScheduler) flushLoop() {
	s.log.Debugf("starting flush loop")

	for {
		select {
		case <-s.stopChan:
			s.log.Info("stopped flush loop")
			s.flushLoopDone <- struct{}{}
			return
		case flushTime := <-s.tickerC:
			s.flush(flushTime)
		}
	}
}

// flush enqueues tests whose nextRun is due.
func (s *SyntheticsTestScheduler) flush(flushTime time.Time) {
	for id, rt := range s.state.tests {
		if flushTime.After(rt.nextRun) || flushTime.Equal(rt.nextRun) {
			s.log.Debugf("enqueuing test %s", id)
			s.syntheticsTestProcessingChan <- SyntheticsTestCtx{
				nextRun: flushTime,
				cfg:     rt.cfg,
			}
			if err := s.updateTestState(rt); err != nil {
				s.log.Errorf("unable to save test state %s", err)
			}
		}
	}
}

// runWorker is the main loop for a single worker.
func (s *SyntheticsTestScheduler) runWorker(workerID int) {
	for {
		select {
		case <-s.stopChan:
			s.log.Debugf("[worker%d] stopped worker", workerID)
			return
		case syntheticsTestCtx := <-s.syntheticsTestProcessingChan:
			triggeredAt := syntheticsTestCtx.nextRun
			startedAt := s.TimeNowFn()

			tracerouteCfg, err := toNetpathConfig(syntheticsTestCtx.cfg)
			if err != nil {
				s.log.Debugf("[worker%d] error interpreting test config: %s", workerID, err)
			}

			result, err := s.runTraceroute(tracerouteCfg, s.telemetry)
			if err != nil {
				s.log.Debugf("[worker%d] Error running traceroute: %s", workerID, err)
			}

			finishedAt := s.TimeNowFn()
			duration := finishedAt.Sub(startedAt)

			if err = s.sendResult(&WorkerResult{
				tracerouteResult: result,
				tracerouteError:  err,
				testCfg:          syntheticsTestCtx,
				triggeredAt:      triggeredAt,
				startedAt:        startedAt,
				finishedAt:       finishedAt,
				duration:         duration,
				tracerouteCfg:    tracerouteCfg,
			}); err != nil {
				s.log.Debugf("[worker%d] error sending result: %s", workerID, err)
			}
		}
	}
}

// WorkerResult represents the result produced by a worker.
type WorkerResult struct {
	tracerouteResult payload.NetworkPath
	tracerouteError  error
	testCfg          SyntheticsTestCtx

	triggeredAt   time.Time
	startedAt     time.Time
	finishedAt    time.Time
	duration      time.Duration
	tracerouteCfg config.Config
}

// toNetpathConfig converts a SyntheticsTestConfig into a system-probe Config.
func toNetpathConfig(c common.SyntheticsTestConfig) (config.Config, error) {
	var cfg config.Config

	switch common.SubType(c.Subtype) {
	case common.SubTypeUDP:
		req, ok := c.Config.Request.(common.UDPConfigRequest)
		if !ok {
			return config.Config{}, fmt.Errorf("invalid UDP request type")
		}
		cfg.Protocol = payload.ProtocolUDP
		cfg.DestHostname = req.Host
		if req.Port != nil {
			cfg.DestPort = uint16(*req.Port)
		}
		if req.MaxTTL != nil {
			cfg.MaxTTL = uint8(*req.MaxTTL)
		}
		if req.Timeout != nil {
			cfg.Timeout = time.Duration(*req.Timeout) * time.Second
		}
		if req.SourceService != nil {
			cfg.SourceService = *req.SourceService
		}
		if req.DestinationService != nil {
			cfg.DestinationService = *req.DestinationService
		}

	case common.SubTypeTCP:
		req, ok := c.Config.Request.(common.TCPConfigRequest)
		if !ok {
			return config.Config{}, fmt.Errorf("invalid TCP request type")
		}
		cfg.Protocol = payload.ProtocolTCP
		cfg.DestHostname = req.Host
		if req.Port != nil {
			cfg.DestPort = uint16(*req.Port)
		}
		if req.MaxTTL != nil {
			cfg.MaxTTL = uint8(*req.MaxTTL)
		}
		if req.Timeout != nil {
			cfg.Timeout = time.Duration(*req.Timeout) * time.Second
		}
		cfg.TCPMethod = payload.TCPMethod(req.TCPMethod)
		if req.SourceService != nil {
			cfg.SourceService = *req.SourceService
		}
		if req.DestinationService != nil {
			cfg.DestinationService = *req.DestinationService
		}

	case common.SubTypeICMP:
		req, ok := c.Config.Request.(common.ICMPConfigRequest)
		if !ok {
			return config.Config{}, fmt.Errorf("invalid ICMP request type")
		}
		cfg.Protocol = payload.ProtocolICMP
		cfg.DestHostname = req.Host
		if req.MaxTTL != nil {
			cfg.MaxTTL = uint8(*req.MaxTTL)
		}
		if req.Timeout != nil {
			cfg.Timeout = time.Duration(*req.Timeout) * time.Second
		}
		if req.SourceService != nil {
			cfg.SourceService = *req.SourceService
		}
		if req.DestinationService != nil {
			cfg.DestinationService = *req.DestinationService
		}

	default:
		return config.Config{}, fmt.Errorf("unsupported subtype: %s", c.Subtype)
	}

	return cfg, nil
}

// SyntheticsTestCtx is the unit of work consumed by workers.
type SyntheticsTestCtx struct {
	nextRun time.Time
	cfg     common.SyntheticsTestConfig
}

// updateTestState updates lastRun and nextRun for a running test.
func (s *SyntheticsTestScheduler) updateTestState(rt *runningTestState) error {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	rt.lastRun = rt.nextRun
	rt.nextRun = rt.nextRun.Add(time.Duration(rt.cfg.Interval) * time.Second)
	return s.persistState()
}

// sendSyntheticsTestResult marshals the WorkerResult and forwards it via the epForwarder.
func (s *SyntheticsTestScheduler) sendSyntheticsTestResult(w *WorkerResult) error {
	res, err := s.networkPathToTestResult(w)
	if err != nil {
		return err
	}
	payloadBytes, err := json.Marshal(res)
	if err != nil {
		return err
	}
	s.log.Debugf("synthetics network path test event: %s", string(payloadBytes))
	m := message.NewMessage(payloadBytes, nil, "", 0)
	return s.epForwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeSynthetics)
}

// runTraceroute is the default traceroute execution using the traceroute package.
func runTraceroute(cfg config.Config, telemetry telemetry.Component) (payload.NetworkPath, error) {
	tr, err := traceroute.New(cfg, telemetry)
	if err != nil {
		return payload.NetworkPath{}, fmt.Errorf("new traceroute error: %s", err)
	}
	path, err := tr.Run(context.TODO())
	if err != nil {
		return payload.NetworkPath{}, fmt.Errorf("run traceroute error: %s", err)
	}
	return path, nil
}

// networkPathToTestResult converts a WorkerResult into the public TestResult structure.
func (s *SyntheticsTestScheduler) networkPathToTestResult(w *WorkerResult) (*common.TestResult, error) {
	t := common.Test{
		InternalID: w.testCfg.cfg.PublicID,
		ID:         w.testCfg.cfg.PublicID,
		SubType:    w.testCfg.cfg.Subtype,
		Type:       w.testCfg.cfg.Type,
		Version:    w.testCfg.cfg.Version,
	}

	testResultID, err := s.generateTestResultID()
	if err != nil {
		return nil, err
	}

	np := w.tracerouteResult
	hops := make([]common.NetpathHop, 0, len(np.Hops))
	for _, h := range np.Hops {
		hops = append(hops, common.NetpathHop{
			TTL:       h.TTL,
			RTT:       h.RTT,
			IPAddress: h.IPAddress,
			Hostname:  h.Hostname,
			Reachable: h.Reachable,
		})
	}

	netpath := common.NetpathResult{
		Timestamp:    np.Timestamp,
		PathtraceID:  np.PathtraceID,
		Origin:       string(np.Origin),
		Protocol:     string(np.Protocol),
		AgentVersion: np.AgentVersion,
		Namespace:    np.Namespace,
		Source: common.NetpathSource{
			Hostname: np.Source.Hostname,
		},
		Destination: common.NetpathDestination{
			Hostname:           np.Destination.Hostname,
			IPAddress:          np.Destination.IPAddress,
			Port:               int(np.Destination.Port),
			ReverseDNSHostname: np.Destination.ReverseDNSHostname,
		},
		Hops:         hops,
		TestConfigID: w.testCfg.cfg.PublicID,
		TestResultID: testResultID,
		Traceroute:   common.TracerouteTest{},
		E2E:          common.E2ETest{},
		Tags:         np.Tags,
	}

	result := common.Result{
		ID:              testResultID,
		InitialID:       testResultID,
		TestFinishedAt:  w.finishedAt.UnixMilli(),
		TestStartedAt:   w.startedAt.UnixMilli(),
		TestTriggeredAt: w.triggeredAt.UnixMilli(),
		Assertions:      nil,
		Duration:        w.duration.Milliseconds(),
		Request: common.Request{
			Host:    w.tracerouteCfg.DestHostname,
			Port:    int(w.tracerouteCfg.DestPort),
			MaxTTL:  int(w.tracerouteCfg.MaxTTL),
			Timeout: int(w.tracerouteCfg.Timeout.Milliseconds()),
		},
		Netstats: common.NetStats{},
		Netpath:  netpath,
		Status:   "passed",
	}

	if w.tracerouteError != nil {
		result.Status = "failed"
		result.Failure = common.APIError{
			Code:    "UNKNOWN",
			Message: w.tracerouteError.Error(),
		}
	}

	return &common.TestResult{
		DD:     make(map[string]interface{}),
		Result: result,
		Test:   t,
		V:      1,
	}, nil
}

// generateRandomStringUInt63 returns a cryptographically random uint63 as decimal string.
func generateRandomStringUInt63() (string, error) {
	maxi := new(big.Int).Lsh(big.NewInt(1), 63) // 2^63
	n, err := rand.Int(rand.Reader, maxi)       // 0 <= n < 2^63
	if err != nil {
		return "", err
	}
	return n.String(), nil
}
