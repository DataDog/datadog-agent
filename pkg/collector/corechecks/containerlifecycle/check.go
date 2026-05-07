// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containerlifecycle implements the container lifecycle check.
package containerlifecycle

import (
	"context"
	"errors"
	"time"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName           = "container_lifecycle"
	maxChunkSize        = 100
	defaultPollInterval = 10
)

// Config holds the container_lifecycle check configuration
type Config struct {
	ChunkSize    int `yaml:"chunk_size"`
	PollInterval int `yaml:"poll_interval_seconds"`
}

// Parse parses the container_lifecycle check config and set default values
func (c *Config) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// Check reports container lifecycle events
type Check struct {
	core.CheckBase
	workloadmetaStore workloadmeta.Component
	instance          *Config
	processor         *processor
	stopCh            chan struct{}
}

// Configure parses the check configuration and initializes the container_lifecycle check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string, provider string) error {
	if !pkgconfigsetup.Datadog().GetBool("container_lifecycle.enabled") {
		log.Debugf("container_lifecycle check is disabled via container_lifecycle.enabled=false")
		return errors.New("collection of container lifecycle events is disabled")
	}
	log.Debugf("Configuring container_lifecycle check (source=%s, provider=%s)", source, provider)

	var err error

	err = c.CommonConfigure(senderManager, initConfig, config, source, provider)
	if err != nil {
		return err
	}

	err = c.instance.Parse(config)
	if err != nil {
		return err
	}

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	if c.instance.ChunkSize <= 0 || c.instance.ChunkSize > maxChunkSize {
		c.instance.ChunkSize = maxChunkSize
	}

	if c.instance.PollInterval <= 0 {
		c.instance.PollInterval = defaultPollInterval
	}

	c.processor = newProcessor(sender, c.instance.ChunkSize, c.workloadmetaStore)
	log.Debugf("container_lifecycle check configured with chunk_size=%d, poll_interval=%ds", c.instance.ChunkSize, c.instance.PollInterval)

	return nil
}

// Run starts the container_lifecycle check
func (c *Check) Run() error {
	log.Infof("Starting long-running check %q", c.ID())
	defer log.Infof("Shutting down long-running check %q", c.ID())

	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceRuntime).
		SetEventType(workloadmeta.EventTypeUnset).
		AddKind(workloadmeta.KindContainer).
		Build()

	contEventsCh := c.workloadmetaStore.Subscribe(
		CheckName+"-cont",
		workloadmeta.NormalPriority,
		filter,
	)

	podFilter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceNodeOrchestrator).
		AddKind(workloadmeta.KindKubernetesPod).
		Build()

	podEventsCh := c.workloadmetaStore.Subscribe(
		CheckName+"-pod",
		workloadmeta.NormalPriority,
		podFilter,
	)

	log.Debugf("container_lifecycle check subscribed to workloadmeta for containers and pods")

	var taskEventsCh chan workloadmeta.EventBundle
	if pkgconfigsetup.Datadog().GetBool("ecs_task_collection_enabled") {
		log.Debugf("ecs_task_collection_enabled=true, subscribing to ECS task events")

		taskFilter := workloadmeta.NewFilterBuilder().
			SetSource(workloadmeta.SourceNodeOrchestrator).
			SetEventType(workloadmeta.EventTypeUnset).
			AddKind(workloadmeta.KindECSTask).
			Build()

		taskEventsCh = c.workloadmetaStore.Subscribe(
			CheckName+"-task",
			workloadmeta.NormalPriority,
			taskFilter,
		)
	}

	pollInterval := time.Duration(c.instance.PollInterval) * time.Second

	processorCtx, stopProcessor := context.WithCancel(context.Background())
	c.processor.start(processorCtx, pollInterval)
	log.Debugf("container_lifecycle processor started with pollInterval=%s", pollInterval)

	defer func() {
		c.sendFargateTaskEvent()
		stopProcessor()
	}()
	for {
		select {
		case eventBundle, ok := <-contEventsCh:
			if !ok {
				log.Debugf("container_lifecycle: container events channel closed")
				return nil
			}
			log.Debugf("container_lifecycle: received container event bundle with %d events", len(eventBundle.Events))
			c.processor.processEvents(eventBundle)
		case eventBundle, ok := <-podEventsCh:
			if !ok {
				log.Debugf("container_lifecycle: pod events channel closed")
				stopProcessor()
				return nil
			}
			log.Debugf("container_lifecycle: received pod event bundle with %d events", len(eventBundle.Events))
			c.processor.processEvents(eventBundle)
		case eventBundle, ok := <-taskEventsCh:
			if !ok {
				log.Debugf("container_lifecycle: task events channel closed")
				stopProcessor()
				return nil
			}
			log.Debugf("container_lifecycle: received task event bundle with %d events", len(eventBundle.Events))
			c.processor.processEvents(eventBundle)
		case <-c.stopCh:
			return nil
		}
	}
}

// Cancel stops the container_lifecycle check
func (c *Check) Cancel() { close(c.stopCh) }

// Interval returns 0, it makes container_lifecycle a long-running check
func (c *Check) Interval() time.Duration { return 0 }

// Factory returns a new check factory
func Factory(store workloadmeta.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return core.NewLongRunningCheckWrapper(&Check{
			CheckBase:         core.NewCheckBase(CheckName),
			workloadmetaStore: store,
			instance:          &Config{},
			stopCh:            make(chan struct{}),
		})
	})
}

// sendFargateTaskEvent sends Fargate task lifecycle event at the end of the check
func (c *Check) sendFargateTaskEvent() {
	if !pkgconfigsetup.Datadog().GetBool("ecs_task_collection_enabled") ||
		!env.IsECSSidecarMode(pkgconfigsetup.Datadog()) {
		return
	}

	tasks := c.workloadmetaStore.ListECSTasks()
	if len(tasks) != 1 {
		log.Infof("Unable to send Fargate task lifecycle event, expected 1 task, got %d", len(tasks))
		return
	}

	log.Infof("Send fargate task lifecycle event, task arn:%s", tasks[0].EntityID.ID)
	c.processor.processEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type:   workloadmeta.EventTypeUnset,
				Entity: tasks[0],
			},
		},
		Ch: make(chan struct{}),
	})
}
