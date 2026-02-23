// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syntheticstestschedulerimpl

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/common"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
)

const (
	syntheticsMetricPrefix = "datadog.synthetics.agent."
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
			s.flush(ctx, flushTime)
		}
	}
}

// flush enqueues tests whose nextRun is due.
func (s *syntheticsTestScheduler) flush(ctx context.Context, flushTime time.Time) {
	if !s.running {
		return
	}

	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	var testsToRun []*runningTestState
	for id, rt := range s.state.tests {
		if flushTime.After(rt.nextRun) || flushTime.Equal(rt.nextRun) {
			s.log.Debugf("test %s is due for execution", id)
			testsToRun = append(testsToRun, rt)
		}
	}
	if len(testsToRun) == 0 {
		return
	}

	threshold := int(float64(cap(s.syntheticsTestProcessingChan)) * 0.7)
	if len(s.syntheticsTestProcessingChan) >= threshold {
		s.log.Warnf("test queue high usage (≥70%%), increase the number of workers")
	}

	maxWait := time.Duration(float64(s.flushInterval) / float64(len(testsToRun)) * 0.9)
	for _, rt := range testsToRun {
		if !s.running {
			return
		}
		s.log.Debugf("enqueuing test %s", rt.cfg.PublicID)
		select {
		case s.syntheticsTestProcessingChan <- SyntheticsTestCtx{
			nextRun: flushTime,
			cfg:     rt.cfg,
		}:
		case <-time.After(maxWait):
			s.log.Warnf("enqueuing test %s timed out, increase the number of workers", rt.cfg.PublicID)
		case <-ctx.Done():
			s.log.Debugf("enqueuing test %s failed because we are stopping the process", rt.cfg.PublicID)
			return
		}
		rt.nextRun = rt.nextRun.Add(time.Duration(rt.cfg.Interval) * time.Second)
	}
}

// runWorker is the main loop for a single worker.
func (s *syntheticsTestScheduler) runWorker(ctx context.Context, workerID int) {
	for {
		// Non-blocking priority check: drain on-demand tests first
		select {
		case testCtx := <-s.onDemandPoller.TestsChan:
			s.executeTest(ctx, workerID, testCtx)
			continue
		default:
		}

		// Blocking wait on all sources
		select {
		case <-ctx.Done():
			s.log.Debugf("worker %d stopping", workerID)
			return
		case testCtx := <-s.onDemandPoller.TestsChan:
			s.executeTest(ctx, workerID, testCtx)
		case syntheticsTestCtx, ok := <-s.syntheticsTestProcessingChan:
			if !ok {
				s.log.Debugf("worker %d stopping: processing channel closed", workerID)
				return
			}
			s.executeTest(ctx, workerID, syntheticsTestCtx)
		}
	}
}

