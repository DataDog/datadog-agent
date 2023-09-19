// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"fmt"
	"hash/fnv"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/comp/process/forwarders"
	"github.com/DataDog/datadog-agent/comp/process/types"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/runner/endpoint"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/status"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type Submitter interface {
	Submit(start time.Time, name string, messages *types.Payload)
	Start() error
	Stop()
}

var _ Submitter = &CheckSubmitter{}

type CheckSubmitter struct {
	log log.Component
	// Per-check Weighted Queues
	processResults     *api.WeightedQueue
	rtProcessResults   *api.WeightedQueue
	eventResults       *api.WeightedQueue
	connectionsResults *api.WeightedQueue
	podResults         *api.WeightedQueue

	// Forwarders
	processForwarder     defaultforwarder.Component
	rtProcessForwarder   defaultforwarder.Component
	connectionsForwarder defaultforwarder.Component
	podForwarder         *forwarder.DefaultForwarder
	eventForwarder       defaultforwarder.Component

	orchestrator *oconfig.OrchestratorConfig
	hostname     string

	exit chan struct{}
	wg   *sync.WaitGroup

	// Used to cache the hash result of the host name and the pid of the process agent. Being used as part of
	// getRequestID method. Must use pointer, to distinguish between uninitialized value and the theoretical but yet
	// possible 0 value for the hash result.
	requestIDCachedHash *uint64
	dropCheckPayloads   []string

	forwarderRetryMaxQueueBytes int

	// Channel for notifying the submitter to enable/disable realtime mode
	rtNotifierChan chan types.RTResponse

	agentStartTime int64
}

