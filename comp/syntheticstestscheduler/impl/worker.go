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
	"io"
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
func (s *syntheticsTestScheduler) runWorkers(ctx context.Context) {
	s.log.Debugf("starting workers (%d)", s.workers)

	var wg sync.WaitGroup
	for w := 0; w < s.workers; w++ {
		wg.Add(1)
		s.log.Debugf("starting worker #%d", w)
		go func() {
			defer wg.Done()
			s.runWorker(ctx, w)
		}()
	}
	wg.Wait()
	s.workersDone <- struct{}{}
}

// flushLoop periodically enqueues tests that are due.
func (s *syntheticsTestScheduler) flushLoop(ctx context.Context) {
	s.log.Debugf("starting flush loop")
	defer close(s.flushLoopDone)

	for {
		select {
		case <-ctx.Done():
			s.log.Info("stopped flush loop")
			return
		case flushTime := <-s.tickerC:
			s.flush(flushTime)
		}
	}
}

// flush enqueues tests whose nextRun is due.
func (s *syntheticsTestScheduler) flush(flushTime time.Time) {
	for id, rt := range s.state.tests {
		if flushTime.After(rt.nextRun) || flushTime.Equal(rt.nextRun) {
			s.log.Debugf("enqueuing test %s", id)
			s.syntheticsTestProcessingChan <- SyntheticsTestCtx{
				nextRun: flushTime,
				cfg:     rt.cfg,
			}
			s.updateTestState(rt)
		}
	}
}

// runWorker is the main loop for a single worker.
func (s *syntheticsTestScheduler) runWorker(ctx context.Context, workerID int) {
	for {
		select {
		case <-ctx.Done():
			s.log.Debugf("worker %d stopping", workerID)
			return
		case syntheticsTestCtx := <-s.syntheticsTestProcessingChan:
			tracerouteCfg, err := toNetpathConfig(syntheticsTestCtx.cfg)
			if err != nil {
				s.log.Debugf("[worker%d] error interpreting test config: %s", workerID, err)
			}

			hname, err := s.hostNameService.Get(ctx)
			if err != nil {
				s.log.Debugf("[worker%d] Error running traceroute: %s", workerID, err)
			}

			wResult := &workerResult{
				testCfg:       syntheticsTestCtx,
				triggeredAt:   syntheticsTestCtx.nextRun,
				startedAt:     s.timeNowFn(),
				tracerouteCfg: tracerouteCfg,
				hostname:      hname,
			}

			result, tracerouteErr := s.runTraceroute(ctx, tracerouteCfg, s.telemetry)
			wResult.finishedAt = s.timeNowFn()
			wResult.duration = wResult.finishedAt.Sub(wResult.startedAt)
			if tracerouteErr != nil {
				s.log.Debugf("[worker%d] Error running traceroute: %s", workerID, err)
				wResult.tracerouteError = tracerouteErr
				if err = s.sendResult(wResult); err != nil {
					s.log.Debugf("[worker%d] error sending result: %s", workerID, err)
				}
				continue
			}
			wResult.tracerouteResult = result
			wResult.assertionResult = runAssertions(syntheticsTestCtx.cfg, common.NetStats{
				PacketsSent:          result.E2eProbe.PacketsSent,
				PacketsReceived:      result.E2eProbe.PacketsReceived,
				PacketLossPercentage: result.E2eProbe.PacketLossPercentage,
				Jitter:               result.E2eProbe.Jitter,
				Latency:              result.E2eProbe.RTT,
				Hops:                 result.Traceroute.HopCount,
			})
			if err = s.sendResult(wResult); err != nil {
				s.log.Debugf("[worker%d] error sending result: %s", workerID, err)
			}
		}
	}
}

// workerResult represents the result produced by a worker.
type workerResult struct {
	tracerouteResult payload.NetworkPath
	tracerouteError  error
	assertionResult  []common.AssertionResult
	testCfg          SyntheticsTestCtx
	triggeredAt      time.Time
	startedAt        time.Time
	finishedAt       time.Time
	duration         time.Duration
	tracerouteCfg    config.Config
	hostname         string
}