// executeTest runs a single test and sends its result.
func (s *syntheticsTestScheduler) executeTest(ctx context.Context, workerID int, syntheticsTestCtx SyntheticsTestCtx) {
	tracerouteCfg, err := toNetpathConfig(syntheticsTestCtx.cfg)
	if err != nil {
		s.log.Debugf("[worker%d] error interpreting test config: %s", workerID, err)
		s.statsdClient.Incr(syntheticsMetricPrefix+"error_test_config", []string{"reason:error_test_config", fmt.Sprintf("org_id:%d", syntheticsTestCtx.cfg.OrgID), fmt.Sprintf("subtype:%s", syntheticsTestCtx.cfg.Config.Request.GetSubType())}, 1) //nolint:errcheck
		ErrorTestConfig.Inc(string(syntheticsTestCtx.cfg.Config.Request.GetSubType()))
	}

	hname, err := s.hostNameService.Get(ctx)
	if err != nil {
		s.log.Debugf("[worker%d] error getting hostname: %s", workerID, err)
	}

	wResult := &workerResult{
		testCfg:       syntheticsTestCtx,
		triggeredAt:   syntheticsTestCtx.nextRun,
		startedAt:     s.timeNowFn(),
		tracerouteCfg: tracerouteCfg,
		hostname:      hname,
	}

	result, tracerouteErr := s.traceroute.Run(ctx, tracerouteCfg)
	wResult.finishedAt = s.timeNowFn()
	wResult.duration = wResult.finishedAt.Sub(wResult.startedAt)
	if tracerouteErr != nil {
		s.log.Debugf("[worker%d] error running traceroute: %s", workerID, tracerouteErr)
		wResult.tracerouteError = tracerouteErr
		s.statsdClient.Incr(syntheticsMetricPrefix+"traceroute.error", []string{"reason:error_running_datadog_traceroute", fmt.Sprintf("org_id:%d", syntheticsTestCtx.cfg.OrgID), fmt.Sprintf("subtype:%s", syntheticsTestCtx.cfg.Config.Request.GetSubType())}, 1) //nolint:errcheck
		TracerouteError.Inc(string(syntheticsTestCtx.cfg.Config.Request.GetSubType()))
		_, err := s.sendResult(wResult)
		if err != nil {
			s.log.Debugf("[worker%d] error sending result: %s, publicID %s", workerID, err, syntheticsTestCtx.cfg.PublicID)
			s.statsdClient.Incr(syntheticsMetricPrefix+"evp.send_result_failure", []string{"reason:error_sending_result", fmt.Sprintf("org_id:%d", syntheticsTestCtx.cfg.OrgID), fmt.Sprintf("subtype:%s", syntheticsTestCtx.cfg.Config.Request.GetSubType())}, 1) //nolint:errcheck
			SendResultFailure.Inc(string(syntheticsTestCtx.cfg.Config.Request.GetSubType()))
		}
		return
	}
	wResult.tracerouteResult = result
	wResult.assertionResult = runAssertions(syntheticsTestCtx.cfg, common.NetStats{
		PacketsSent:          result.E2eProbe.PacketsSent,
		PacketsReceived:      result.E2eProbe.PacketsReceived,
		PacketLossPercentage: result.E2eProbe.PacketLossPercentage,
		Jitter:               &result.E2eProbe.Jitter,
		Latency:              &result.E2eProbe.RTT,
		Hops:                 result.Traceroute.HopCount,
	})

	status, err := s.sendResult(wResult)
	if err != nil {
		s.log.Debugf("[worker%d] error sending result: %s, publicID %s", workerID, err, syntheticsTestCtx.cfg.PublicID)
		s.statsdClient.Incr(syntheticsMetricPrefix+"evp.send_result_failure", []string{"reason:error_sending_result", fmt.Sprintf("org_id:%d", syntheticsTestCtx.cfg.OrgID), fmt.Sprintf("subtype:%s", syntheticsTestCtx.cfg.Config.Request.GetSubType())}, 1) //nolint:errcheck
		SendResultFailure.Inc(string(syntheticsTestCtx.cfg.Config.Request.GetSubType()))
	}

	s.statsdClient.Incr(syntheticsMetricPrefix+"checks_processed", []string{"status:" + status, fmt.Sprintf("org_id:%d", syntheticsTestCtx.cfg.OrgID), fmt.Sprintf("subtype:%s", syntheticsTestCtx.cfg.Config.Request.GetSubType())}, 1) //nolint:errcheck
	ChecksProcessed.Inc(status, string(syntheticsTestCtx.cfg.Config.Request.GetSubType()))
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
			return config.Config{}, errors.New("invalid UDP request type")
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
			return config.Config{}, errors.New("invalid TCP request type")
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
			return config.Config{}, errors.New("invalid ICMP request type")
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

// sendSyntheticsTestResult marshals the workerResult and forwards it via the epForwarder.
func (s *syntheticsTestScheduler) sendSyntheticsTestResult(w *workerResult) (string, error) {
	res, err := s.networkPathToTestResult(w)
	if err != nil {
		return "", err
	}

	payloadBytes, err := json.Marshal(res)
	if err != nil {
		return "", err
	}

	s.log.Debugf("synthetics network path test event: %s", string(payloadBytes))

	m := message.NewMessage(payloadBytes, nil, "", 0)
	if err := s.epForwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeSynthetics); err != nil {
		return "", err
	}
	return res.Result.Status, nil
}

// configRequestToResultRequest converts a ConfigRequest to a ResultRequest.
func configRequestToResultRequest(req common.ConfigRequest) (common.ResultRequest, error) {
	switch r := req.(type) {
	case common.UDPConfigRequest:
		return common.ResultRequest{
			Host:              r.Host,
			Port:              r.Port,
			SourceService:     r.SourceService,
			MaxTTL:            r.MaxTTL,
			Timeout:           r.Timeout,
			TracerouteQueries: r.TracerouteCount,
			E2eQueries:        r.ProbeCount,
		}, nil
	case common.TCPConfigRequest:
		return common.ResultRequest{
			Host:               r.Host,
			Port:               r.Port,
			DestinationService: r.DestinationService,
			SourceService:      r.SourceService,
			MaxTTL:             r.MaxTTL,
			Timeout:            r.Timeout,
			TracerouteQueries:  r.TracerouteCount,
			E2eQueries:         r.ProbeCount,
			TCPMethod:          r.TCPMethod,
		}, nil
	case common.ICMPConfigRequest:
		return common.ResultRequest{
			Host:               r.Host,
			DestinationService: r.DestinationService,
			SourceService:      r.SourceService,
			MaxTTL:             r.MaxTTL,
			Timeout:            r.Timeout,
			TracerouteQueries:  r.TracerouteCount,
			E2eQueries:         r.ProbeCount,
		}, nil
	default:
		return common.ResultRequest{}, fmt.Errorf("unknown config request: %q", r)
	}
}

