// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package client holds the client to send data to the Cluster-Agent
package client

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"go.uber.org/fx"
)

const (
	// subscriber is the workloadmeta subscriber name
	subscriber = "language_detection_client"

	// runningMetricPeriod emits the `running` metric every 15 minutes
	runningMetricPeriod = 15 * time.Minute

	// periodicalFlushPeriod parametrizes when the current batch needs to be entirely sent
	periodicalFlushPeriod = 100 * time.Minute
)

type dependencies struct {
	fx.In

	Lc        fx.Lifecycle
	Config    config.Component
	Log       logComponent.Component
	Telemetry telemetry.Component

	// workloadmeta is still not a component but should be provided as one in the future
	// TODO(components): Workloadmeta workloadmeta.Component
}

// languageDetectionClient defines the method to send a message to the Cluster-Agent
type languageDetectionClient interface {
	PostLanguageMetadata(ctx context.Context, data *pbgo.ParentLanguageAnnotationRequest) error
}

// client sends language information to the Cluster-Agent
type client struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger logComponent.Component
	store  workloadmeta.Store

	// mutex protecting UpdatedPodDetails and currentBatch
	mutex sync.Mutex

	// DCA Client
	langDetectionCl languageDetectionClient

	// telemetry
	telemetry *componentTelemetry

	// Current batch, populated by process events and cleaned by pod events
	currentBatch batch

	// The client must send freshly updated PodDetails as soon as possible however,
	// waiting `newUpdatePeriod` allows to wait until every process of a pod is scanned,
	// limiting the number of messages that need to be sent
	newUpdatePeriod    time.Duration
	freshlyUpdatedPods map[string]struct{}

	// There is a race between the process check and the kubelet. If the process check detects a language
	// before the kubelet pulls pods, the client should retry after waiting that workloadmeta pulled metadata
	// from the kubelet
	processesWithoutPod          []workloadmeta.Event
	retryProcessWithoutPodPeriod time.Duration

	// periodicalFlushPeriod sets the interval between two periodical flushes
	periodicalFlushPeriod time.Duration
}

// newClient creates a new Client
func newClient(
	deps dependencies,
) Component {
	if !deps.Config.GetBool("language_detection.enabled") || !deps.Config.GetBool("cluster_agent.enabled") {
		return util.NewNoneOptional[Component]()
	}

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	cl := &client{
		ctx:                          ctx,
		cancel:                       cancel,
		logger:                       deps.Log,
		newUpdatePeriod:              deps.Config.GetDuration("language_detection.client_period"),
		mutex:                        sync.Mutex{},
		telemetry:                    newComponentTelemetry(deps.Telemetry),
		currentBatch:                 make(batch),
		processesWithoutPod:          make([]workloadmeta.Event, 0),
		retryProcessWithoutPodPeriod: deps.Config.GetDuration("kubelet_cache_pods_duration") * time.Second,
		freshlyUpdatedPods:           make(map[string]struct{}),
		periodicalFlushPeriod:        periodicalFlushPeriod,
	}
	deps.Lc.Append(fx.Hook{
		OnStart: cl.start,
		OnStop:  cl.stop,
	})

	return util.NewOptional[Component](cl)
}

// start starts streaming languages to the Cluster-Agent
func (c *client) start(_ context.Context) error {
	c.logger.Infof("Starting language detection client")
	go c.run()
	return nil
}

// stop stops the client
func (c *client) stop(_ context.Context) error {
	c.cancel()
	return nil
}

// run starts processing events and starts streaming
func (c *client) run() {
	defer c.logger.Infof("Shutting down language detection client")
	// workloadmeta can't be initialized in the constructor or provided as a dependency until workloadmeta is refactored as a component
	if c.store == nil {
		c.store = workloadmeta.GetGlobalStore() // TODO(components): should be replaced by components
	}

	eventCh := c.store.Subscribe(
		subscriber,
		workloadmeta.NormalPriority,
		workloadmeta.NewFilter(
			[]workloadmeta.Kind{
				workloadmeta.KindKubernetesPod, // Subscribe to pod events to clean up the current batch when a pod is deleted
				workloadmeta.KindProcess,       // Subscribe to process events to populate the current batch
			},
			workloadmeta.SourceAll,
			workloadmeta.EventTypeAll,
		),
	)
	defer c.store.Unsubscribe(eventCh)

	metricTicker := time.NewTicker(runningMetricPeriod)
	defer metricTicker.Stop()

	retryProcessWithoutPodTicker := time.NewTicker(c.retryProcessWithoutPodPeriod)
	defer retryProcessWithoutPodTicker.Stop()

	go c.startStreaming()

	for {
		select {
		case eventBundle := <-eventCh:
			c.processEvent(eventBundle)
		case <-retryProcessWithoutPodTicker.C:
			c.retryProcessEventsWithoutPod()
		case <-c.ctx.Done():
			return
		}
	}
}

// processEvent processes events from workloadmeta
func (c *client) processEvent(evBundle workloadmeta.EventBundle) {
	close(evBundle.Ch)
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.logger.Tracef("Processing %d events", len(evBundle.Events))
	for _, event := range evBundle.Events {
		switch event.Entity.GetID().Kind {
		case workloadmeta.KindProcess:
			c.handleProcessEvent(event, false)
		case workloadmeta.KindKubernetesPod:
			c.handlePodEvent(event)
		}

	}
}

