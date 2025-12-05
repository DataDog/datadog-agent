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
	"slices"
	"strconv"
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"

	//nolint:revive // TODO(PROC) Fix revive linter
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/comp/process/forwarders"
	"github.com/DataDog/datadog-agent/comp/process/types"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/runner/endpoint"
	"github.com/DataDog/datadog-agent/pkg/process/status"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/version"
)

//nolint:revive // TODO(PROC) Fix revive linter
type Submitter interface {
	Submit(start time.Time, name string, messages *types.Payload)
}

var _ Submitter = &CheckSubmitter{}

type submitFunc func(transaction.BytesPayloads, http.Header) (chan forwarder.Response, error)

//nolint:revive // TODO(PROC) Fix revive linter
type CheckSubmitter struct {
	log            log.Component
	queues         []*api.WeightedQueue
	resultsQueue   map[string]*api.WeightedQueue
	submitFuncs    map[string]submitFunc
	realtimeUpdate map[string]bool

	// Endpoints for logging purposes
	processAPIEndpoints []apicfg.Endpoint

	hostname string

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

	stopHeartbeat chan struct{}
	clock         clock.Clock

	statsd statsd.ClientInterface

	// Used to set headers on the payloads
	processesEnabled        string
	serviceDiscoveryEnabled string
}

//nolint:revive // TODO(PROC) Fix revive linter
func NewSubmitter(config config.Component, log log.Component, forwarders forwarders.Component, statsd statsd.ClientInterface, hostname string, sysprobeconfig sysprobeconfig.Component) (*CheckSubmitter, error) {
	queueBytes := config.GetInt("process_config.process_queue_bytes")
	if queueBytes <= 0 {
		log.Warnf("Invalid queue bytes size: %d. Using default value: %d", queueBytes, pkgconfigsetup.DefaultProcessQueueBytes)
		queueBytes = pkgconfigsetup.DefaultProcessQueueBytes
	}

	queueSize := config.GetInt("process_config.queue_size")
	if queueSize <= 0 {
		log.Warnf("Invalid check queue size: %d. Using default value: %d", queueSize, pkgconfigsetup.DefaultProcessQueueSize)
		queueSize = pkgconfigsetup.DefaultProcessQueueSize
	}
	processResults := api.NewWeightedQueue(queueSize, int64(queueBytes))
	log.Debugf("Creating process check queue with max_size=%d and max_weight=%d", processResults.MaxSize(), processResults.MaxWeight())

	rtQueueSize := config.GetInt("process_config.rt_queue_size")
	if rtQueueSize <= 0 {
		log.Warnf("Invalid rt check queue size: %d. Using default value: %d", rtQueueSize, pkgconfigsetup.DefaultProcessRTQueueSize)
		rtQueueSize = pkgconfigsetup.DefaultProcessRTQueueSize
	}
	// reuse main queue's ProcessQueueBytes because it's unlikely that it'll reach to that size in bytes, so we don't need a separate config for it
	rtProcessResults := api.NewWeightedQueue(rtQueueSize, int64(queueBytes))
	log.Debugf("Creating rt process check queue with max_size=%d and max_weight=%d", rtProcessResults.MaxSize(), rtProcessResults.MaxWeight())

	connectionsResults := api.NewWeightedQueue(queueSize, int64(queueBytes))
	log.Debugf("Creating connections queue with max_size=%d and max_weight=%d", connectionsResults.MaxSize(), connectionsResults.MaxWeight())

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

	processFwd := forwarders.GetProcessForwarder()
	rtProcessFwd := forwarders.GetRTProcessForwarder()

	return &CheckSubmitter{
		log: log,
		queues: []*api.WeightedQueue{
			processResults,
			rtProcessResults,
			connectionsResults,
		},
		resultsQueue: map[string]*api.WeightedQueue{
			checks.ProcessCheckName:     processResults,
			checks.DiscoveryCheckName:   processResults,
			checks.ContainerCheckName:   processResults,
			checks.RTProcessCheckName:   rtProcessResults,
			checks.RTContainerCheckName: rtProcessResults,
			checks.ConnectionsCheckName: connectionsResults,
		},
		submitFuncs: map[string]submitFunc{
			checks.ProcessCheckName:     processFwd.SubmitProcessChecks,
			checks.DiscoveryCheckName:   processFwd.SubmitProcessDiscoveryChecks,
			checks.ContainerCheckName:   processFwd.SubmitContainerChecks,
			checks.RTProcessCheckName:   rtProcessFwd.SubmitRTProcessChecks,
			checks.RTContainerCheckName: rtProcessFwd.SubmitRTContainerChecks,
			checks.ConnectionsCheckName: forwarders.GetConnectionsForwarder().SubmitConnectionChecks,
		},
		realtimeUpdate: map[string]bool{
			checks.ProcessCheckName:     true,
			checks.RTProcessCheckName:   true,
			checks.ContainerCheckName:   true,
			checks.RTContainerCheckName: true,
		},

		processAPIEndpoints: processAPIEndpoints,

		hostname: hostname,

		dropCheckPayloads: dropCheckPayloads,

		forwarderRetryMaxQueueBytes: queueBytes,

		rtNotifierChan: make(chan types.RTResponse, 1), // Buffer the channel so we don't block submissions

		wg:   &sync.WaitGroup{},
		exit: make(chan struct{}),

		agentStartTime: time.Now().Unix(),

		stopHeartbeat: make(chan struct{}),
		clock:         clock.New(),

		statsd: statsd,

		processesEnabled:        config.GetString("process_config.process_collection.enabled"),
		serviceDiscoveryEnabled: sysprobeconfig.GetString("discovery.enabled"),
	}, nil
}

