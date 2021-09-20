package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type checkResult struct {
	name        string
	payloads    []checkPayload
	sizeInBytes int64
}

func (cr *checkResult) Weight() int64 {
	return cr.sizeInBytes
}

func (cr *checkResult) Type() string {
	return cr.name
}

var _ api.WeightedItem = &checkResult{}

type checkPayload struct {
	body    []byte
	headers http.Header
}

// Collector will collect metrics from the local system and ship to the backend.
type Collector struct {
	// Set to 1 if enabled 0 is not. We're using an integer
	// so we can use the sync/atomic for thread-safe access.
	realTimeEnabled int32

	groupID int32

	rtIntervalCh chan time.Duration
	cfg          *config.AgentConfig

	// counters for each type of check
	runCounters   sync.Map
	enabledChecks []checks.Check

	// Controls the real-time interval, can change live.
	realTimeInterval time.Duration

	processResults   *api.WeightedQueue
	rtProcessResults *api.WeightedQueue
	podResults       *api.WeightedQueue
}

// NewCollector creates a new Collector
func NewCollector(cfg *config.AgentConfig) (Collector, error) {
	sysInfo, err := checks.CollectSystemInfo(cfg)
	if err != nil {
		return Collector{}, err
	}

	enabledChecks := make([]checks.Check, 0)
	for _, c := range checks.All {
		if cfg.CheckIsEnabled(c.Name()) {
			c.Init(cfg, sysInfo)
			enabledChecks = append(enabledChecks, c)
		}
	}

	return NewCollectorWithChecks(cfg, enabledChecks), nil
}

// NewCollectorWithChecks creates a new Collector
func NewCollectorWithChecks(cfg *config.AgentConfig, checks []checks.Check) Collector {
	return Collector{
		rtIntervalCh:  make(chan time.Duration),
		cfg:           cfg,
		groupID:       rand.Int31(),
		enabledChecks: checks,

		// Defaults for real-time on start
		realTimeInterval: 2 * time.Second,
		realTimeEnabled:  0,
	}
}

func (l *Collector) runCheck(c checks.Check, results *api.WeightedQueue) {
	runCounter := l.nextRunCounter(c.Name())
	start := time.Now()
	// update the last collected timestamp for info
	updateLastCollectTime(start)

	messages, err := c.Run(l.cfg, l.nextGroupID())
	if err != nil {
		log.Errorf("Unable to run check '%s': %s", c.Name(), err)
		return
	}
	l.messagesToResults(start, c.Name(), messages, results)

	if !c.RealTime() {
		logCheckDuration(c.Name(), start, runCounter)
	}
}

func (l *Collector) runCheckWithRealTime(c checks.CheckWithRealTime, results, rtResults *api.WeightedQueue, options checks.RunOptions) {
	runCounter := l.nextRunCounter(c.Name())
	start := time.Now()
	// update the last collected timestamp for info
	updateLastCollectTime(start)

	run, err := c.RunWithOptions(l.cfg, l.nextGroupID, options)
	if err != nil {
		log.Errorf("Unable to run check '%s': %s", c.Name(), err)
		return
	}
	l.messagesToResults(start, c.Name(), run.Standard, results)
	l.messagesToResults(start, c.RealTimeName(), run.RealTime, rtResults)

	if options.RunStandard {
		logCheckDuration(c.Name(), start, runCounter)
	}
}

func (l *Collector) nextRunCounter(name string) int32 {
	runCounter := int32(1)
	if rc, ok := l.runCounters.Load(name); ok {
		runCounter = rc.(int32) + 1
	}
	l.runCounters.Store(name, runCounter)
	return runCounter
}

func logCheckDuration(name string, start time.Time, runCounter int32) {
	d := time.Since(start)
	switch {
	case runCounter < 5:
		log.Infof("Finished %s check #%d in %s", name, runCounter, d)
	case runCounter == 5:
		log.Infof("Finished %s check #%d in %s. First 5 check runs finished, next runs will be logged every 20 runs.", name, runCounter, d)
	case runCounter%20 == 0:
		log.Infof("Finish %s check #%d in %s", name, runCounter, d)
	}
}

func (l *Collector) nextGroupID() int32 {
	return atomic.AddInt32(&l.groupID, 1)
}

