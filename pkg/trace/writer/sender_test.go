package writer

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/writer/backoff"
	writerconfig "github.com/DataDog/datadog-agent/pkg/trace/writer/config"
	"github.com/stretchr/testify/assert"
)

func TestNewPayloadSetsCreationDate(t *testing.T) {
	assert := assert.New(t)

	p := newPayload(nil, nil)

	assert.WithinDuration(time.Now(), p.creationDate, 1*time.Second)
}

func TestQueuablePayloadSender_WorkingEndpoint(t *testing.T) {
	assert := assert.New(t)

	// Given an endpoint that doesn't fail
	workingEndpoint := &testEndpoint{}

	// And a queuable sender using that endpoint
	conf := writerconfig.DefaultQueuablePayloadSenderConf()
	conf.InChannelSize = 0 // block in tests
	queuableSender := newSender(workingEndpoint, conf)

	// And a test monitor for that sender
	monitor := newTestPayloadSenderMonitor(queuableSender)

	// When we start the sender
	monitor.Start()
	queuableSender.Start()

	// And send some payloads
	payload1 := randomPayload()
	queuableSender.Send(payload1)
	payload2 := randomPayload()
	queuableSender.Send(payload2)
	payload3 := randomPayload()
	queuableSender.Send(payload3)
	payload4 := randomPayload()
	queuableSender.Send(payload4)
	payload5 := randomPayload()
	queuableSender.Send(payload5)

	// And stop the sender
	queuableSender.Stop()
	monitor.Stop()

	// Then we expect all sent payloads to have been successfully sent
	successPayloads := monitor.SuccessPayloads()
	errorPayloads := monitor.FailurePayloads()
	assert.ElementsMatch([]*payload{payload1, payload2, payload3, payload4, payload5}, successPayloads,
		"Expect all sent payloads to have been successful")
	assert.ElementsMatch(successPayloads, workingEndpoint.SuccessPayloads(), "Expect sender and endpoint to match on successful payloads")
	assert.Len(errorPayloads, 0, "No payloads should have errored out on send")
	assert.Len(workingEndpoint.ErrorPayloads(), 0, "No payloads should have errored out on send")
}

func TestQueuablePayloadSender_FlakyEndpoint(t *testing.T) {
	assert := assert.New(t)

	// Given an endpoint that initially works ok
	flakyEndpoint := &testEndpoint{}

	// And a test backoff timer that can be triggered on-demand
	testBackoffTimer := testutil.NewTestBackoffTimer()

	// And a queuable sender using said endpoint and timer
	conf := writerconfig.DefaultQueuablePayloadSenderConf()
	conf.InChannelSize = 0 // block in tests
	queuableSender := newSender(flakyEndpoint, conf)
	queuableSender.backoffTimer = testBackoffTimer
	syncBarrier := make(chan interface{})
	queuableSender.syncBarrier = syncBarrier
	wait := func() {
		syncBarrier <- nil
		queuableSender.wg.Wait()
	}

	// And a test monitor for that sender
	monitor := newTestPayloadSenderMonitor(queuableSender)

	monitor.Start()
	queuableSender.Start()

	// With a working endpoint
	// We send some payloads
	payload1 := randomPayload()
	queuableSender.Send(payload1)
	payload2 := randomPayload()
	queuableSender.Send(payload2)

	wait()

	assert.Equal(0, queuableSender.queuedPayloads.Len(), "Expect no queued payloads")

	// With a failing endpoint with a retriable error
	flakyEndpoint.SetError(&retriableError{err: fmt.Errorf("bleh"), endpoint: flakyEndpoint})
	// We send some payloads
	payload3 := randomPayload()
	queuableSender.Send(payload3)

	wait()

	payload4 := randomPayload()
	queuableSender.Send(payload4)
	// And retry once
	testBackoffTimer.TriggerTick()
	// And retry twice
	testBackoffTimer.TriggerTick()

	wait()

	assert.Equal(2, queuableSender.queuedPayloads.Len(), "Expect 2 queued payloads")

	// With the previously failing endpoint working again
	flakyEndpoint.SetError(nil)
	// We retry for the third time
	testBackoffTimer.TriggerTick()

	wait()

	assert.Equal(0, queuableSender.queuedPayloads.Len(), "Expect no queued payloads")

	// Finally, with a failing endpoint with a non-retriable error
	flakyEndpoint.SetError(fmt.Errorf("non retriable bleh"))
	// We send some payloads
	payload5 := randomPayload()
	queuableSender.Send(payload5)
	payload6 := randomPayload()
	queuableSender.Send(payload6)

	wait()

	assert.Equal(0, queuableSender.queuedPayloads.Len(), "Expect no queued payloads")

	// With the previously failing endpoint working again
	flakyEndpoint.SetError(nil)
	// We retry just in case there's something in the queue
	testBackoffTimer.TriggerTick()

	// And stop the sender
	queuableSender.Stop()
	monitor.Stop()

	// Then we expect payloads sent during working endpoint or those that were retried due to retriable errors to have
	// been sent eventually (and in order). Those that failed because of non-retriable errors should have been discarded
	// even after a retry.
	successPayloads := monitor.SuccessPayloads()
	errorPayloads := monitor.FailurePayloads()
	retryPayloads := monitor.RetryPayloads()
	assert.ElementsMatch([]*payload{payload1, payload2, payload3, payload4}, successPayloads,
		"Expect all sent payloads to have been successful")
	assert.ElementsMatch(successPayloads, flakyEndpoint.SuccessPayloads(), "Expect sender and endpoint to match on successful payloads")
	// Expect 3 retry events for payload 3 (one because of first send, two others because of the two retries)
	assert.ElementsMatch([]*payload{payload3, payload3, payload3}, retryPayloads, "Expect payload 3 to have been retries 3 times")
	// We expect payloads 5 and 6 to appear in error payloads as they failed for non-retriable errors.
	assert.ElementsMatch([]*payload{payload5, payload6}, errorPayloads, "Expect errored payloads to have been discarded as expected")
}

