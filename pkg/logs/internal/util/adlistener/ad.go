// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package adlistener

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
)

// ADListener implements pkg/autodiscovery/scheduler.Scheduler.
//
// It proxies Schedule and Unschedule calls to its parent.
//
// This must be a distinct type from schedulers, since both types implement
// interfaces with different Stop methods.
type ADListener struct {
	// name is the name of this listener
	name string

	// ac is the AutoConfig instance
	ac *autodiscovery.AutoConfig

	// schedule and unschedule are the functions to which Schedule and
	// Unschedule calls should be proxied.
	schedule, unschedule func([]integration.Config)
}

var _ scheduler.Scheduler = &ADListener{}

// NewADListener creates a new ADListener, proxying schedule and unschedule calls to
// the given functions.
func NewADListener(name string, ac *autodiscovery.AutoConfig, schedule, unschedule func([]integration.Config)) *ADListener {
	return &ADListener{
		name:       name,
		ac:         ac,
		schedule:   schedule,
		unschedule: unschedule,
	}
}

// StartListener starts the ADListener, subscribing to the feed of integration.Configs and
// additionally gathering any currently-scheduled configs.
func (l *ADListener) StartListener() {
	l.ac.AddScheduler(l.name, l, true)
}

// StopListener stops the ADListener
func (l *ADListener) StopListener() {
	l.ac.RemoveScheduler(l.name)
}

// Stop implements pkg/autodiscovery/scheduler.Scheduler#Stop.
func (l *ADListener) Stop() {}

// Schedule implements pkg/autodiscovery/scheduler.Scheduler#Schedule.
func (l *ADListener) Schedule(configs []integration.Config) {
	l.schedule(configs)
}

// Unschedule implements pkg/autodiscovery/scheduler.Scheduler#Unschedule.
func (l *ADListener) Unschedule(configs []integration.Config) {
	l.unschedule(configs)
}
