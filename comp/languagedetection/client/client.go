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
	runningMetricPeriod = 1 * time.Minute

	minBackoffFactor = 2.0
	baseBackoffTime  = 1.0 * time.Second
	recoveryInterval = 2 * time.Second
	maxError         = 10
	maxBackoffTime   = 30 * time.Second
)

type dependencies struct {
	fx.In

	Lc        fx.Lifecycle
	Config    config.Component
	Log       logComponent.Component
	telemetry telemetry.Component
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
	currentBatch    *batch
}

// newClient creates a new Client
func newClient(
	deps dependencies,
) Component {
	if !deps.Config.GetBool("language_detection.enabled") {
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
		telemetry:    newComponentTelemetry(deps.telemetry),
		currentBatch: newBatch(),
	}
	deps.Lc.Append(fx.Hook{
		OnStart: cl.start,
		OnStop:  cl.stop,
	})

	return cl
}

func (c *client) processEvent(evBundle workloadmeta.EventBundle) {
	close(evBundle.Ch)
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.logger.Tracef("Processing %d events", len(evBundle.Events))
	for _, event := range evBundle.Events {
		process := event.Entity.(*workloadmeta.Process)
		if process.Language == nil {
			continue
		}
		pod, err := c.store.GetKubernetesPodForContainer(process.ContainerID)
		if err != nil {
			c.logger.Debug("skipping language detection for process %s", process.ID)
			continue
		}
		if !podHasOwner(pod) {
			c.logger.Debug("pod %s has no owner, skipping %s", pod.Name, process.ID)
			continue
		}
		containerName, isInitcontainer, ok := getContainerInfoFromPod(process.ContainerID, pod)
		if !ok {
			c.logger.Debug("container name not found for %s", process.ContainerID)
			continue
		}
		podInfo := c.currentBatch.getOrAddPodInfo(pod.Name, pod.Namespace, &pod.Owners[0])
		containerInfo := podInfo.getOrAddcontainerInfo(containerName, isInitcontainer)
		containerInfo.add(string(process.Language.Name))
		c.telemetry.ProcessedEvents.Inc(pod.Name, containerName, string(process.Language.Name))
	}
}

func (c *client) stop(_ context.Context) error {
	c.cancel()
	return nil
}

// start starts streaming languages to the Cluster-Agent
func (c *client) start(_ context.Context) error {
	c.logger.Infof("Starting language detection client")
	go c.stream()
	return nil
}

// stream starts streaming languages to the Cluster-Agent
func (c *client) stream() {
	defer c.logger.Infof("Shutting down language detection client")
	processEventCh := c.store.Subscribe(
		subscriber,
		workloadmeta.NormalPriority,
		workloadmeta.NewFilter(
			[]workloadmeta.Kind{
				workloadmeta.KindProcess,
			},
			workloadmeta.SourceAll,
			workloadmeta.EventTypeSet,
		),
	)

	metricTicker := time.NewTicker(runningMetricPeriod)
	defer metricTicker.Stop()

	go c.startFlushing()

	for {
		select {
		case eventBundle := <-processEventCh:
			c.processEvent(eventBundle)
		case <-metricTicker.C:
			c.telemetry.Running.Set(1)
		case <-c.ctx.Done():
			return
		}
	}
}
