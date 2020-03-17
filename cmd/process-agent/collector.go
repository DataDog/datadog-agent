package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
)

type checkPayload struct {
	messages []model.MessageBody
	endpoint string
	name     string
}

// Collector will collect metrics from the local system and ship to the backend.
type Collector struct {
	// Set to 1 if enabled 0 is not. We're using an integer
	// so we can use the sync/atomic for thread-safe access.
	realTimeEnabled int32

	groupID int32

	send         chan checkPayload
	rtIntervalCh chan time.Duration
	cfg          *config.AgentConfig
	apiClient    api.Client

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

	return Collector{
		send:          make(chan checkPayload, cfg.QueueSize),
		rtIntervalCh:  make(chan time.Duration),
		cfg:           cfg,
		groupID:       rand.Int31(),
		apiClient:     api.NewClient(http.Client{Timeout: HTTPTimeout, Transport: cfg.Transport}, ReqCtxTimeout),
		enabledChecks: enabledChecks,

		// Defaults for real-time on start
		realTimeInterval: 2 * time.Second,
		realTimeEnabled:  0,
	}, nil
}

func (l *Collector) runCheck(c checks.Check) {
	runCounter := int32(1)
	if rc, ok := l.runCounters.Load(c.Name()); ok {
		runCounter = rc.(int32) + 1
	}
	l.runCounters.Store(c.Name(), runCounter)

	s := time.Now()
	// update the last collected timestamp for info
	updateLastCollectTime(time.Now())
	messages, err := c.Run(l.cfg, atomic.AddInt32(&l.groupID, 1))
	if err != nil {
		log.Errorf("Unable to run check '%s': %s", c.Name(), err)
	} else {
		l.send <- checkPayload{messages, c.Endpoint(), c.Name()}
		// update proc and container count for info
		updateProcContainerCount(messages)
		if !c.RealTime() {
			d := time.Since(s)
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
}

func (l *Collector) run(exit chan bool) {
	eps := make([]string, 0, len(l.cfg.APIEndpoints))
	for _, e := range l.cfg.APIEndpoints {
		eps = append(eps, e.Endpoint.String())
	}
	orchestratorEps := make([]string, 0, len(l.cfg.OrchestratorEndpoints))
	for _, e := range l.cfg.OrchestratorEndpoints {
		orchestratorEps = append(orchestratorEps, e.Endpoint.String())
	}
	log.Infof("Starting process-agent for host=%s, endpoints=%s, orchestrator endpoints=%s, enabled checks=%v", l.cfg.HostName, eps, orchestratorEps, l.cfg.EnabledChecks)

	go util.HandleSignals(exit)
	heartbeat := time.NewTicker(15 * time.Second)
	queueSizeTicker := time.NewTicker(10 * time.Second)
	go func() {
		tags := []string{
			fmt.Sprintf("version:%s", Version),
			fmt.Sprintf("revision:%s", GitCommit),
		}
		for {
			select {
			case payload := <-l.send:
				if len(l.send) >= l.cfg.QueueSize {
					log.Info("Expiring payload from in-memory queue.")
					// Limit number of items kept in memory while we wait.
					<-l.send
				}
				for _, m := range payload.messages {
					extraHeaders := map[string]string{
						api.HostHeader:           l.cfg.HostName,
						api.ProcessVersionHeader: Version,
						api.ContainerCountHeader: strconv.Itoa(getContainerCount(m)),
					}
					statuses := l.apiClient.PostMessage(l.endpointsForCheck(payload.name), payload.endpoint, m, extraHeaders)
					if len(statuses) > 0 {
						l.updateStatus(statuses)
					}
				}
			case <-heartbeat.C:
				statsd.Client.Gauge("datadog.process.agent", 1, tags, 1)
			case <-queueSizeTicker.C:
				updateQueueSize(l.send)
			case <-exit:
				return
			}
		}
	}()

	for _, c := range l.enabledChecks {
		go func(c checks.Check) {
			// Run the check the first time to prime the caches.
			if !c.RealTime() {
				l.runCheck(c)
			}

			ticker := time.NewTicker(l.cfg.CheckInterval(c.Name()))
			for {
				select {
				case <-ticker.C:
					realTimeEnabled := atomic.LoadInt32(&l.realTimeEnabled) == 1
					if !c.RealTime() || realTimeEnabled {
						l.runCheck(c)
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
		}(c)
	}
	<-exit
}

func (l *Collector) updateStatus(statuses []*model.CollectorStatus) {
	curEnabled := atomic.LoadInt32(&l.realTimeEnabled) == 1

	// If any of the endpoints wants real-time we'll do that.
	// We will pick the maximum interval given since generally this is
	// only set if we're trying to limit load on the backend.
	shouldEnableRT := false
	maxInterval := 0 * time.Second
	for _, s := range statuses {
		shouldEnableRT = shouldEnableRT || (s.ActiveClients > 0 && l.cfg.AllowRealTime)
		interval := time.Duration(s.Interval) * time.Second
		if interval > maxInterval {
			maxInterval = interval
		}
	}

	if curEnabled && !shouldEnableRT {
		log.Info("Detected 0 clients, disabling real-time mode")
		atomic.StoreInt32(&l.realTimeEnabled, 0)
	} else if !curEnabled && shouldEnableRT {
		log.Info("Detected active clients, enabling real-time mode")
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

func (l *Collector) endpointsForCheck(checkName string) []api.Endpoint {
	if checkName == checks.Pod.Name() {
		return l.cfg.OrchestratorEndpoints
	}
	return l.cfg.APIEndpoints
}

const (
	// HTTPTimeout is the timeout in seconds for process-agent to send process payloads to DataDog
	HTTPTimeout = 20 * time.Second
	// ReqCtxTimeout is the timeout in seconds for process-agent to cancel POST request using context timeout
	ReqCtxTimeout = 30 * time.Second
)

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
