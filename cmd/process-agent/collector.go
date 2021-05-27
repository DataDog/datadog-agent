package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/forwarder"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
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
	runCounter := int32(1)
	if rc, ok := l.runCounters.Load(c.Name()); ok {
		runCounter = rc.(int32) + 1
	}
	l.runCounters.Store(c.Name(), runCounter)

	start := time.Now()
	// update the last collected timestamp for info
	updateLastCollectTime(start)
	messages, err := c.Run(l.cfg, atomic.AddInt32(&l.groupID, 1))
	if err != nil {
		log.Errorf("Unable to run check '%s': %s", c.Name(), err)
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

		if cid, err := clustername.GetClusterID(); err == nil && cid != "" {
			extraHeaders.Set(headers.ClusterIDHeader, cid)
		}

		payloads = append(payloads, checkPayload{
			body:    body,
			headers: extraHeaders,
		})

		sizeInBytes += len(body)
	}

	result := &checkResult{
		name:        c.Name(),
		payloads:    payloads,
		sizeInBytes: int64(sizeInBytes),
	}

	results.Add(result)

	// update proc and container count for info
	updateProcContainerCount(messages)

	if !c.RealTime() {
		d := time.Since(start)
		switch {
		case runCounter < 5:
			log.Infof("Finished %s check #%d in %s", c.Name(), runCounter, d)
		case runCounter == 5:
			log.Infof("Finished %s check #%d in %s. First 5 check runs finished, next runs will be logged every 20 runs.", c.Name(), runCounter, d)
		case runCounter%20 == 0:
			log.Infof("Finish %s check #%d in %s", c.Name(), runCounter, d)
		}
	}
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

	processResults := api.NewWeightedQueue(l.cfg.QueueSize, int64(l.cfg.ProcessQueueBytes))
	podResults := api.NewWeightedQueue(l.cfg.QueueSize, int64(l.cfg.Orchestrator.PodQueueBytes))

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
				updateQueueBytes(processResults.Weight(), podResults.Weight())
				updateQueueSize(processResults.Len(), podResults.Len())
			case <-queueLogTicker.C:
				processSize, podSize := processResults.Len(), podResults.Len()
				if processSize > 0 || podSize > 0 {
					log.Infof(
						"Delivery queues: process[size=%d, weight=%d], pod[size=%d, weight=%d]",
						processSize, processResults.Weight(), podSize, podResults.Weight(),
					)
				}
			case <-exit:
				return
			}
		}
	}()

	processForwarderOpts := forwarder.NewOptions(apicfg.KeysPerDomains(l.cfg.APIEndpoints))
	processForwarderOpts.DisableAPIKeyChecking = true
	processForwarderOpts.RetryQueuePayloadsTotalMaxSize = l.cfg.ProcessQueueBytes // Allow more in-flight requests than the default
	processForwarder := forwarder.NewDefaultForwarder(processForwarderOpts)

	podForwarderOpts := forwarder.NewOptions(apicfg.KeysPerDomains(l.cfg.Orchestrator.OrchestratorEndpoints))
	podForwarderOpts.DisableAPIKeyChecking = true
	podForwarderOpts.RetryQueuePayloadsTotalMaxSize = l.cfg.ProcessQueueBytes // Allow more in-flight requests than the default
	podForwarder := forwarder.NewDefaultForwarder(podForwarderOpts)

	if err := processForwarder.Start(); err != nil {
		return fmt.Errorf("error starting forwarder: %s", err)
	}

	if err := podForwarder.Start(); err != nil {
		return fmt.Errorf("error starting pod forwarder: %s", err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		l.consumePayloads(processResults, processForwarder, exit)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		l.consumePayloads(podResults, podForwarder, exit)
	}()

	for _, c := range l.enabledChecks {
		results := processResults
		if c.Name() == checks.Pod.Name() {
			results = podResults
		}

		wg.Add(1)
		go func(c checks.Check, results *api.WeightedQueue) {
			defer wg.Done()

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
		}(c, results)
	}

	<-exit
	wg.Wait()

	processForwarder.Stop()
	podForwarder.Stop()
	return nil
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
			case checks.RTProcess.Name():
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
				responses, err = fwd.SubmitOrchestratorChecks(forwarderPayload, payload.headers, checks.Pod.Name())
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