func printStartMessage(log log.Component, hostname string, processAPIEndpoints []apicfg.Endpoint) {
	eps := make([]string, 0, len(processAPIEndpoints))
	for _, e := range processAPIEndpoints {
		eps = append(eps, e.Endpoint.String())
	}

	log.Infof("Starting CheckSubmitter for host=%s, endpoints=%s", hostname, eps)
}

//nolint:revive // TODO(PROC) Fix revive linter
func (s *CheckSubmitter) Submit(start time.Time, name string, messages *types.Payload) {
	results := s.resultsQueue[name]
	s.messagesToResultsQueue(start, name, messages.Message, results)
}

//nolint:revive // TODO(PROC) Fix revive linter
func (s *CheckSubmitter) Start() error {
	printStartMessage(s.log, s.hostname, s.processAPIEndpoints)

	for _, q := range s.queues {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.consumePayloads(q)
		}()
	}

	if flavor.GetFlavor() == flavor.ProcessAgent {
		heartbeatTicker := s.clock.Ticker(15 * time.Second)
		s.wg.Add(1)
		go func() {
			defer heartbeatTicker.Stop()
			defer s.wg.Done()
			s.heartbeat(heartbeatTicker)
		}()
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		queueSizeTicker := s.clock.Ticker(10 * time.Second)
		defer queueSizeTicker.Stop()

		queueLogTicker := s.clock.Ticker(time.Minute)
		defer queueLogTicker.Stop()

		for {
			select {
			case <-queueSizeTicker.C:
				status.UpdateQueueStats(&status.QueueStats{
					ProcessQueueSize:      s.resultsQueue[checks.ProcessCheckName].Len(),
					RtProcessQueueSize:    s.resultsQueue[checks.RTProcessCheckName].Len(),
					ConnectionsQueueSize:  s.resultsQueue[checks.ConnectionsCheckName].Len(),
					ProcessQueueBytes:     s.resultsQueue[checks.ProcessCheckName].Weight(),
					RtProcessQueueBytes:   s.resultsQueue[checks.RTProcessCheckName].Weight(),
					ConnectionsQueueBytes: s.resultsQueue[checks.ConnectionsCheckName].Weight(),
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

//nolint:revive // TODO(PROC) Fix revive linter
func (s *CheckSubmitter) Stop() {
	close(s.exit)

	for _, q := range s.queues {
		q.Stop()
	}

	close(s.stopHeartbeat)

	s.wg.Wait()

	close(s.rtNotifierChan)
}

//nolint:revive // TODO(PROC) Fix revive linter
func (s *CheckSubmitter) GetRTNotifierChan() <-chan types.RTResponse {
	return s.rtNotifierChan
}

func (s *CheckSubmitter) consumePayloads(results *api.WeightedQueue) {
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

			submitFn, ok := s.submitFuncs[result.name]
			updateRTStatus = s.realtimeUpdate[result.name]
			if !ok {
				err = fmt.Errorf("unsupported payload type: %s", result.name)
			} else {
				responses, err = submitFn(forwarderPayload, payload.headers)
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

func (s *CheckSubmitter) logQueuesSize() {
	var (
		processSize     = s.resultsQueue[checks.ProcessCheckName].Len()
		rtProcessSize   = s.resultsQueue[checks.RTProcessCheckName].Len()
		connectionsSize = s.resultsQueue[checks.ConnectionsCheckName].Len()
	)

	if processSize == 0 &&
		rtProcessSize == 0 &&
		connectionsSize == 0 {
		return
	}

	s.log.Infof(
		"Delivery queues: process[size=%d, weight=%d], rtprocess[size=%d, weight=%d], connections[size=%d, weight=%d]",
		processSize, s.resultsQueue[checks.ProcessCheckName].Weight(),
		rtProcessSize, s.resultsQueue[checks.RTProcessCheckName].Weight(),
		connectionsSize, s.resultsQueue[checks.ConnectionsCheckName].Weight(),
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
		extraHeaders.Set(headers.PayloadSource, flavor.GetFlavor())
		extraHeaders.Set(headers.ProcessesEnabled, s.processesEnabled)
		extraHeaders.Set(headers.ServiceDiscoveryEnabled, s.serviceDiscoveryEnabled)

		switch name {
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
	return strconv.FormatUint(seconds+*s.requestIDCachedHash+chunk, 10)
}

func (s *CheckSubmitter) shouldDropPayload(check string) bool {
	return slices.Contains(s.dropCheckPayloads, check)
}

func (s *CheckSubmitter) heartbeat(heartbeatTicker *clock.Ticker) {
	agentVersion, _ := version.Agent()
	tags := []string{
		"version:" + agentVersion.GetNumberAndPre(),
		"revision:" + agentVersion.Commit,
	}

	for {
		select {
		case <-heartbeatTicker.C:
			_ = s.statsd.Gauge("datadog.process.agent", 1, tags, 1)
		case <-s.stopHeartbeat:
			return
		}
	}
}

func notifyRTStatusChange(rtNotifierChan chan<- types.RTResponse, statuses types.RTResponse) {
	select {
	case rtNotifierChan <- statuses:
	default: // Never block on the rtNotifierChan in case the runner has somehow stopped
	}
}
