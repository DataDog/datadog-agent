// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package clientimpl holds the client to send data to the Cluster-Agent
package clientimpl

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	clientComp "github.com/DataDog/datadog-agent/comp/languagedetection/client"
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	"go.uber.org/fx"
)

const (
	// subscriber is the workloadmeta subscriber name
	subscriber = "language_detection_client"

	// defaultProcessWithoutPodTTL defines the TTL before a process event is expired in the ProcessWithoutPod map
	// if the associated pod is not found
	defaultProcessWithoutPodTTL = 5 * time.Minute

	// defaultprocessesWithoutPodCleanupPeriod defines the period to clean up process events from the map
	defaultprocessesWithoutPodCleanupPeriod = time.Hour
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newClient))
}

type dependencies struct {
	fx.In

	Lc           fx.Lifecycle
	Config       config.Component
	Log          logComponent.Component
	Telemetry    telemetry.Component
	Workloadmeta workloadmeta.Component

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
	store  workloadmeta.Component

	// mutex protecting UpdatedPodDetails and currentBatch
	mutex sync.Mutex

	// DCA Client
	langDetectionCl languageDetectionClient

	// telemetry
	telemetry *componentTelemetry

	// Current batch, populated by process events and cleaned by pod events
	currentBatch batch

	// The client must send freshly updated PodDetails as soon as possible however,
	// streaming every update to the cluster-agent could be costly. Thus we wait for
	// `freshDataPeriod` before sending fresh updates.
	freshDataPeriod    time.Duration
	freshlyUpdatedPods map[string]struct{}

	// There is a race between the process check and the kubelet. If the process check detects a language
	// before the kubelet pulls pods, the client should retry after waiting that workloadmeta pulled metadata
	// from the kubelet
	processesWithoutPod              map[string]*eventsToRetry
	processesWithoutPodTTL           time.Duration
	processesWithoutPodCleanupPeriod time.Duration

	// periodicalFlushPeriod sets the interval between two periodical flushes
	periodicalFlushPeriod time.Duration
}

// newClient creates a new Client
func newClient(
	deps dependencies,
) clientComp.Component {
	if !deps.Config.GetBool("language_detection.reporting.enabled") || !deps.Config.GetBool("language_detection.enabled") || !deps.Config.GetBool("cluster_agent.enabled") {
		return optional.NewNoneOption[clientComp.Component]()
	}

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	cl := &client{
		ctx:                              ctx,
		cancel:                           cancel,
		logger:                           deps.Log,
		store:                            deps.Workloadmeta,
		freshDataPeriod:                  deps.Config.GetDuration("language_detection.reporting.buffer_period"),
		mutex:                            sync.Mutex{},
		telemetry:                        newComponentTelemetry(deps.Telemetry),
		currentBatch:                     make(batch),
		processesWithoutPod:              make(map[string]*eventsToRetry),
		processesWithoutPodTTL:           defaultProcessWithoutPodTTL,
		processesWithoutPodCleanupPeriod: defaultprocessesWithoutPodCleanupPeriod,
		freshlyUpdatedPods:               make(map[string]struct{}),
		periodicalFlushPeriod:            deps.Config.GetDuration("language_detection.reporting.refresh_period"),
	}
	deps.Lc.Append(fx.Hook{
		OnStart: cl.start,
		OnStop:  cl.stop,
	})

	return optional.NewOption[clientComp.Component](cl)
}

// start starts streaming languages to the Cluster-Agent
func (c *client) start(_ context.Context) error {
	c.logger.Info("Starting language detection client")
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
	defer c.logger.Info("Shutting down language detection client")

	filterParams := workloadmeta.FilterParams{
		Kinds: []workloadmeta.Kind{
			workloadmeta.KindKubernetesPod, // Subscribe to pod events to clean up the current batch when a pod is deleted
			workloadmeta.KindProcess,       // Subscribe to process events to populate the current batch
		},
		Source:    workloadmeta.SourceAll,
		EventType: workloadmeta.EventTypeAll,
	}

	eventCh := c.store.Subscribe(
		subscriber,
		workloadmeta.NormalPriority,
		workloadmeta.NewFilter(&filterParams),
	)
	defer c.store.Unsubscribe(eventCh)

	cleanupProcessesWithoutPodCleanupTicker := time.NewTicker(c.processesWithoutPodCleanupPeriod)
	defer cleanupProcessesWithoutPodCleanupTicker.Stop()

	go c.startStreaming()

	for {
		select {
		case eventBundle, ok := <-eventCh:
			if !ok {
				return
			}
			c.handleEvent(eventBundle)
		case <-cleanupProcessesWithoutPodCleanupTicker.C:
			c.cleanUpProcesssesWithoutPod(time.Now())
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *client) cleanUpProcesssesWithoutPod(now time.Time) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for k, v := range c.processesWithoutPod {
		if v.expirationTime.Before(now) {
			delete(c.processesWithoutPod, k)
		}
	}
}

// handleEvent handles events from workloadmeta
func (c *client) handleEvent(evBundle workloadmeta.EventBundle) {
	evBundle.Acknowledge()
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
	freshUpdateTimer := time.NewTicker(c.freshDataPeriod)
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
			err := c.send(ctx, data)
			if err != nil {
				c.logger.Errorf("failed to send fresh update %v", err)
			}
		// less frequently, send the entire batch
		case <-periodicFlushTimer.C:
			data := c.getCurrentBatchProto()
			err := c.send(ctx, data)
			if err != nil {
				c.logger.Errorf("failed to send entire batch %v", err)
			}
		}
	}
}