func NewSubmitter(config config.Component, log log.Component, forwarders forwarders.Component, hostname string) (*CheckSubmitter, error) {
	queueBytes := config.GetInt("process_config.process_queue_bytes")
	if queueBytes <= 0 {
		log.Warnf("Invalid queue bytes size: %d. Using default value: %d", queueBytes, ddconfig.DefaultProcessQueueBytes)
		queueBytes = ddconfig.DefaultProcessQueueBytes
	}

	queueSize := config.GetInt("process_config.queue_size")
	if queueSize <= 0 {
		log.Warnf("Invalid check queue size: %d. Using default value: %d", queueSize, ddconfig.DefaultProcessQueueSize)
		queueSize = ddconfig.DefaultProcessQueueSize
	}
	processResults := api.NewWeightedQueue(queueSize, int64(queueBytes))
	log.Debugf("Creating process check queue with max_size=%d and max_weight=%d", processResults.MaxSize(), processResults.MaxWeight())

	rtQueueSize := config.GetInt("process_config.rt_queue_size")
	if rtQueueSize <= 0 {
		log.Warnf("Invalid rt check queue size: %d. Using default value: %d", rtQueueSize, ddconfig.DefaultProcessRTQueueSize)
		rtQueueSize = ddconfig.DefaultProcessRTQueueSize
	}
	// reuse main queue's ProcessQueueBytes because it's unlikely that it'll reach to that size in bytes, so we don't need a separate config for it
	rtProcessResults := api.NewWeightedQueue(rtQueueSize, int64(queueBytes))
	log.Debugf("Creating rt process check queue with max_size=%d and max_weight=%d", rtProcessResults.MaxSize(), rtProcessResults.MaxWeight())

	connectionsResults := api.NewWeightedQueue(queueSize, int64(queueBytes))
	log.Debugf("Creating connections queue with max_size=%d and max_weight=%d", connectionsResults.MaxSize(), connectionsResults.MaxWeight())

	orchestrator := oconfig.NewDefaultOrchestratorConfig()
	if err := orchestrator.Load(); err != nil {
		return nil, err
	}
	podResults := api.NewWeightedQueue(queueSize, int64(orchestrator.PodQueueBytes))
	log.Debugf("Creating pod check queue with max_size=%d and max_weight=%d", podResults.MaxSize(), podResults.MaxWeight())

	eventResults := api.NewWeightedQueue(queueSize, int64(queueBytes))
	log.Debugf("Creating event check queue with max_size=%d and max_weight=%d", eventResults.MaxSize(), eventResults.MaxWeight())

	dropCheckPayloads := config.GetStringSlice("process_config.drop_check_payloads")
	if len(dropCheckPayloads) > 0 {
		log.Debugf("Dropping payloads from checks: %v", dropCheckPayloads)
	}
	status.UpdateDropCheckPayloads(dropCheckPayloads)

	// Forwarder initialization
	processAPIEndpoints, err := endpoint.GetAPIEndpoints(config)
	if err != nil {
		return nil, err
	}

	podForwarderOpts := forwarder.NewOptionsWithResolvers(config, log, resolver.NewSingleDomainResolvers(apicfg.KeysPerDomains(orchestrator.OrchestratorEndpoints)))
	podForwarderOpts.DisableAPIKeyChecking = true
	podForwarderOpts.RetryQueuePayloadsTotalMaxSize = queueBytes // Allow more in-flight requests than the default
	podForwarder := forwarder.NewDefaultForwarder(config, log, podForwarderOpts)

	processEventsAPIEndpoints, err := endpoint.GetEventsAPIEndpoints(config)
	if err != nil {
		return nil, err
	}

	printStartMessage(log, hostname, processAPIEndpoints, processEventsAPIEndpoints, orchestrator.OrchestratorEndpoints)
	return &CheckSubmitter{
		log:                log,
		processResults:     processResults,
		rtProcessResults:   rtProcessResults,
		eventResults:       eventResults,
		connectionsResults: connectionsResults,
		podResults:         podResults,

		processForwarder:     forwarders.GetProcessForwarder(),
		rtProcessForwarder:   forwarders.GetRTProcessForwarder(),
		connectionsForwarder: forwarders.GetConnectionsForwarder(),
		podForwarder:         podForwarder,
		eventForwarder:       forwarders.GetEventForwarder(),

		orchestrator: orchestrator,
		hostname:     hostname,

		dropCheckPayloads: dropCheckPayloads,

		forwarderRetryMaxQueueBytes: queueBytes,

		rtNotifierChan: make(chan types.RTResponse, 1), // Buffer the channel so we don't block submissions

		wg:   &sync.WaitGroup{},
		exit: make(chan struct{}),

		agentStartTime: time.Now().Unix(),
	}, nil
}

func printStartMessage(log log.Component, hostname string, processAPIEndpoints, processEventsAPIEndpoints, orchestratorEndpoints []apicfg.Endpoint) {
	eps := make([]string, 0, len(processAPIEndpoints))
	for _, e := range processAPIEndpoints {
		eps = append(eps, e.Endpoint.String())
	}
	orchestratorEps := make([]string, 0, len(orchestratorEndpoints))
	for _, e := range orchestratorEndpoints {
		orchestratorEps = append(orchestratorEps, e.Endpoint.String())
	}
	eventsEps := make([]string, 0, len(processEventsAPIEndpoints))
	for _, e := range processEventsAPIEndpoints {
		eventsEps = append(eventsEps, e.Endpoint.String())
	}

	log.Infof("Starting CheckSubmitter for host=%s, endpoints=%s, events endpoints=%s orchestrator endpoints=%s", hostname, eps, eventsEps, orchestratorEps)
}

func (s *CheckSubmitter) Submit(start time.Time, name string, messages *types.Payload) {
	results := s.resultsQueueForCheck(name)
	if name == checks.PodCheckName {
		s.messagesToResultsQueue(start, checks.PodCheckName, messages.Message[:len(messages.Message)/2], results)
		if s.orchestrator.IsManifestCollectionEnabled {
			s.messagesToResultsQueue(start, checks.PodCheckManifestName, messages.Message[len(messages.Message)/2:], results)
		}
		return
	}

	s.messagesToResultsQueue(start, name, messages.Message, results)
}

