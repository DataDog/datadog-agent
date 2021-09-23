package time

import (
	gotime "time"
)

func Sleep(d Duration) {
	if faker != nil {
		<-faker.after(d)
	} else {
		gotime.Sleep(d)
	}
}

// Interface satisfied by the real time.Timer struct
type Timer interface {
	Stop() bool
	Reset(Duration) bool
}

type fakeTimer struct {
	// stop is closed when the timer should stop
	stop chan struct{}

	// done is closed when the ticker's goroutine has stopped
	done chan struct{}

	// the ticker channel
	c chan Time

	// if not nil, a function that should be called when the timer is
	// triggered (used for AfterFunc)
	f func()

	// true if this timer stopped
	stopped bool
}

func newFakeTimer(d Duration) *fakeTimer {
	// The "real" time.Timer uses a buffered channel with one slot
	c := make(chan Time, 1)

	tmr := &fakeTimer{
		c:       c,
		stopped: false,
	}
	tmr.start(d)
	return tmr
}

func (tmr *fakeTimer) start(d Duration) {
	// use a fresh set of stop/done channels for each (re)start
	// of the timer
	tmr.stop = make(chan struct{})
	tmr.done = make(chan struct{})
	tmr.stopped = false
	go func() {
		after := faker.after(d)
		select {
		case <-tmr.stop:
			tmr.stopped = true
		case <-after:
			if tmr.f != nil {
				go (tmr.f)()
			} else {
				tmr.c <- faker.Now()
			}
		}
		close(tmr.done)
	}()
}

func (tmr *fakeTimer) Reset(d Duration) bool {
	stopped := tmr.Stop()
	tmr.start(d)
	// NOTE: docs say Reset's return value cannot be used correctly,
	// so its accuracy is not critical (and it is not tested)
	return stopped
}

func (tmr *fakeTimer) Stop() bool {
	if tmr.stop != nil {
		close(tmr.stop)
		<-tmr.done
		tmr.stop = nil
	}
	stopped := tmr.stopped
	tmr.stopped = false
	return stopped
}

func NewTimer(d Duration) (Timer, <-chan Time) {
	if faker != nil {
		tmr := newFakeTimer(d)
		return tmr, tmr.c
	} else {
		tmr := gotime.NewTimer(d)
		return tmr, tmr.C
	}
}

func After(d Duration) <-chan Time {
	_, c := NewTimer(d)
	return c
}

func AfterFunc(d Duration, f func()) (Timer, <-chan Time) {
	if faker != nil {
		tmr := newFakeTimer(d)
		tmr.f = f
		return tmr, tmr.c
	} else {
		tmr := gotime.AfterFunc(d, f)
		return tmr, tmr.C
	}
}
