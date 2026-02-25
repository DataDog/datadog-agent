// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package podlifecycle implements the pod lifecycle check.
package podlifecycle

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "pod_lifecycle"

	defaultCommitInterval = 10 * time.Second
)

// Check reports pod startup duration metrics.
type Check struct {
	core.CheckBase
	workloadmetaStore workloadmeta.Component
	tagger            tagger.Component
	processor         *processor
	stopCh            chan struct{}
}

// Configure initializes the pod_lifecycle check.
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}

	s, err := c.GetSender()
	if err != nil {
		return err
	}

	c.processor = newProcessor(s, c.tagger)
	return nil
}

// Run starts the pod_lifecycle check.
func (c *Check) Run() error {
	log.Infof("Starting long-running check %q", c.ID())
	defer log.Infof("Shutting down long-running check %q", c.ID())

	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceNodeOrchestrator).
		AddKind(workloadmeta.KindKubernetesPod).
		Build()

	podEventsCh := c.workloadmetaStore.Subscribe(CheckName, workloadmeta.NormalPriority, filter)
	defer c.workloadmetaStore.Unsubscribe(podEventsCh)

	ctx, stopCommitter := context.WithCancel(context.Background())
	go c.processor.start(ctx, defaultCommitInterval)
	defer stopCommitter()

	for {
		select {
		case evBundle, ok := <-podEventsCh:
			if !ok {
				return nil
			}
			c.processor.processEvents(evBundle)
		case <-c.stopCh:
			return nil
		}
	}
}

// Cancel stops the pod_lifecycle check.
func (c *Check) Cancel() { close(c.stopCh) }

// Interval returns 0, making pod_lifecycle a long-running check.
func (c *Check) Interval() time.Duration { return 0 }

// Factory returns a new check factory.
func Factory(store workloadmeta.Component, taggerComp tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return core.NewLongRunningCheckWrapper(&Check{
			CheckBase:         core.NewCheckBase(CheckName),
			workloadmetaStore: store,
			tagger:            taggerComp,
			stopCh:            make(chan struct{}),
		})
	})
}