func (s *CheckSubmitter) Start() error {
	if err := s.processForwarder.Start(); err != nil {
		return fmt.Errorf("error starting forwarder: %s", err)
	}

	if err := s.rtProcessForwarder.Start(); err != nil {
		return fmt.Errorf("error starting RT forwarder: %s", err)
	}

	if err := s.connectionsForwarder.Start(); err != nil {
		return fmt.Errorf("error starting connections forwarder: %s", err)
	}

	if err := s.podForwarder.Start(); err != nil {
		return fmt.Errorf("error starting pod forwarder: %s", err)
	}

	if err := s.eventForwarder.Start(); err != nil {
		return fmt.Errorf("error starting event forwarder: %s", err)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.consumePayloads(s.processResults, s.processForwarder)
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.consumePayloads(s.rtProcessResults, s.rtProcessForwarder)
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.consumePayloads(s.connectionsResults, s.connectionsForwarder)
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.consumePayloads(s.podResults, s.podForwarder)
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.consumePayloads(s.eventResults, s.eventForwarder)
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()

		queueSizeTicker := time.NewTicker(10 * time.Second)
		defer queueSizeTicker.Stop()

		queueLogTicker := time.NewTicker(time.Minute)
		defer queueLogTicker.Stop()

		agentVersion, _ := version.Agent()
		tags := []string{
			fmt.Sprintf("version:%s", agentVersion.GetNumberAndPre()),
			fmt.Sprintf("revision:%s", agentVersion.Commit),
		}
		for {
			select {
			case <-heartbeat.C:
				statsd.Client.Gauge("datadog.process.agent", 1, tags, 1) //nolint:errcheck
			case <-queueSizeTicker.C:
				status.UpdateQueueStats(&status.QueueStats{
					ProcessQueueSize:      s.processResults.Len(),
					RtProcessQueueSize:    s.rtProcessResults.Len(),
					ConnectionsQueueSize:  s.connectionsResults.Len(),
					EventQueueSize:        s.eventResults.Len(),
					PodQueueSize:          s.podResults.Len(),
					ProcessQueueBytes:     s.processResults.Weight(),
					RtProcessQueueBytes:   s.rtProcessResults.Weight(),
					ConnectionsQueueBytes: s.connectionsResults.Weight(),
					EventQueueBytes:       s.eventResults.Weight(),
					PodQueueBytes:         s.podResults.Weight(),
				})
			case <-queueLogTicker.C:
				s.logQueuesSize()
			case <-s.exit:
				return
			}
		}
	}()

	return nil
}

func (s *CheckSubmitter) Stop() {
	close(s.exit)

	s.processResults.Stop()
	s.rtProcessResults.Stop()
	s.connectionsResults.Stop()
	s.podResults.Stop()
	s.eventResults.Stop()

	s.wg.Wait()

	s.processForwarder.Stop()
	s.rtProcessForwarder.Stop()
	s.connectionsForwarder.Stop()
	s.podForwarder.Stop()
	s.eventForwarder.Stop()

	close(s.rtNotifierChan)
}

func (s *CheckSubmitter) GetRTNotifierChan() <-chan types.RTResponse {
	return s.rtNotifierChan
}

