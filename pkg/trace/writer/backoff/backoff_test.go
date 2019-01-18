package backoff

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type SpecialError struct{}

func (*SpecialError) Error() string {
	return "this is a very special error"
}

func TestCustomTimer_ScheduleRetry(t *testing.T) {
	assert := assert.New(t)

	testDelay := 200 * time.Millisecond

	timer := NewCustomTimer(func(numRetries int, err error) time.Duration {
		if _, ok := err.(*SpecialError); ok {
			// If special error use fixed delay of 100 ms
			return 100 * time.Millisecond
		}

		// If normal error (or nil)
		return time.Duration(int64(1+numRetries) * int64(testDelay))
	})

	// First schedule (numRetries == 0)
	callTime := time.Now()
	timer.ScheduleRetry(nil)
	assert.Equal(testDelay, timer.CurrentDelay(), "Timer should report correct retry delay")

	select {
	case tickTime := <-timer.ReceiveTick():
		assert.WithinDuration(tickTime, callTime, time.Duration(1.5*float64(testDelay)),
			"Tick time and call time should be within expected delay of each other (with a small margin)")
	case <-time.After(1 * time.Second):
		assert.Fail("Received no tick within 500ms")
	}

	// Second schedule (numRetries == 1)
	callTime = time.Now()
	timer.ScheduleRetry(nil)
	assert.Equal(time.Duration(2*testDelay), timer.CurrentDelay(), "Timer should report correct retry delay")

	select {
	case tickTime := <-timer.ReceiveTick():
		assert.WithinDuration(tickTime, callTime, time.Duration(2.5*float64(testDelay)),
			"Tick time and call time should be within expected delay of each other (with a small margin)")
	case <-time.After(1 * time.Second):
		assert.Fail("Received no tick within 500ms")
	}

	// Third schedule (numRetries == 2 but error is SpecialError)
	callTime = time.Now()
	timer.ScheduleRetry(&SpecialError{})
	assert.Equal(100*time.Millisecond, timer.CurrentDelay(), "Timer should report correct retry delay")

	select {
	case tickTime := <-timer.ReceiveTick():
		assert.WithinDuration(tickTime, callTime, time.Duration(200*time.Millisecond),
			"Tick time and call time should be within expected delay of each other (with a small margin)")
	case <-time.After(1 * time.Second):
		assert.Fail("Received no tick within 500ms")
	}

	timer.Close()
}

func TestCustomTimer_StopNotTicked(t *testing.T) {
	assert := assert.New(t)

	testDelay := 100 * time.Millisecond

	timer := NewCustomTimer(func(_ int, _ error) time.Duration { return testDelay })

	timer.ScheduleRetry(nil)
	timer.Stop()

	select {
	case <-timer.ReceiveTick():
		assert.Fail("Shouldn't have received tick because timer was stopped")
	case <-time.After(2 * testDelay):
		assert.True(true, "Should end without receiving anything")
	}

	assert.Equal(1, timer.NumRetries(), "Stopping the timer should not have reset it")
	assert.Equal(testDelay, timer.CurrentDelay(), "Stopping the timer should not have reset it")

	timer.Close()
}

func TestCustomTimer_Reset(t *testing.T) {
	assert := assert.New(t)

	testDelay := 100 * time.Millisecond

	timer := NewCustomTimer(func(_ int, _ error) time.Duration { return testDelay })

	timer.ScheduleRetry(nil)
	timer.Reset()

	select {
	case <-timer.ReceiveTick():
		assert.Fail("Shouldn't have received tick because resetting a timer should also stop it")
	case <-time.After(2 * testDelay):
		assert.True(true, "Should end without receiving anything")
	}

	assert.Equal(0, timer.NumRetries(), "Timer should have been reset")
	assert.Equal(0*time.Millisecond, timer.CurrentDelay(), "Timer should have been reset")

	timer.Close()
}