func TestQueuablePayloadSender_MaxQueuedPayloads(t *testing.T) {
	assert := assert.New(t)

	// Given an endpoint that continuously throws out retriable errors
	flakyEndpoint := &testEndpoint{}
	flakyEndpoint.SetError(&retriableError{err: fmt.Errorf("bleh"), endpoint: flakyEndpoint})

	// And a test backoff timer that can be triggered on-demand
	testBackoffTimer := testutil.NewTestBackoffTimer()

	// And a queuable sender using said endpoint and timer and with a meager max queued payloads value of 1
	conf := writerconfig.DefaultQueuablePayloadSenderConf()
	conf.InChannelSize = 0 // block in tests
	conf.MaxQueuedPayloads = 1
	queuableSender := newSender(flakyEndpoint, conf)
	queuableSender.backoffTimer = testBackoffTimer
	syncBarrier := make(chan interface{})
	queuableSender.syncBarrier = syncBarrier
	wait := func() {
		syncBarrier <- nil
		queuableSender.wg.Wait()
	}

	// And a test monitor for that sender
	monitor := newTestPayloadSenderMonitor(queuableSender)

	monitor.Start()
	queuableSender.Start()

	// When sending a first payload
	payload1 := randomPayload()
	queuableSender.Send(payload1)
	wait()

	// Followed by another one
	payload2 := randomPayload()
	queuableSender.Send(payload2)
	wait()

	// Followed by a third
	payload3 := randomPayload()
	queuableSender.Send(payload3)
	wait()

	// Then, when the endpoint finally works
	flakyEndpoint.SetError(nil)

	// And we trigger a retry
	testBackoffTimer.TriggerTick()
	wait()

	// Then we should have no queued payloads
	assert.Equal(0, queuableSender.queuedPayloads.Len(), "We should have no queued payloads")

	// When we stop the sender
	queuableSender.Stop()
	monitor.Stop()

	// Then endpoint should have received only payload3. Other should have been discarded because max queued payloads
	// is 1
	assert.ElementsMatch([]*payload{payload3}, flakyEndpoint.SuccessPayloads(), "Endpoint should have received only payload 3")

	// Monitor should agree on previous fact
	assert.ElementsMatch([]*payload{payload3}, monitor.SuccessPayloads(),
		"Monitor should agree with endpoint on successful payloads")
	assert.ElementsMatch([]*payload{payload1, payload2}, monitor.FailurePayloads(),
		"Monitor should agree with endpoint on failed payloads")
	assert.Contains(monitor.FailureEvents()[0].err.Error(), "max queued payloads",
		"Monitor failure event should mention correct reason for error")
	assert.Contains(monitor.FailureEvents()[1].err.Error(), "max queued payloads",
		"Monitor failure event should mention correct reason for error")
}