func (l *Collector) messagesToResults(start time.Time, name string, messages []model.MessageBody, results *api.WeightedQueue) {
	if len(messages) == 0 {
		return
	}

	payloads := make([]checkPayload, 0, len(messages))
	sizeInBytes := 0

	for _, m := range messages {
		body, err := api.EncodePayload(m)
		if err != nil {
			log.Errorf("Unable to encode message: %s", err)
			continue
		}

		extraHeaders := make(http.Header)
		extraHeaders.Set(headers.TimestampHeader, strconv.Itoa(int(start.Unix())))
		extraHeaders.Set(headers.HostHeader, l.cfg.HostName)
		extraHeaders.Set(headers.ProcessVersionHeader, Version)
		extraHeaders.Set(headers.ContainerCountHeader, strconv.Itoa(getContainerCount(m)))

		if l.cfg.Orchestrator.OrchestrationCollectionEnabled {
			if cid, err := clustername.GetClusterID(); err == nil && cid != "" {
				extraHeaders.Set(headers.ClusterIDHeader, cid)
			}
		}

		payloads = append(payloads, checkPayload{
			body:    body,
			headers: extraHeaders,
		})

		sizeInBytes += len(body)
	}

	result := &checkResult{
		name:        name,
		payloads:    payloads,
		sizeInBytes: int64(sizeInBytes),
	}
	results.Add(result)
	// update proc and container count for info
	updateProcContainerCount(messages)
}

func (l *Collector) run(exit chan struct{}) error {
	eps := make([]string, 0, len(l.cfg.APIEndpoints))
	for _, e := range l.cfg.APIEndpoints {
		eps = append(eps, e.Endpoint.String())
	}
	orchestratorEps := make([]string, 0, len(l.cfg.Orchestrator.OrchestratorEndpoints))
	for _, e := range l.cfg.Orchestrator.OrchestratorEndpoints {
		orchestratorEps = append(orchestratorEps, e.Endpoint.String())
	}
	log.Infof("Starting process-agent for host=%s, endpoints=%s, orchestrator endpoints=%s, enabled checks=%v", l.cfg.HostName, eps, orchestratorEps, l.cfg.EnabledChecks)

	go util.HandleSignals(exit)

	l.processResults = api.NewWeightedQueue(l.cfg.QueueSize, int64(l.cfg.ProcessQueueBytes))
	// reuse main queue's ProcessQueueBytes because it's unlikely that it'll reach to that size in bytes, so we don't need a separate config for it
	l.rtProcessResults = api.NewWeightedQueue(l.cfg.RTQueueSize, int64(l.cfg.ProcessQueueBytes))
	l.podResults = api.NewWeightedQueue(l.cfg.QueueSize, int64(l.cfg.Orchestrator.PodQueueBytes))

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()

		queueSizeTicker := time.NewTicker(10 * time.Second)
		defer queueSizeTicker.Stop()

		queueLogTicker := time.NewTicker(time.Minute)
		defer queueLogTicker.Stop()

		tags := []string{
			fmt.Sprintf("version:%s", Version),
			fmt.Sprintf("revision:%s", GitCommit),
		}
		for {
			select {
			case <-heartbeat.C:
				statsd.Client.Gauge("datadog.process.agent", 1, tags, 1) //nolint:errcheck
			case <-queueSizeTicker.C:
				updateQueueBytes(l.processResults.Weight(), l.rtProcessResults.Weight(), l.podResults.Weight())
				updateQueueSize(l.processResults.Len(), l.rtProcessResults.Len(), l.podResults.Len())
			case <-queueLogTicker.C:
				processSize, rtProcessSize, podSize := l.processResults.Len(), l.rtProcessResults.Len(), l.podResults.Len()
				if processSize > 0 || rtProcessSize > 0 || podSize > 0 {
					log.Infof(
						"Delivery queues: process[size=%d, weight=%d], rtprocess [size=%d, weight=%d], pod[size=%d, weight=%d]",
						processSize, l.processResults.Weight(), rtProcessSize, l.rtProcessResults.Weight(), podSize, l.podResults.Weight(),
					)
				}
			case <-exit:
				return
			}
		}
	}()

	processForwarderOpts := forwarder.NewOptions(forwarder.NewSingleDomainResolvers(apicfg.KeysPerDomains(l.cfg.APIEndpoints)))
	processForwarderOpts.DisableAPIKeyChecking = true
	processForwarderOpts.RetryQueuePayloadsTotalMaxSize = l.cfg.ProcessQueueBytes // Allow more in-flight requests than the default
	processForwarder := forwarder.NewDefaultForwarder(processForwarderOpts)

	// rt forwarder can reuse processForwarder's config
	rtProcessForwarder := forwarder.NewDefaultForwarder(processForwarderOpts)

	podForwarderOpts := forwarder.NewOptions(forwarder.NewSingleDomainResolvers(apicfg.KeysPerDomains(l.cfg.Orchestrator.OrchestratorEndpoints)))
	podForwarderOpts.DisableAPIKeyChecking = true
	podForwarderOpts.RetryQueuePayloadsTotalMaxSize = l.cfg.ProcessQueueBytes // Allow more in-flight requests than the default
	podForwarder := forwarder.NewDefaultForwarder(podForwarderOpts)

	if err := processForwarder.Start(); err != nil {
		return fmt.Errorf("error starting forwarder: %s", err)
	}

	if err := rtProcessForwarder.Start(); err != nil {
		return fmt.Errorf("error starting RT forwarder: %s", err)
	}

	if err := podForwarder.Start(); err != nil {
		return fmt.Errorf("error starting pod forwarder: %s", err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		l.consumePayloads(l.processResults, processForwarder, exit)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		l.consumePayloads(l.rtProcessResults, rtProcessForwarder, exit)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		l.consumePayloads(l.podResults, podForwarder, exit)
	}()

	for _, c := range l.enabledChecks {
		runner, err := l.runnerForCheck(c, exit)
		if err != nil {
			return fmt.Errorf("error starting check %s: %s", c.Name(), err)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			runner()
		}()
	}

	<-exit
	wg.Wait()

	processForwarder.Stop()
	rtProcessForwarder.Stop()
	podForwarder.Stop()
	return nil
}

