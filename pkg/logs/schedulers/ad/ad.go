package ad

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	adScheduler "github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
)

var (
	// The AD MetaScheduler is not available until the logs-agent has started,
	// as part of the delicate balance of agent startup.  So, adListener blocks
	// its startup until that occurs.
	//
	// The component architecture should remove the need for this workaround.

	// adMetaSchedulerCh carries the current MetaScheduler, once it is known.
	adMetaSchedulerCh chan *adScheduler.MetaScheduler
)

func init() {
	adMetaSchedulerCh = make(chan *adScheduler.MetaScheduler, 1)
}

// SetADMetaScheduler supplies this package with a reference to the AD MetaScheduler,
// once it has been started.
func SetADMetaScheduler(sch *adScheduler.MetaScheduler) {
	// perform a non-blocking add to the channel
	select {
	case adMetaSchedulerCh <- sch:
	default:
	}
}

// adListener implements pkg/autodiscovery/scheduler/Scheduler.
//
// It proxies Schedule and Unschedule calls to its parent, and also handles
// delayed availability of the AD MetaScheduler.
//
// This must be a distinct type from Scheduler, since both types implement
// interfaces with different Stop methods.
type adListener struct {
	// schedule and unschedule are the functions to which Schedule and
	// Unschedule calls should be proxied.
	schedule, unschedule func([]integration.Config)

	// adMetaScheduler is nil to begin with, and becomes non-nil after
	// SetADMetaScheduler is called.
	adMetaScheduler *adScheduler.MetaScheduler

	// cancelRegister cancels efforts to register with the AD MetaScheduler
	cancelRegister context.CancelFunc
}

var _ adScheduler.Scheduler = &adListener{}

// newADListener creates a new ADListener, proxying schedule and unschedule calls to
// the given functions.
func newADListener(schedule, unschedule func([]integration.Config)) *adListener {
	return &adListener{schedule: schedule, unschedule: unschedule}
}

// start starts the adListener.  It will subscribe to the MetaScheduler as soon
// as it is available
func (l *adListener) start() {
	ctx, cancelRegister := context.WithCancel(context.Background())
	go func() {
		// wait for the scheduler to be set, and register once it is set
		select {
		case sch := <-adMetaSchedulerCh:
			l.adMetaScheduler = sch
			l.adMetaScheduler.Register("logs", l)
			// put the value back in the channel, in case it is needed again
			SetADMetaScheduler(sch)
		case <-ctx.Done():
		}
	}()

	l.cancelRegister = cancelRegister
}

// stop stops the adListener
func (l *adListener) stop() {
	l.cancelRegister()
	if l.adMetaScheduler != nil {
		l.adMetaScheduler.Deregister("logs")
	}
}

// Stop implements pkg/autodiscovery/scheduler.Scheduler#Stop.
func (l *adListener) Stop() {}

// Schedule implements pkg/autodiscovery/scheduler.Scheduler#Schedule.
func (l *adListener) Schedule(configs []integration.Config) {
	l.schedule(configs)
}

// Unschedule implements pkg/autodiscovery/scheduler.Scheduler#Unschedule.
func (l *adListener) Unschedule(configs []integration.Config) {
	l.unschedule(configs)
}
