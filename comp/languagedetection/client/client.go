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
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"go.uber.org/fx"
)

const (
	subscriber          = "language_detection_client"
	runningMetricPeriod = 15 * time.Minute
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

// client sends language information to the Cluster-Agent
type client struct {
	ctx             context.Context
	cancel          context.CancelFunc
	logger          logComponent.Component
	flushPeriod     time.Duration
	store           workloadmeta.Store
	mutex           sync.Mutex
	langDetectionCl clusteragent.LanguageDetectionClient
	telemetry       *componentTelemetry
	currentBatch    batch
	// there is a race between the process check and the kubelet. In that case we
	// want to retry after waiting that workloadmeta pulled metadata from the kubelet
	processesWithoutPod []workloadmeta.Event
	retryProcessWithoutPodPeriod time.Duration
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
		ctx:          ctx,
		cancel:       cancel,
		logger:       deps.Log,
		flushPeriod:  deps.Config.GetDuration("language_detection.client_period"),
		mutex:        sync.Mutex{},
		telemetry:    newComponentTelemetry(deps.Telemetry),
		currentBatch: make(batch),
		processesWithoutPod: make([]workloadmeta.Event, 0),
		retryProcessWithoutPodsPeriod: deps.Config.GetInt("kubelet_listener_polling_interval") * time.Second,
	}
	deps.Lc.Append(fx.Hook{
		OnStart: cl.start,
		OnStop:  cl.stop,
	})

	return util.NewOptional[Component](cl)
}

func (c *client) retryProcessEventsWithoutPod()() {
	for _, event := c.processesWithoutPod {
		c.processProcessEvent(event)
	}
	c.processesWithoutPod = make([]workloadmeta.Event, 0)
}

func (c *client) processProcessEvent(processEvent workloadmeta.Event) {
	if processEvent.Type != workloadmeta.EventTypeSet {
		return
	}

	process := processEvent.Entity.(*workloadmeta.Process)
	if process.Language == nil {
		return
	}

	pod, err := c.store.GetKubernetesPodForContainer(process.ContainerID)
	if err != nil {
		c.logger.Debug("skipping language detection for process %s, will retry in %v", process.ID, retryPeriod)
		c.processesWithoutPod = append(c.processesWithoutPod, processEvent)
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
	containerInfo.add(string(process.Language.Name))
	c.telemetry.ProcessedEvents.Inc(pod.Name, containerName, string(process.Language.Name))
}

func (c *client) processPodEvent(podEvent workloadmeta.Event) {
	if podEvent.Type == workloadmeta.EventTypeUnset {
		pod := podEvent.Entity.(*workloadmeta.KubernetesPod)
		delete(c.currentBatch, pod.Name)
	}
}

func (c *client) processEvent(evBundle workloadmeta.EventBundle) {
	close(evBundle.Ch)
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.logger.Tracef("Processing %d events", len(evBundle.Events))
	for _, event := range evBundle.Events {
		switch event.Entity.GetID().Kind {
		case workloadmeta.KindProcess:
			c.processProcessEvent(event)
		case workloadmeta.KindKubernetesPod:
			c.processPodEvent(event)
		}

	}
}

func (c *client) stop(_ context.Context) error {
	c.cancel()
	return nil
}

// start starts streaming languages to the Cluster-Agent
func (c *client) start(_ context.Context) error {
	c.logger.Infof("Starting language detection client")
	go c.run()
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

	metricTicker := time.NewTicker(runningMetricPeriod)
	defer metricTicker.Stop()

	retryProcessWithoutPodTicker := time.NewTicker(c.retryProcessWithoutPodPeriod)
	defer retryProcessWithoutPodTicker.Stop()

	go c.startStreaming()

	for {
		select {
		case eventBundle := <-eventCh:
			c.processEvent(eventBundle)
		case retryProcessWithoutPodTicker.C:
			c.retryProcessEventsWithoutPod()
		case <-metricTicker.C:
			c.telemetry.Running.Set(1)
		case <-c.ctx.Done():
			return
		}
	}
}

// startStreaming retrieves the language detection client (= the DCA Client) and periodically sends data to the Cluster-Agent
func (c *client) startStreaming() {
	periodicFlushTimer := time.NewTicker(c.flushPeriod)
	defer periodicFlushTimer.Stop()

	if c.langDetectionCl == nil {
		// TODO(components): The ClusterAgentClient should most likely be a component. Moreover it should provide a retry mechanism or at least, the duration before the next try.
		// Since currently we never retry `GetClusterAgentClient` in other parts of the code, we choose to follow the same pattern.
		dcaClient, err := clusteragent.GetClusterAgentClient()
		if err != nil {
			c.logger.Errorf("failed to get dca client %s, stopping language exporter", err)
			c.cancel()
			return
		}
		c.langDetectionCl = dcaClient
	}

	for {
		select {
		case <-periodicFlushTimer.C:
			c.flush()
		case <-c.ctx.Done():
			return
		}
	}
}

// flush sends the current batch to the cluster-agent
func (c *client) flush() {
	// To avoid blocking the loop processing events for too long, we retrieve the current batch and release the mutex. On failures, items are added back to the current batch.
	var data batch
	c.mutex.Lock()
	if len(c.currentBatch) > 0 {
		data = c.currentBatch
		c.currentBatch = make(batch)
	}
	c.mutex.Unlock()
	// if no data was found
	if data == nil {
		return
	}

	t := time.Now()
	err := c.langDetectionCl.PostLanguageMetadata(c.ctx, data.toProto())
	if err != nil {
		c.logger.Errorf("failed to post language metadata %v", err)
		c.mergeBatchAfterError(data)
		c.telemetry.Requests.Inc(statusError)
		return
	}
	c.telemetry.Latency.Observe(time.Since(t).Seconds())
	c.telemetry.Requests.Inc(statusSuccess)
}

func (c *client) mergeBatchAfterError(b batch) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.currentBatch.merge(b)
}
