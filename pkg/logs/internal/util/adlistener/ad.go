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
	// There is at most one ADListener adListener, and this is it.
	adListener *ADListener

	// This tracks the MetaScheduler, if one exists.
	metascheduler *scheduler.MetaScheduler
)

// SetADMetaScheduler supplies this package with a reference to the AD MetaScheduler,
// once it has been started.
func SetADMetaScheduler(sch *scheduler.MetaScheduler) {
	metascheduler = sch
	maybeStart()
}

// maybeStart might start the ADListener, if there is both an ADListener and
// a MetaScheduler.
//
// This is a short-term fix for 7.36.0.
func maybeStart() {
	if adListener != nil && metascheduler != nil {
		adListener.adMetaScheduler = metascheduler
		metascheduler.Register(adListener.name, adListener)
	}
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
	return l
}

// StartListener starts the ADListener.  It will subscribe to the MetaScheduler as soon
// as it is available
func (l *ADListener) StartListener() {
	if adListener != nil {
		panic("only one instance of ADListener can exist")
	}
	adListener = l

	// either start now, or wait until there is a MetaScheduler available.
	maybeStart()
}

// StopListener stops the ADListener
func (l *ADListener) StopListener() {
	if l.adMetaScheduler != nil {
		l.adMetaScheduler.Deregister("logs")
	}
	adListener = nil
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