func TestQueuablePayloadSender_MaxQueuedBytes(t *testing.T) {
	assert := assert.New(t)

	// Given an endpoint that continuously throws out retriable errors
	flakyEndpoint := &testEndpoint{}
	flakyEndpoint.SetError(&retriableError{err: fmt.Errorf("bleh"), endpoint: flakyEndpoint})

	// And a test backoff timer that can be triggered on-demand
	testBackoffTimer := testutil.NewTestBackoffTimer()

	// And a queuable sender using said endpoint and timer and with a meager max size of 10 bytes
	conf := writerconfig.DefaultQueuablePayloadSenderConf()
	conf.InChannelSize = 0 // block in tests
	conf.MaxQueuedBytes = 10
	queuableSender := newSender(flakyEndpoint, conf)
	queuableSender.backoffTimer = testBackoffTimer
	syncBarrier := make(chan interface{})
	queuableSender.syncBarrier = syncBarrier
	wait := func() {
		syncBarrier <- nil
		queuableSender.wg.Wait()
	}

	// And a test monitor for that sender
	monitor := newTestPayloadSenderMonitor(queuableSender)

	monitor.Start()
	queuableSender.Start()

	// When sending a first payload of 4 bytes
	payload1 := randomSizedPayload(4)
	queuableSender.Send(payload1)
	wait()

	// Followed by another one of 2 bytes
	payload2 := randomSizedPayload(2)
	queuableSender.Send(payload2)
	wait()

	// Followed by a third of 8 bytes
	payload3 := randomSizedPayload(8)
	queuableSender.Send(payload3)
	wait()

	// Then, when the endpoint finally works
	flakyEndpoint.SetError(nil)

	// And we trigger a retry
	testBackoffTimer.TriggerTick()

	wait()

	// Then we should have no queued payloads
	assert.Equal(0, queuableSender.queuedPayloads.Len(), "We should have no queued payloads")

	// When we stop the sender
	queuableSender.Stop()
	monitor.Stop()

	// Then endpoint should have received payload2 and payload3. Payload1 should have been discarded because keeping all
	// 3 would have put us over the max size of sender
	assert.ElementsMatch([]*payload{payload2, payload3}, flakyEndpoint.SuccessPayloads(),
		"Endpoint should have received only payload 2 and 3 (in that order)")

	// Monitor should agree on previous fact
	assert.ElementsMatch([]*payload{payload2, payload3}, monitor.SuccessPayloads(),
		"Monitor should agree with endpoint on successful payloads")
	assert.ElementsMatch([]*payload{payload1}, monitor.FailurePayloads(),
		"Monitor should agree with endpoint on failed payloads")
	assert.Contains(monitor.FailureEvents()[0].err.Error(), "max queued bytes",
		"Monitor failure event should mention correct reason for error")
}