func (l *Collector) resultsQueueForCheck(name string) *api.WeightedQueue {
	switch name {
	case checks.Pod.Name():
		return l.podResults
	case checks.Process.RealTimeName(), checks.RTContainer.Name():
		return l.rtProcessResults
	}
	return l.processResults
}

func (l *Collector) runnerForCheck(c checks.Check, exit chan struct{}) (func(), error) {
	results := l.resultsQueueForCheck(c.Name())

	if withRealTime, ok := c.(checks.CheckWithRealTime); ok {
		rtResults := l.resultsQueueForCheck(withRealTime.RealTimeName())

		return checks.NewRunnerWithRealTime(
			checks.RunnerConfig{
				CheckInterval: l.cfg.CheckInterval(withRealTime.Name()),
				RtInterval:    l.cfg.CheckInterval(withRealTime.RealTimeName()),

				ExitChan:       exit,
				RtIntervalChan: l.rtIntervalCh,
				RtEnabled: func() bool {
					return atomic.LoadInt32(&l.realTimeEnabled) == 1
				},
				RunCheck: func(options checks.RunOptions) {
					l.runCheckWithRealTime(withRealTime, results, rtResults, options)
				},
			},
		)
	}
	return l.basicRunner(c, results, exit), nil
}

func (l *Collector) basicRunner(c checks.Check, results *api.WeightedQueue, exit chan struct{}) func() {
	return func() {
		// Run the check the first time to prime the caches.
		if !c.RealTime() {
			l.runCheck(c, results)
		}

		ticker := time.NewTicker(l.cfg.CheckInterval(c.Name()))
		for {
			select {
			case <-ticker.C:
				realTimeEnabled := atomic.LoadInt32(&l.realTimeEnabled) == 1
				if !c.RealTime() || realTimeEnabled {
					l.runCheck(c, results)
				}
			case d := <-l.rtIntervalCh:
				// Live-update the ticker.
				if c.RealTime() {
					ticker.Stop()
					ticker = time.NewTicker(d)
				}
			case _, ok := <-exit:
				if !ok {
					return
				}
			}
		}
	}
}