func (s *CheckSubmitter) consumePayloads(results *api.WeightedQueue, fwd forwarder.Forwarder) {
	for {
		// results.Poll() will return ok=false when stopped
		item, ok := results.Poll()
		if !ok {
			return
		}
		result := item.(*checkResult)
		for _, payload := range result.payloads {
			var (
				forwarderPayload = transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&payload.body})
				responses        chan forwarder.Response
				err              error
				updateRTStatus   bool
			)

			if s.shouldDropPayload(result.name) {
				continue
			}

			switch result.name {
			case checks.ProcessCheckName:
				updateRTStatus = true
				responses, err = fwd.SubmitProcessChecks(forwarderPayload, payload.headers)
			case checks.RTProcessCheckName:
				updateRTStatus = true
				responses, err = fwd.SubmitRTProcessChecks(forwarderPayload, payload.headers)
			case checks.ContainerCheckName:
				updateRTStatus = true
				responses, err = fwd.SubmitContainerChecks(forwarderPayload, payload.headers)
			case checks.RTContainerCheckName:
				updateRTStatus = true
				responses, err = fwd.SubmitRTContainerChecks(forwarderPayload, payload.headers)
			case checks.ConnectionsCheckName:
				responses, err = fwd.SubmitConnectionChecks(forwarderPayload, payload.headers)
			// Pod check metadata
			case checks.PodCheckName:
				responses, err = fwd.SubmitOrchestratorChecks(forwarderPayload, payload.headers, int(orchestrator.K8sPod))
			// Pod check manifest data
			case checks.PodCheckManifestName:
				responses, err = fwd.SubmitOrchestratorManifests(forwarderPayload, payload.headers)
			case checks.DiscoveryCheckName:
				// A Process Discovery check does not change the RT mode
				responses, err = fwd.SubmitProcessDiscoveryChecks(forwarderPayload, payload.headers)
			case checks.ProcessEventsCheckName:
				responses, err = fwd.SubmitProcessEventChecks(forwarderPayload, payload.headers)
			default:
				err = fmt.Errorf("unsupported payload type: %s", result.name)
			}

			if err != nil {
				s.log.Errorf("Unable to submit payload: %s", err)
				continue
			}

			if statuses := readResponseStatuses(result.name, responses); len(statuses) > 0 {
				if updateRTStatus {
					notifyRTStatusChange(s.rtNotifierChan, statuses)
				}
			}
		}
	}
}

func (s *CheckSubmitter) resultsQueueForCheck(name string) *api.WeightedQueue {
	switch name {
	case checks.PodCheckName:
		return s.podResults
	case checks.RTProcessCheckName, checks.RTContainerCheckName:
		return s.rtProcessResults
	case checks.ConnectionsCheckName:
		return s.connectionsResults
	case checks.ProcessEventsCheckName:
		return s.eventResults
	}
	return s.processResults
}

func (s *CheckSubmitter) logQueuesSize() {
	var (
		processSize     = s.processResults.Len()
		rtProcessSize   = s.rtProcessResults.Len()
		connectionsSize = s.connectionsResults.Len()
		eventsSize      = s.eventResults.Len()
		podSize         = s.podResults.Len()
	)

	if processSize == 0 &&
		rtProcessSize == 0 &&
		connectionsSize == 0 &&
		eventsSize == 0 &&
		podSize == 0 {
		return
	}

	s.log.Infof(
		"Delivery queues: process[size=%d, weight=%d], rtprocess[size=%d, weight=%d], connections[size=%d, weight=%d], event[size=%d, weight=%d], pod[size=%d, weight=%d]",
		processSize, s.processResults.Weight(),
		rtProcessSize, s.rtProcessResults.Weight(),
		connectionsSize, s.connectionsResults.Weight(),
		eventsSize, s.eventResults.Weight(),
		podSize, s.podResults.Weight(),
	)
}

func (s *CheckSubmitter) messagesToResultsQueue(start time.Time, name string, messages []model.MessageBody, queue *api.WeightedQueue) {
	result := s.messagesToCheckResult(start, name, messages)
	if result == nil {
		return
	}
	queue.Add(result)
	// update proc and container count for info
	status.UpdateProcContainerCount(messages)
}