func TestQueuablePayloadSender_DropBigPayloadsOnRetry(t *testing.T) {
	assert := assert.New(t)

	// Given an endpoint that continuously throws out retriable errors
	flakyEndpoint := &testEndpoint{}
	flakyEndpoint.SetError(&retriableError{err: fmt.Errorf("bleh"), endpoint: flakyEndpoint})

	// And a test backoff timer that can be triggered on-demand
	testBackoffTimer := testutil.NewTestBackoffTimer()

	// And a queuable sender using said endpoint and timer and with a meager max size of 10 bytes
	conf := writerconfig.DefaultQueuablePayloadSenderConf()
	conf.InChannelSize = 0 // block in tests
	conf.MaxQueuedBytes = 10
	queuableSender := newSender(flakyEndpoint, conf)
	queuableSender.backoffTimer = testBackoffTimer
	syncBarrier := make(chan interface{})
	queuableSender.syncBarrier = syncBarrier
	wait := func() {
		syncBarrier <- nil
		queuableSender.wg.Wait()
	}

	// And a test monitor for that sender
	monitor := newTestPayloadSenderMonitor(queuableSender)

	monitor.Start()
	queuableSender.Start()

	// When sending a payload of 12 bytes
	payload1 := randomSizedPayload(12)
	queuableSender.Send(payload1)

	wait()

	// Then, when the endpoint finally works
	flakyEndpoint.SetError(nil)

	// And we trigger a retry
	testBackoffTimer.TriggerTick()

	wait()

	// Then we should have no queued payloads
	assert.Equal(0, queuableSender.queuedPayloads.Len(), "We should have no queued payloads")

	// When we stop the sender
	queuableSender.Stop()
	monitor.Stop()

	// Then endpoint should have received no payloads because payload1 was too big to store in queue.
	assert.Len(flakyEndpoint.SuccessPayloads(), 0, "Endpoint should have received no payloads")

	// And monitor should have received failed event for payload1 with correct reason
	assert.ElementsMatch([]*payload{payload1}, monitor.FailurePayloads(),
		"Monitor should agree with endpoint on failed payloads")
	assert.Contains(monitor.FailureEvents()[0].err.Error(), "bigger than max size",
		"Monitor failure event should mention correct reason for error")
}

func TestQueuablePayloadSender_SendBigPayloadsIfNoRetry(t *testing.T) {
	assert := assert.New(t)

	// Given an endpoint that works
	workingEndpoint := &testEndpoint{}

	// And a test backoff timer that can be triggered on-demand
	testBackoffTimer := testutil.NewTestBackoffTimer()

	// And a queuable sender using said endpoint and timer and with a meager max size of 10 bytes
	conf := writerconfig.DefaultQueuablePayloadSenderConf()
	conf.InChannelSize = 0 // block in tests
	conf.MaxQueuedBytes = 10
	queuableSender := newSender(workingEndpoint, conf)
	queuableSender.backoffTimer = testBackoffTimer
	syncBarrier := make(chan interface{})
	queuableSender.syncBarrier = syncBarrier
	wait := func() {
		syncBarrier <- nil
		queuableSender.wg.Wait()
	}

	// And a test monitor for that sender
	monitor := newTestPayloadSenderMonitor(queuableSender)

	monitor.Start()
	queuableSender.Start()

	// When sending a payload of 12 bytes
	payload1 := randomSizedPayload(12)
	queuableSender.Send(payload1)

	wait()

	// Then we should have no queued payloads
	assert.Equal(0, queuableSender.queuedPayloads.Len(), "We should have no queued payloads")

	// When we stop the sender
	queuableSender.Stop()
	monitor.Stop()

	// Then endpoint should have received payload1 because although it was big, it didn't get queued.
	assert.ElementsMatch([]*payload{payload1}, workingEndpoint.SuccessPayloads(), "Endpoint should have received payload1")

	// And monitor should have received success event for payload1
	assert.ElementsMatch([]*payload{payload1}, monitor.SuccessPayloads(),
		"Monitor should agree with endpoint on success payloads")
}