func (l *Collector) consumePayloads(results *api.WeightedQueue, fwd forwarder.Forwarder, exit chan struct{}) {
	for {
		// results.Poll() will block until either `exit` is closed, or an item is available on the queue (a check run occurs and adds data)
		item, ok := results.Poll(exit)
		if !ok {
			return
		}
		result := item.(*checkResult)
		for _, payload := range result.payloads {
			var (
				forwarderPayload = forwarder.Payloads{&payload.body}
				responses        chan forwarder.Response
				err              error
				updateRTStatus   = true
			)

			switch result.name {
			case checks.Process.Name():
				responses, err = fwd.SubmitProcessChecks(forwarderPayload, payload.headers)
			case checks.Process.RealTimeName():
				responses, err = fwd.SubmitRTProcessChecks(forwarderPayload, payload.headers)
			case checks.Container.Name():
				responses, err = fwd.SubmitContainerChecks(forwarderPayload, payload.headers)
			case checks.RTContainer.Name():
				responses, err = fwd.SubmitRTContainerChecks(forwarderPayload, payload.headers)
			case checks.Connections.Name():
				responses, err = fwd.SubmitConnectionChecks(forwarderPayload, payload.headers)
			case checks.Pod.Name():
				// Orchestrator intake response does not change RT checks enablement or interval
				updateRTStatus = false
				responses, err = fwd.SubmitOrchestratorChecks(forwarderPayload, payload.headers, int(orchestrator.K8sPod))
			case checks.ProcessDiscovery.Name():
				// A Process Discovery check does not change the RT mode
				updateRTStatus = false
				responses, err = fwd.SubmitProcessDiscoveryChecks(forwarderPayload, payload.headers)
			default:
				err = fmt.Errorf("unsupported payload type: %s", result.name)
			}

			if err != nil {
				log.Errorf("Unable to submit payload: %s", err)
				continue
			}

			if statuses := readResponseStatuses(result.name, responses); len(statuses) > 0 {
				if updateRTStatus {
					l.updateRTStatus(statuses)
				}
			}
		}
	}
}

func (l *Collector) updateRTStatus(statuses []*model.CollectorStatus) {
	curEnabled := atomic.LoadInt32(&l.realTimeEnabled) == 1

	// If any of the endpoints wants real-time we'll do that.
	// We will pick the maximum interval given since generally this is
	// only set if we're trying to limit load on the backend.
	shouldEnableRT := false
	maxInterval := 0 * time.Second
	activeClients := int32(0)
	for _, s := range statuses {
		shouldEnableRT = shouldEnableRT || (s.ActiveClients > 0 && l.cfg.AllowRealTime)
		if s.ActiveClients > 0 {
			activeClients += s.ActiveClients
		}
		interval := time.Duration(s.Interval) * time.Second
		if interval > maxInterval {
			maxInterval = interval
		}
	}

	if curEnabled && !shouldEnableRT {
		log.Info("Detected 0 clients, disabling real-time mode")
		atomic.StoreInt32(&l.realTimeEnabled, 0)
	} else if !curEnabled && shouldEnableRT {
		log.Infof("Detected %d active clients, enabling real-time mode", activeClients)
		atomic.StoreInt32(&l.realTimeEnabled, 1)
	}

	if maxInterval != l.realTimeInterval {
		l.realTimeInterval = maxInterval
		if l.realTimeInterval <= 0 {
			l.realTimeInterval = 2 * time.Second
		}
		// Pass along the real-time interval, one per check, so that every
		// check routine will see the new interval.
		for range l.enabledChecks {
			l.rtIntervalCh <- l.realTimeInterval
		}
		log.Infof("real time interval updated to %s", l.realTimeInterval)
	}
}

// getContainerCount returns the number of containers in the message body
func getContainerCount(mb model.MessageBody) int {
	switch v := mb.(type) {
	case *model.CollectorProc:
		return len(v.GetContainers())
	case *model.CollectorRealTime:
		return len(v.GetContainerStats())
	case *model.CollectorContainer:
		return len(v.GetContainers())
	case *model.CollectorContainerRealTime:
		return len(v.GetStats())
	case *model.CollectorConnections:
		return 0
	}
	return 0
}

func readResponseStatuses(checkName string, responses <-chan forwarder.Response) []*model.CollectorStatus {
	var statuses []*model.CollectorStatus

	for response := range responses {
		if response.Err != nil {
			log.Errorf("[%s] Error from %s: %s", checkName, response.Domain, response.Err)
			continue
		}

		if response.StatusCode >= 300 {
			log.Errorf("[%s] Invalid response from %s: %d -> %s", checkName, response.Domain, response.StatusCode, response.Err)
			continue
		}

		r, err := model.DecodeMessage(response.Body)
		if err != nil {
			log.Errorf("[%s] Could not decode response body: %s", checkName, err)
			continue
		}

		switch r.Header.Type {
		case model.TypeResCollector:
			rm := r.Body.(*model.ResCollector)
			if len(rm.Message) > 0 {
				log.Errorf("[%s] Error in response from %s: %s", checkName, response.Domain, rm.Message)
			} else {
				statuses = append(statuses, rm.Status)
			}
		default:
			log.Errorf("[%s] Unexpected response type from %s: %d", checkName, response.Domain, r.Header.Type)
		}
	}

	return statuses
}