// startStreaming retrieves the language detection client (= the DCA Client) and periodically sends data to the Cluster-Agent
func (c *client) startStreaming() {
	freshUpdateTimer := time.NewTicker(c.newUpdatePeriod)
	defer freshUpdateTimer.Stop()

	periodicFlushTimer := time.NewTicker(c.periodicalFlushPeriod)
	defer periodicFlushTimer.Stop()

	health := health.RegisterLiveness("process-language-detection-client-sender")

	ctx, cancel := context.WithCancel(c.ctx)
	for {
		select {
		case <-c.ctx.Done():
			cancel()
			err := health.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}
			return
		case healthDeadline := <-health.C:
			cancel()
			ctx, cancel = context.WithDeadline(c.ctx, healthDeadline)
		// frequently send only fresh updates
		case <-freshUpdateTimer.C:
			data := c.getFreshBatchProto()
			c.send(ctx, data)
		// less frequently, send the entire batch
		case <-periodicFlushTimer.C:
			data := c.getCurrentBatchProto()
			c.send(ctx, data)
		}
	}
}

// send sends the data to the cluster-agent. It doesn't implement a retry mechanism because if the dca is available
// then the data will eventually be transmitted by the periodic flush mechanism.
func (c *client) send(ctx context.Context, data *pbgo.ParentLanguageAnnotationRequest) {
	if data == nil {
		return
	}
	if c.langDetectionCl == nil {
		// TODO: modify GetClusterAgentClient to accept a context with a deadline. If this
		// functions hangs forever, the component will be unhealthy and crash.
		dcaClient, err := clusteragent.GetClusterAgentClient()
		if err != nil {
			c.logger.Debugf("failed to get dca client %s, not sending batch", err)
			return
		}
		c.langDetectionCl = dcaClient
	}
	t := time.Now()
	err := c.langDetectionCl.PostLanguageMetadata(ctx, data)
	if err != nil {
		c.logger.Errorf("failed to post language metadata %v", err)
		c.telemetry.Requests.Inc(statusError)
		return
	}
	c.telemetry.Latency.Observe(time.Since(t).Seconds())
	c.telemetry.Requests.Inc(statusSuccess)
}

// retryProcessEventsWithoutPod processes a second time process events for which the associated
// pod was not found because it is possible that the pod will be added to workloadmeta after the
// kubelet collector pulls data
func (c *client) retryProcessEventsWithoutPod() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, event := range c.processesWithoutPod {
		c.handleProcessEvent(event, true)
	}
	c.processesWithoutPod = make([]workloadmeta.Event, 0)
}

// handleProcessEvent updates the current batch and the freshlyUpdatedPods
func (c *client) handleProcessEvent(processEvent workloadmeta.Event, isRetry bool) {
	if processEvent.Type != workloadmeta.EventTypeSet {
		return
	}

	process := processEvent.Entity.(*workloadmeta.Process)
	if process.Language == nil {
		return
	}

	pod, err := c.store.GetKubernetesPodForContainer(process.ContainerID)
	if err != nil {
		c.logger.Debug("skipping language detection for process %s, will retry in %v", process.ID, periodicalFlushPeriod)
		if !isRetry {
			c.telemetry.ProcessWithoutPod.Inc()
			c.processesWithoutPod = append(c.processesWithoutPod, processEvent)
		}
		return
	}
	if !podHasOwner(pod) {
		c.logger.Debug("pod %s has no owner, skipping %s", pod.Name, process.ID)
		return
	}
	containerName, isInitcontainer, ok := getContainerInfoFromPod(process.ContainerID, pod)
	if !ok {
		c.logger.Debug("container name not found for %s", process.ContainerID)
		return
	}
	podInfo := c.currentBatch.getOrAddPodInfo(pod.Name, pod.Namespace, &pod.Owners[0])
	containerInfo := podInfo.getOrAddContainerInfo(containerName, isInitcontainer)
	added := containerInfo.Add(string(process.Language.Name))
	if added {
		c.freshlyUpdatedPods[pod.Name] = struct{}{}
	}
	c.telemetry.ProcessedEvents.Inc(pod.Name, containerName, string(process.Language.Name))
}

// handlePodEvent removes delete pods from the current batch
func (c *client) handlePodEvent(podEvent workloadmeta.Event) {
	if podEvent.Type == workloadmeta.EventTypeUnset {
		pod := podEvent.Entity.(*workloadmeta.KubernetesPod)
		delete(c.currentBatch, pod.Name)
		delete(c.freshlyUpdatedPods, pod.Name)
	}
}

func (c *client) getCurrentBatchProto() *pbgo.ParentLanguageAnnotationRequest {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if len(c.currentBatch) == 0 {
		return nil
	}
	return c.currentBatch.toProto()
}

// getFreshBatch returns a batch with only freshly updated pods
func (c *client) getFreshBatchProto() *pbgo.ParentLanguageAnnotationRequest {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	batch := make(batch)

	for podName := range c.freshlyUpdatedPods {
		if containerInfo, ok := c.currentBatch[podName]; ok {
			batch[podName] = containerInfo
		}
	}

	c.freshlyUpdatedPods = make(map[string]struct{}, 0)

	if len(batch) > 0 {
		return batch.toProto()
	}

	return nil
}