// send sends the data to the cluster-agent. It doesn't implement a retry mechanism because if the dca is available
// then the data will eventually be transmitted by the periodic flush mechanism.
func (c *client) send(ctx context.Context, data *pbgo.ParentLanguageAnnotationRequest) error {
	if data == nil {
		return nil
	}
	if c.langDetectionCl == nil {
		// TODO: modify GetClusterAgentClient to accept a context with a deadline. If this
		// functions hangs forever, the component will be unhealthy and crash.
		dcaClient, err := clusteragent.GetClusterAgentClient()
		if err != nil {
			return err
		}
		c.langDetectionCl = dcaClient
	}
	t := time.Now()
	err := c.langDetectionCl.PostLanguageMetadata(ctx, data)
	if err != nil {
		c.telemetry.Requests.Inc(statusError)
		return err
	}
	c.telemetry.Latency.Observe(time.Since(t).Seconds())
	c.telemetry.Requests.Inc(statusSuccess)
	c.mutex.Lock()
	c.freshlyUpdatedPods = make(map[string]struct{})
	c.mutex.Unlock()
	return nil
}

// retryProcessEventsWithoutPod processes a second time process events for which the associated
// pod was not found because it is possible that the pod will be added to workloadmeta after the
// kubelet collector pulls data
func (c *client) retryProcessEventsWithoutPod(containerIDs []string) {
	for _, containerID := range containerIDs {
		eventsForContainer, ok := c.processesWithoutPod[containerID]
		if !ok {
			continue
		}
		for _, procEvent := range eventsForContainer.events {
			c.handleProcessEvent(procEvent, true)
		}
	}
}

// handleProcessEvent updates the current batch and the freshlyUpdatedPods
func (c *client) handleProcessEvent(processEvent workloadmeta.Event, isRetry bool) {
	if processEvent.Type != workloadmeta.EventTypeSet {
		return
	}

	process := processEvent.Entity.(*workloadmeta.Process)
	if process.Language == nil || process.Language.Name == "" {
		c.logger.Debugf("no language detected for process %s", process.ID)
		return
	}

	if process.ContainerID == "" {
		c.logger.Debugf("no container id detected for process %s", process.ID)
		return
	}

	pod, err := c.store.GetKubernetesPodForContainer(process.ContainerID)
	if err != nil {
		c.logger.Debugf("no pod found for process %s and containerID %s", process.ID, process.ContainerID)
		if !isRetry {
			c.telemetry.ProcessWithoutPod.Inc()
			evs, found := c.processesWithoutPod[process.ContainerID]
			if found {
				evs.events = append(evs.events, processEvent)
				return
			}
			c.processesWithoutPod[process.ContainerID] = &eventsToRetry{
				expirationTime: time.Now().Add(c.processesWithoutPodTTL),
				events:         []workloadmeta.Event{processEvent},
			}
		}
		return
	}

	if !podHasOwner(pod) {
		c.logger.Debugf("pod %s has no owner, skipping %s", pod.Name, process.ID)
		return
	}

	containerName, isInitcontainer, ok := getContainerInfoFromPod(process.ContainerID, pod)
	if !ok {
		c.logger.Debugf("container name not found for %s", process.ContainerID)
		return
	}

	podInfo := c.currentBatch.getOrAddPodInfo(pod.Name, pod.Namespace, &pod.Owners[0])
	containerInfo := podInfo.getOrAddContainerInfo(containerName, isInitcontainer)
	added := containerInfo.Add(langUtil.Language(process.Language.Name))
	if added {
		c.freshlyUpdatedPods[pod.Name] = struct{}{}
		delete(c.processesWithoutPod, process.ContainerID)
	}
	c.telemetry.ProcessedEvents.Inc(pod.Namespace, pod.Name, containerName, string(process.Language.Name))
}

// handlePodEvent removes delete pods from the current batch
func (c *client) handlePodEvent(podEvent workloadmeta.Event) {
	pod := podEvent.Entity.(*workloadmeta.KubernetesPod)
	containerIDs := make([]string, 0, len(pod.InitContainers)+len(pod.Containers))
	for _, c := range append(pod.InitContainers, pod.Containers...) {
		containerIDs = append(containerIDs, c.ID)
	}

	switch podEvent.Type {
	case workloadmeta.EventTypeSet:
		c.retryProcessEventsWithoutPod(containerIDs)
	case workloadmeta.EventTypeUnset:
		delete(c.currentBatch, pod.Name)
		delete(c.freshlyUpdatedPods, pod.Name)
		for _, cid := range containerIDs {
			delete(c.processesWithoutPod, cid)
		}
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

	if len(batch) > 0 {
		return batch.toProto()
	}

	return nil
}
