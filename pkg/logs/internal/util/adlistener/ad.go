// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package adlistener

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
)

var (
	// The AD MetaScheduler is not available until the logs-agent has started,
	// as part of the delicate balance of agent startup.  So, ADListener blocks
	// its startup until that occurs.
	//
	// The component architecture should remove the need for this workaround.

	// adMetaSchedulerCh carries the current MetaScheduler, once it is known.
	adMetaSchedulerCh chan *scheduler.MetaScheduler
)

func init() {
	adMetaSchedulerCh = make(chan *scheduler.MetaScheduler, 1)
}

// SetADMetaScheduler supplies this package with a reference to the AD MetaScheduler,
// once it has been started.
func SetADMetaScheduler(sch *scheduler.MetaScheduler) {
	// perform a non-blocking add to the channel
	select {
	case adMetaSchedulerCh <- sch:
	default:
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

	// registered is closed when the scheduler is registered (used for tests)
	registered chan struct{}

	// cancelRegister cancels efforts to register with the AD MetaScheduler
	cancelRegister context.CancelFunc
}

var _ scheduler.Scheduler = &ADListener{}

// NewADListener creates a new ADListener, proxying schedule and unschedule calls to
// the given functions.
func NewADListener(name string, schedule, unschedule func([]integration.Config)) *ADListener {
	return &ADListener{
		name:       name,
		schedule:   schedule,
		unschedule: unschedule,
		registered: make(chan struct{}),
	}
}

// StartListener starts the ADListener.  It will subscribe to the MetaScheduler as soon
// as it is available
func (l *ADListener) StartListener() {
	ctx, cancelRegister := context.WithCancel(context.Background())
	go func() {
		// wait for the scheduler to be set, and register once it is set
		select {
		case sch := <-adMetaSchedulerCh:
			l.adMetaScheduler = sch
			l.adMetaScheduler.Register(l.name, l)
			close(l.registered)
			// put the value back in the channel, in case it is needed again
			SetADMetaScheduler(sch)

		case <-ctx.Done():
		}
	}()

	l.cancelRegister = cancelRegister
}

// StopListener stops the ADListener
func (l *ADListener) StopListener() {
	l.cancelRegister()
	if l.adMetaScheduler != nil {
		l.adMetaScheduler.Deregister("logs")
	}
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
