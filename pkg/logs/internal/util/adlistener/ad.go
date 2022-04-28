// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package adlistener

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
)

var (
	// There is only one ADListener instance, and this is it.  This is a short-
	// term fix for 7.36.0.
	instance *ADListener
)

// SetADMetaScheduler supplies this package with a reference to the AD MetaScheduler,
// once it has been started.  This occurs after the ADListener has been created.
func SetADMetaScheduler(sch *scheduler.MetaScheduler) {
	if instance == nil {
		panic("AD listener has not been created yet")
	}
	instance.adMetaScheduler = sch
	sch.Register(instance.name, instance)
}

// ADListener implements pkg/autodiscovery/scheduler/Scheduler.
//
// It proxies Schedule and Unschedule calls to its parent, and also handles
// delayed availability of the AD MetaScheduler.
//
// This must be a distinct type from schedulers, since both types implement
// interfaces with different Stop methods.
type ADListener struct {
	// name is the name of this listener
	name string

	// schedule and unschedule are the functions to which Schedule and
	// Unschedule calls should be proxied.
	schedule, unschedule func([]integration.Config)

	// adMetaScheduler is nil to begin with, and becomes non-nil after
	// SetADMetaScheduler is called.
	adMetaScheduler *scheduler.MetaScheduler
}

var _ scheduler.Scheduler = &ADListener{}

// NewADListener creates a new ADListener, proxying schedule and unschedule calls to
// the given functions.
func NewADListener(name string, schedule, unschedule func([]integration.Config)) *ADListener {
	l := &ADListener{
		name:       name,
		schedule:   schedule,
		unschedule: unschedule,
	}
	if instance != nil {
		panic("only one instance of ADListener can exist")
	}
	instance = l
	return l
}

// StartListener starts the ADListener.  It will subscribe to the MetaScheduler as soon
// as it is available
func (l *ADListener) StartListener() {
	// nothing to do; listener is started by SetADMetaScheduler
}

// StopListener stops the ADListener
func (l *ADListener) StopListener() {
	if l.adMetaScheduler != nil {
		l.adMetaScheduler.Deregister("logs")
	}
	instance = nil
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