func TestQueuablePayloadSender_MaxAge(t *testing.T) {
	assert := assert.New(t)

	// Given an endpoint that continuously throws out retriable errors
	flakyEndpoint := &testEndpoint{}
	flakyEndpoint.SetError(&retriableError{err: fmt.Errorf("bleh"), endpoint: flakyEndpoint})

	// And a test backoff timer that can be triggered on-demand
	testBackoffTimer := testutil.NewTestBackoffTimer()

	// And a queuable sender using said endpoint and timer and with a meager max age of 100ms
	conf := writerconfig.DefaultQueuablePayloadSenderConf()
	conf.InChannelSize = 0 // block in tests
	conf.MaxAge = 100 * time.Millisecond
	queuableSender := newSender(flakyEndpoint, conf)
	queuableSender.backoffTimer = testBackoffTimer
	syncBarrier := make(chan interface{})
	queuableSender.syncBarrier = syncBarrier
	wait := func() {
		syncBarrier <- nil
		queuableSender.wg.Wait()
	}

	// And a test monitor for that sender
	monitor := newTestPayloadSenderMonitor(queuableSender)

	monitor.Start()
	queuableSender.Start()

	// When sending two payloads one after the other
	payload1 := randomPayload()
	queuableSender.Send(payload1)
	payload2 := randomPayload()
	queuableSender.Send(payload2)

	// And then sleeping for 500ms
	time.Sleep(500 * time.Millisecond)

	// And then sending a third payload
	payload3 := randomPayload()
	queuableSender.Send(payload3)

	// And then triggering a retry
	testBackoffTimer.TriggerTick()

	wait()

	// Then, when the endpoint finally works
	flakyEndpoint.SetError(nil)

	// And we trigger a retry
	testBackoffTimer.TriggerTick()

	wait()

	// Then we should have no queued payloads
	assert.Equal(0, queuableSender.queuedPayloads.Len(), "We should have no queued payloads")

	// When we stop the sender
	queuableSender.Stop()
	monitor.Stop()

	// Then endpoint should have received only payload3. Because payload1 and payload2 were too old after the failed
	// retry (first TriggerTick).
	assert.ElementsMatch([]*payload{payload3}, flakyEndpoint.SuccessPayloads(), "Endpoint should have received only payload 3")

	// And monitor should have received failed events for payload1 and payload2 with correct reason
	assert.ElementsMatch([]*payload{payload1, payload2}, monitor.FailurePayloads(),
		"Monitor should agree with endpoint on failed payloads")
	assert.Contains(monitor.FailureEvents()[0].err.Error(), "older than max age",
		"Monitor failure event should mention correct reason for error")
}

func TestQueuablePayloadSender_RetryOfTooOldQueue(t *testing.T) {
	assert := assert.New(t)

	// Given an endpoint that continuously throws out retriable errors
	flakyEndpoint := &testEndpoint{}
	flakyEndpoint.SetError(&retriableError{err: fmt.Errorf("bleh"), endpoint: flakyEndpoint})

	// And a backoff timer that triggers every 100ms
	testBackoffTimer := backoff.NewCustomTimer(func(numRetries int, err error) time.Duration {
		return 100 * time.Millisecond
	})

	// And a queuable sender using said endpoint and timer and with a meager max age of 200ms
	conf := writerconfig.DefaultQueuablePayloadSenderConf()
	conf.InChannelSize = 0 // block in tests
	conf.MaxAge = 200 * time.Millisecond
	queuableSender := newSender(flakyEndpoint, conf)
	queuableSender.backoffTimer = testBackoffTimer
	syncBarrier := make(chan interface{})
	queuableSender.syncBarrier = syncBarrier
	wait := func() {
		syncBarrier <- nil
		queuableSender.wg.Wait()
	}

	// And a test monitor for that sender
	monitor := newTestPayloadSenderMonitor(queuableSender)

	monitor.Start()
	queuableSender.Start()

	// When sending two payloads one after the other
	payload1 := randomPayload()
	queuableSender.Send(payload1)
	payload2 := randomPayload()
	queuableSender.Send(payload2)

	// And then sleeping for 500ms
	time.Sleep(600 * time.Millisecond)

	// Then, eventually, during one of the retries those 2 payloads should end up being discarded and our queue
	// will end up with a size of 0 and a flush call will be made for a queue of size 0

	// Then send a third payload
	payload3 := randomPayload()
	queuableSender.Send(payload3)

	wait()

	// Then, when the endpoint finally works
	flakyEndpoint.SetError(nil)

	// Wait for a retry
	time.Sleep(200 * time.Millisecond)

	// When we stop the sender
	queuableSender.Stop()
	monitor.Stop()

	// Then we should have no queued payloads
	assert.Equal(0, queuableSender.queuedPayloads.Len(), "We should have no queued payloads")

	// Then endpoint should have received only payload3. Because payload1 and payload2 were too old after the failed
	// retry (first TriggerTick).
	assert.ElementsMatch([]*payload{payload3}, flakyEndpoint.SuccessPayloads(), "Endpoint should have received only payload 3")

	// And monitor should have received failed events for payload1 and payload2 with correct reason
	assert.ElementsMatch([]*payload{payload1, payload2}, monitor.FailurePayloads(),
		"Monitor should agree with endpoint on failed payloads")
	assert.Contains(monitor.FailureEvents()[0].err.Error(), "older than max age",
		"Monitor failure event should mention correct reason for error")
}
