package forwarder

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestNewWorker(t *testing.T) {
	input := make(chan Transaction)
	requeue := make(chan Transaction)

	w := NewWorker(input, requeue)
	assert.NotNil(t, w)
	assert.Equal(t, w.Client.Timeout, config.Datadog.GetDuration("forwarder_timeout")*time.Second)
}

func TestNewNoSSLWorker(t *testing.T) {
	input := make(chan Transaction)
	requeue := make(chan Transaction)

	config.Datadog.Set("skip_ssl_validation", true)
	defer config.Datadog.Set("skip_ssl_validation", false)

	w := NewWorker(input, requeue)
	assert.True(t, w.Client.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
}

func TestWorkerStart(t *testing.T) {
	input := make(chan Transaction)
	requeue := make(chan Transaction, 1)
	w := NewWorker(input, requeue)

	mock := newTestTransaction()
	mock.On("Process", w.Client).Return(nil).Times(1)

	w.Start()
	input <- mock
	w.Stop()
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Process", 1)
	mock.AssertNumberOfCalls(t, "Reschedule", 0)
}

func TestWorkerRetry(t *testing.T) {
	input := make(chan Transaction)
	requeue := make(chan Transaction, 1)
	w := NewWorker(input, requeue)

	mock := newTestTransaction()
	mock.On("Process", w.Client).Return(fmt.Errorf("some kind of error")).Times(1)
	mock.On("Reschedule").Return(nil).Times(1)

	w.Start()
	input <- mock
	retryTransaction := <-requeue
	w.Stop()
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Process", 1)
	mock.AssertNumberOfCalls(t, "Reschedule", 1)
	assert.Equal(t, mock, retryTransaction)
}
