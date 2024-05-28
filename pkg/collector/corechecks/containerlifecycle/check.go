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

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ddConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
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
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if !ddConfig.Datadog.GetBool("container_lifecycle.enabled") {
		return errors.New("collection of container lifecycle events is disabled")
	}

	var err error

	err = c.CommonConfigure(senderManager, initConfig, config, source)
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

	return nil
}

// Run starts the container_lifecycle check
func (c *Check) Run() error {
	log.Infof("Starting long-running check %q", c.ID())
	defer log.Infof("Shutting down long-running check %q", c.ID())

	containerFilterParams := workloadmeta.FilterParams{
		Kinds:     []workloadmeta.Kind{workloadmeta.KindContainer},
		Source:    workloadmeta.SourceRuntime,
		EventType: workloadmeta.EventTypeUnset,
	}
	contEventsCh := c.workloadmetaStore.Subscribe(
		CheckName+"-cont",
		workloadmeta.NormalPriority,
		workloadmeta.NewFilter(&containerFilterParams),
	)

	podFilterParams := workloadmeta.FilterParams{
		Kinds:     []workloadmeta.Kind{workloadmeta.KindKubernetesPod},
		Source:    workloadmeta.SourceNodeOrchestrator,
		EventType: workloadmeta.EventTypeUnset,
	}
	podEventsCh := c.workloadmetaStore.Subscribe(
		CheckName+"-pod",
		workloadmeta.NormalPriority,
		workloadmeta.NewFilter(&podFilterParams),
	)

	var taskEventsCh chan workloadmeta.EventBundle
	if ddConfig.Datadog.GetBool("container_lifecycle.ecs_task_event.enabled") {
		taskFilterParams := workloadmeta.FilterParams{
			Kinds:     []workloadmeta.Kind{workloadmeta.KindECSTask},
			Source:    workloadmeta.SourceNodeOrchestrator,
			EventType: workloadmeta.EventTypeUnset,
		}
		taskEventsCh = c.workloadmetaStore.Subscribe(
			CheckName+"-task",
			workloadmeta.NormalPriority,
			workloadmeta.NewFilter(&taskFilterParams),
		)
	}

	pollInterval := time.Duration(c.instance.PollInterval) * time.Second

	processorCtx, stopProcessor := context.WithCancel(context.Background())
	c.processor.start(processorCtx, pollInterval)

	defer func() {
		c.sendFargateTaskEvent()
		stopProcessor()
	}()
	for {
		select {
		case eventBundle, ok := <-contEventsCh:
			if !ok {
				return nil
			}
			c.processor.processEvents(eventBundle)
		case eventBundle, ok := <-podEventsCh:
			if !ok {
				stopProcessor()
				return nil
			}
			c.processor.processEvents(eventBundle)
		case eventBundle, ok := <-taskEventsCh:
			if !ok {
				stopProcessor()
				return nil
			}
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
func Factory(store workloadmeta.Component) optional.Option[func() check.Check] {
	return optional.NewOption(func() check.Check {
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
	if !ddConfig.Datadog.GetBool("container_lifecycle.ecs_task_event.enabled") ||
		!ddConfig.IsECSFargate() {
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