func (s *CheckSubmitter) messagesToCheckResult(start time.Time, name string, messages []model.MessageBody) *checkResult {
	if len(messages) == 0 {
		return nil
	}

	payloads := make([]checkPayload, 0, len(messages))
	sizeInBytes := 0

	for messageIndex, m := range messages {
		body, err := api.EncodePayload(m)
		if err != nil {
			s.log.Errorf("Unable to encode message: %s", err)
			continue
		}

		agentVersion, _ := version.Agent()
		extraHeaders := make(http.Header)
		extraHeaders.Set(headers.TimestampHeader, strconv.Itoa(int(start.Unix())))
		extraHeaders.Set(headers.HostHeader, s.hostname)
		extraHeaders.Set(headers.ProcessVersionHeader, agentVersion.GetNumber())
		extraHeaders.Set(headers.ContainerCountHeader, strconv.Itoa(getContainerCount(m)))
		extraHeaders.Set(headers.ContentTypeHeader, headers.ProtobufContentType)
		extraHeaders.Set(headers.AgentStartTime, strconv.FormatInt(s.agentStartTime, 10))

		if s.orchestrator.OrchestrationCollectionEnabled {
			if cid, err := clustername.GetClusterID(); err == nil && cid != "" {
				extraHeaders.Set(headers.ClusterIDHeader, cid)
			}
			extraHeaders.Set(headers.EVPOriginHeader, "process-agent")
			extraHeaders.Set(headers.EVPOriginVersionHeader, version.AgentVersion)
		}

		switch name {
		case checks.ProcessEventsCheckName:
			extraHeaders.Set(headers.EVPOriginHeader, "process-agent")
			extraHeaders.Set(headers.EVPOriginVersionHeader, version.AgentVersion)
		case checks.ConnectionsCheckName, checks.ProcessCheckName:
			requestID := s.getRequestID(start, messageIndex)
			s.log.Debugf("the request id of the current message: %s", requestID)
			extraHeaders.Set(headers.RequestIDHeader, requestID)
		}

		payloads = append(payloads, checkPayload{
			body:    body,
			headers: extraHeaders,
		})

		sizeInBytes += len(body)
	}

	return &checkResult{
		name:        name,
		payloads:    payloads,
		sizeInBytes: int64(sizeInBytes),
	}
}

// getRequestID generates a unique identifier (string representation of 64 bits integer) that is composed as follows:
//  1. 22 bits of the seconds in the current month.
//  2. 28 bits of hash of the hostname and process agent pid.
//  3. 14 bits of the current message in the batch being sent to the server.
func (s *CheckSubmitter) getRequestID(start time.Time, chunkIndex int) string {
	// The epoch is the beginning of the month of the `start` variable.
	epoch := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location())
	// We are taking the seconds in the current month, and representing them under 22 bits.
	// In a month we have 60 seconds per minute * 60 minutes per hour * 24 hours per day * maximum 31 days a month
	// which is 2678400, and it can be represented with log2(2678400) = 21.35 bits.
	seconds := (uint64(start.Sub(epoch).Seconds()) & secondsMask) << (hashNumberOfBits + chunkNumberOfBits)

	//// Next, we want 28 bits of hashed hostname & process agent pid.
	if s.requestIDCachedHash == nil {
		hash := fnv.New32()
		hash.Write([]byte(s.hostname))
		hash.Write([]byte(strconv.Itoa(os.Getpid())))
		hostNamePIDHash := (uint64(hash.Sum32()) & hashMask) << chunkNumberOfBits
		s.requestIDCachedHash = &hostNamePIDHash
	}

	// Next, we take up to 14 bits to represent the message index in the batch.
	// It means that we support up to 16384 (2 ^ 14) different messages being sent on the same batch.
	chunk := uint64(chunkIndex & chunkMask)
	return fmt.Sprintf("%d", seconds+*s.requestIDCachedHash+chunk)
}

func (s *CheckSubmitter) shouldDropPayload(check string) bool {
	for _, d := range s.dropCheckPayloads {
		if d == check {
			return true
		}
	}

	return false
}

func notifyRTStatusChange(rtNotifierChan chan<- types.RTResponse, statuses types.RTResponse) {
	select {
	case rtNotifierChan <- statuses:
	default: // Never block on the rtNotifierChan in case the runner has somehow stopped
	}
}