// fillNetworkConfig fills the common fields from NetworkConfigRequest into Config.
func fillNetworkConfig(cfg *config.Config, ncr common.NetworkConfigRequest) {
	if ncr.SourceService != nil {
		cfg.SourceService = *ncr.SourceService
	}
	if ncr.DestinationService != nil {
		cfg.DestinationService = *ncr.DestinationService
	}
	if ncr.MaxTTL != nil {
		cfg.MaxTTL = uint8(*ncr.MaxTTL)
	}
	if ncr.Timeout != nil {
		cfg.Timeout = time.Duration(float64(*ncr.Timeout) * 0.9 / float64(cfg.MaxTTL) * float64(time.Second))
	}
	if ncr.TracerouteCount != nil {
		cfg.TracerouteQueries = *ncr.TracerouteCount
	}
	if ncr.ProbeCount != nil {
		cfg.E2eQueries = *ncr.ProbeCount
	}
	cfg.ReverseDNS = true
}

// toNetpathConfig converts a SyntheticsTestConfig into a system-probe Config.
func toNetpathConfig(c common.SyntheticsTestConfig) (config.Config, error) {
	var cfg config.Config

	switch t := c.Config.Request.(type) {
	case common.UDPConfigRequest:
		req, ok := c.Config.Request.(common.UDPConfigRequest)
		if !ok {
			return config.Config{}, fmt.Errorf("invalid UDP request type")
		}
		cfg.Protocol = payload.ProtocolUDP
		cfg.DestHostname = req.Host
		if req.Port != nil {
			cfg.DestPort = uint16(*req.Port)
		}
		fillNetworkConfig(&cfg, req.NetworkConfigRequest)

	case common.TCPConfigRequest:
		req, ok := c.Config.Request.(common.TCPConfigRequest)
		if !ok {
			return config.Config{}, fmt.Errorf("invalid TCP request type")
		}
		cfg.Protocol = payload.ProtocolTCP
		cfg.DestHostname = req.Host
		if req.Port != nil {
			cfg.DestPort = uint16(*req.Port)
		}
		cfg.TCPMethod = payload.TCPMethod(req.TCPMethod)
		fillNetworkConfig(&cfg, req.NetworkConfigRequest)
	case common.ICMPConfigRequest:
		req, ok := c.Config.Request.(common.ICMPConfigRequest)
		if !ok {
			return config.Config{}, fmt.Errorf("invalid ICMP request type")
		}
		cfg.Protocol = payload.ProtocolICMP
		cfg.DestHostname = req.Host
		fillNetworkConfig(&cfg, req.NetworkConfigRequest)

	default:
		return config.Config{}, fmt.Errorf("unsupported subtype: %s", t)
	}

	return cfg, nil
}

// SyntheticsTestCtx is the unit of work consumed by workers.
type SyntheticsTestCtx struct {
	nextRun time.Time
	cfg     common.SyntheticsTestConfig
}

// updateTestState updates lastRun and nextRun for a running test.
func (s *syntheticsTestScheduler) updateTestState(rt *runningTestState) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	rt.lastRun = rt.nextRun
	rt.nextRun = rt.nextRun.Add(time.Duration(rt.cfg.Interval) * time.Second)
}

