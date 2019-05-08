package backoff

import "time"

// Timer represents a timer that implements some backOff strategy that can adapt to number of schedulings.
type Timer interface {
	ScheduleRetry(err error) (int, time.Duration)
	CurrentDelay() time.Duration
	NumRetries() int
	ReceiveTick() <-chan time.Time
	Reset()
	Stop()
}

// DelayProvider is a function that takes the current numRetries and last error and returns the delay until next retry.
type DelayProvider func(numRetries int, err error) time.Duration

// CustomTimer represents a backoff timer configured with a certain DelayProvider.
type CustomTimer struct {
	numRetries   int
	currentDelay time.Duration

	delayProvider DelayProvider

	tickChannel chan time.Time
	timer       *time.Timer
}

// NewCustomTimer creates a new custom timer using the provided delay provider.
func NewCustomTimer(delayProvider DelayProvider) *CustomTimer {
	return &CustomTimer{
		delayProvider: delayProvider,
		tickChannel:   make(chan time.Time),
	}
}

// ScheduleRetry schedules the next retry tick according to the delay provider, returning retry num and retry delay.
func (t *CustomTimer) ScheduleRetry(err error) (int, time.Duration) {
	t.Stop()
	t.currentDelay = t.delayProvider(t.numRetries, err)

	t.timer = time.AfterFunc(t.currentDelay, func() {
		t.tickChannel <- time.Now()
	})

	t.numRetries++

	return t.numRetries, t.currentDelay
}

// CurrentDelay returns the delay of the current or last ticked retry.
func (t *CustomTimer) CurrentDelay() time.Duration {
	return t.currentDelay
}

// NumRetries returns the number of tries since this timer was last reset.
func (t *CustomTimer) NumRetries() int {
	return t.numRetries
}

// ReceiveTick returns a channel that will receive a time.Time object as soon as the previously scheduled retry ticks.
func (t *CustomTimer) ReceiveTick() <-chan time.Time {
	return t.tickChannel
}

// Reset stops and resets the number of retries counter of this timer.
func (t *CustomTimer) Reset() {
	t.Stop()
	t.numRetries = 0
	t.currentDelay = 0
}

// Stop prevents any current scheduled retry from ticking.
func (t *CustomTimer) Stop() {
	if t.timer != nil {
		t.timer.Stop()
	}
}

// Close cleans up the resources used by this timer. It cannot be reused after this call.
func (t *CustomTimer) Close() {
	t.Reset()
	close(t.tickChannel)
}
