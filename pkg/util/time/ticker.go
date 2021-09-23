package time

import (
	gotime "time"
)

// Ticker is an interface satisfied by the "real" time.Ticker
type Ticker interface {
	Reset(Duration)
	Stop()
}

type fakeTicker struct {
	// reset sends a duration on this channel; stop closes it
	reset chan<- Duration

	// done is closed when the ticker's goroutine has stopped
	done <-chan struct{}

	// the ticker channel
	c <-chan Time
}

func newFakeTicker(d Duration) *fakeTicker {
	reset := make(chan Duration)
	done := make(chan struct{})

	// The "real" time.Ticker uses a buffered channel with one slot
	c := make(chan Time, 1)

	tkr := &fakeTicker{reset, done, c}
	tkr.start(d, reset, done, c)

	return tkr
}

func (tkr *fakeTicker) start(
	d Duration,
	reset <-chan Duration,
	done chan<- struct{},
	c chan<- Time,
) {
	go func() {
		nextTick := faker.after(d)
		for {
			select {
			case dur, ok := <-reset:
				// reset channel was closed, signalling this
				// goroutine to exit
				if !ok {
					close(done)
					return
				}
				// we got reset to a new duration, so update
				// the stored duration value and restart the
				// timer
				d = dur
				nextTick = faker.after(d)
			case <-nextTick:
				nextTick = faker.after(d)
				// the "real" time.Ticker uses a non-blocking send, so
				// do the same here
				select {
				case c <- faker.Now():
				default:
				}
			}
		}
	}()
}

func (tkr *fakeTicker) Reset(d Duration) {
	tkr.reset <- d
}

func (tkr *fakeTicker) Stop() {
	close(tkr.reset)
	<-tkr.done
}

func Tick(d Duration) <-chan Time {
	if d <= 0 {
		return nil
	}
	_, c := NewTicker(d)
	return c
}

func NewTicker(d Duration) (Ticker, <-chan Time) {
	if faker != nil {
		tkr := newFakeTicker(d)
		return tkr, tkr.c
	} else {
		tkr := gotime.NewTicker(d)
		return tkr, tkr.C
	}
}