// networkPathToTestResult converts a workerResult into the public TestResult structure.
func (s *syntheticsTestScheduler) networkPathToTestResult(w *workerResult) (*common.TestResult, error) {
	t := common.Test{
		ID:      w.testCfg.cfg.PublicID,
		SubType: strings.ToLower(string(w.testCfg.cfg.Config.Request.GetSubType())),
		Type:    w.testCfg.cfg.Type,
		Version: w.testCfg.cfg.Version,
	}

	// on-demand tests have a result ID generated on the backend
	testResultID := ""
	if w.testCfg.cfg.ResultID != "" {
		testResultID = w.testCfg.cfg.ResultID
	} else {
		resultID, err := s.generateTestResultID(rand.Int)
		if err != nil {
			return nil, err
		}
		testResultID = resultID
	}

	w.tracerouteResult.Source.Name = w.hostname
	w.tracerouteResult.Source.DisplayName = w.hostname
	w.tracerouteResult.Source.Hostname = w.hostname
	w.tracerouteResult.TestConfigID = w.testCfg.cfg.PublicID
	w.tracerouteResult.TestResultID = testResultID
	w.tracerouteResult.Origin = payload.PathOriginSynthetics
	w.tracerouteResult.TestRunType = payload.TestRunTypeScheduled
	w.tracerouteResult.SourceProduct = payload.SourceProductSynthetics
	w.tracerouteResult.CollectorType = payload.CollectorTypeAgent
	w.tracerouteResult.Timestamp = w.finishedAt.UnixMilli()

	cfgRequest, err := configRequestToResultRequest(w.testCfg.cfg.Config.Request)
	if err != nil {
		return nil, err
	}
	result := common.Result{
		ID:              testResultID,
		InitialID:       testResultID,
		TestFinishedAt:  w.finishedAt.UnixMilli(),
		TestStartedAt:   w.startedAt.UnixMilli(),
		TestTriggeredAt: w.triggeredAt.UnixMilli(),
		Duration:        w.duration.Milliseconds(),
		Assertions:      w.assertionResult,
		Config: common.Config{
			Assertions: w.testCfg.cfg.Config.Assertions,
			Request:    cfgRequest,
		},
		Netstats: common.NetStats{
			PacketsSent:          w.tracerouteResult.E2eProbe.PacketsSent,
			PacketsReceived:      w.tracerouteResult.E2eProbe.PacketsReceived,
			PacketLossPercentage: w.tracerouteResult.E2eProbe.PacketLossPercentage,
			Jitter:               &w.tracerouteResult.E2eProbe.Jitter,
			Latency:              &w.tracerouteResult.E2eProbe.RTT,
			Hops:                 w.tracerouteResult.Traceroute.HopCount,
		},
		Netpath: w.tracerouteResult,
		Status:  "passed",
		RunType: w.testCfg.cfg.RunType,
	}

	if w.tracerouteResult.E2eProbe.RTT.Max == 0 || w.tracerouteResult.E2eProbe.PacketsReceived == 0 {
		result.Netstats.Latency = nil
		result.Netstats.Jitter = nil
	}

	if w.tracerouteResult.E2eProbe.PacketsReceived == 1 {
		result.Netstats.Jitter = nil
	}

	s.setResultStatus(w, &result)

	return &common.TestResult{
		Location: struct {
			ID string `json:"id"`
		}{ID: "agent:" + w.hostname},
		DD:     make(map[string]interface{}),
		Result: result,
		Test:   t,
		V:      1,
	}, nil
}

func (s *syntheticsTestScheduler) setResultStatus(w *workerResult, result *common.Result) {
	if result.Netstats.PacketLossPercentage == 1 {
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
		if assertion.Type == common.AssertionTypePacketLoss && assertion.Operator == common.OperatorIs && assertion.Expected == "1" {
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