// sendSyntheticsTestResult marshals the workerResult and forwards it via the epForwarder.
func (s *syntheticsTestScheduler) sendSyntheticsTestResult(w *workerResult) error {
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
func runTraceroute(ctx context.Context, cfg config.Config, telemetry telemetry.Component) (payload.NetworkPath, error) {
	tr, err := traceroute.New(cfg, telemetry)
	if err != nil {
		return payload.NetworkPath{}, fmt.Errorf("new traceroute error: %s", err)
	}
	path, err := tr.Run(ctx)
	if err != nil {
		return payload.NetworkPath{}, fmt.Errorf("run traceroute error: %s", err)
	}
	return path, nil
}

// networkPathToTestResult converts a workerResult into the public TestResult structure.
func (s *syntheticsTestScheduler) networkPathToTestResult(w *workerResult) (*common.TestResult, error) {
	t := common.Test{
		ID:      w.testCfg.cfg.PublicID,
		SubType: string(w.testCfg.cfg.Config.Request.GetSubType()),
		Type:    w.testCfg.cfg.Type,
		Version: w.testCfg.cfg.Version,
	}

	testResultID, err := s.generateTestResultID(rand.Int)
	if err != nil {
		return nil, err
	}

	w.tracerouteResult.Source.Name = w.hostname
	w.tracerouteResult.Source.DisplayName = w.hostname
	w.tracerouteResult.Source.Hostname = w.hostname
	w.tracerouteResult.TestConfigID = w.testCfg.cfg.PublicID
	w.tracerouteResult.TestResultID = testResultID
	w.tracerouteResult.Origin = "synthetics"
	w.tracerouteResult.Timestamp = w.finishedAt.UnixMilli()

	result := common.Result{
		ID:              testResultID,
		InitialID:       testResultID,
		TestFinishedAt:  w.finishedAt.UnixMilli(),
		TestStartedAt:   w.startedAt.UnixMilli(),
		TestTriggeredAt: w.triggeredAt.UnixMilli(),
		Duration:        w.duration.Milliseconds(),
		Assertions:      w.assertionResult,
		Request: common.Request{
			Host:    w.tracerouteCfg.DestHostname,
			Port:    int(w.tracerouteCfg.DestPort),
			MaxTTL:  int(w.tracerouteCfg.MaxTTL),
			Timeout: int(w.tracerouteCfg.Timeout.Milliseconds()),
		},
		Netstats: common.NetStats{
			PacketsSent:          w.tracerouteResult.E2eProbe.PacketsSent,
			PacketsReceived:      w.tracerouteResult.E2eProbe.PacketsReceived,
			PacketLossPercentage: w.tracerouteResult.E2eProbe.PacketLossPercentage,
			Jitter:               w.tracerouteResult.E2eProbe.Jitter,
			Latency:              w.tracerouteResult.E2eProbe.RTT,
			Hops:                 w.tracerouteResult.Traceroute.HopCount,
		},
		Netpath: w.tracerouteResult,
		Status:  "passed",
		RunType: w.testCfg.cfg.RunType,
	}

	s.setResultStatus(w, &result)

	return &common.TestResult{
		Location: struct {
			ID string `json:"id"`
		}{ID: fmt.Sprintf("agent:%s", w.hostname)},
		DD:     make(map[string]interface{}),
		Result: result,
		Test:   t,
		V:      1,
	}, nil
}

func (s *syntheticsTestScheduler) setResultStatus(w *workerResult, result *common.Result) {
	if result.Netstats.PacketLossPercentage == 100 {
		if !hasAssertionOn100PacketLoss(w.assertionResult) {
			result.Status = "failed"
			result.Failure = common.APIError{
				Code:    "NETUNREACH",
				Message: "The remote server network is unreachable.",
			}
		}
	}
	if w.tracerouteError != nil {
		result.Status = "failed"
		result.Failure = common.APIError{
			Code:    "UNKNOWN",
			Message: w.tracerouteError.Error(),
		}
	}
	if result.Status != "failed" {
		for _, res := range w.assertionResult {
			if !res.Valid {
				result.Status = "failed"
				assertionResultJSON, err := json.Marshal(w.assertionResult)
				message := "Assertions failed"
				if err == nil {
					message = string(assertionResultJSON)
				}

				result.Failure = common.APIError{
					Code:    incorrectAssertion,
					Message: message,
				}
			}
		}
	}
}

func hasAssertionOn100PacketLoss(assertionResults []common.AssertionResult) bool {
	for _, assertion := range assertionResults {
		if assertion.Type == common.AssertionTypePacketLoss && assertion.Operator == common.OperatorIs && assertion.Expected == "100" {
			return true
		}
	}
	return false
}

// generateRandomStringUInt63 returns a cryptographically random uint63 as decimal string.
func generateRandomStringUInt63(randIntFn func(rand io.Reader, max *big.Int) (n *big.Int, err error)) (string, error) {
	maxi := new(big.Int).Lsh(big.NewInt(1), 63) // 2^63
	n, err := randIntFn(rand.Reader, maxi)      // 0 <= n < 2^63
	if err != nil {
		return "", err
	}
	return n.String(), nil
}
