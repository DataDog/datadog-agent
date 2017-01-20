package forwarder

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewHTTPTransaction(t *testing.T) {
	before := time.Now()
	transaction := NewHTTPTransaction()
	after := time.Now()

	assert.NotNil(t, transaction)
	assert.Equal(t, transaction.ErrorCount, 0)

	assert.True(t, transaction.nextFlush.After(before))
	assert.True(t, transaction.nextFlush.Before(after))

	assert.True(t, transaction.createdAt.After(before))
	assert.True(t, transaction.createdAt.Before(after))
}

func TestGetNextFlush(t *testing.T) {
	transaction := NewHTTPTransaction()
	assert.Equal(t, transaction.nextFlush, transaction.GetNextFlush())
}

func TestGetCreatedAt(t *testing.T) {
	transaction := NewHTTPTransaction()

	assert.NotNil(t, transaction)
	assert.Equal(t, transaction.createdAt, transaction.GetCreatedAt())
}

func TestReschedule(t *testing.T) {
	transaction := NewHTTPTransaction()

	baseSchedule := transaction.nextFlush
	transaction.Reschedule()

	assert.Equal(t, baseSchedule, transaction.nextFlush)

	transaction.ErrorCount = 1
	before := time.Now()
	transaction.Reschedule()
	after := time.Now()

	assert.True(t, transaction.nextFlush.After(before.Add(retryInterval)))
	assert.True(t, transaction.nextFlush.Before(after.Add(retryInterval)))
}

func TestMaxReschedule(t *testing.T) {
	transaction := NewHTTPTransaction()
	transaction.ErrorCount = 200

	before := time.Now()
	transaction.Reschedule()
	after := time.Now()

	assert.True(t, transaction.nextFlush.After(before.Add(maxRetryInterval)))
	assert.True(t, transaction.nextFlush.Before(after.Add(maxRetryInterval)))
}

func TestProcess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	transaction := NewHTTPTransaction()
	transaction.Domain = ts.URL
	transaction.Endpoint = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = &payload

	client := &http.Client{}

	err := transaction.Process(client)
	assert.Nil(t, err)
	assert.Equal(t, transaction.ErrorCount, 0)
}

func TestProcessInvalidDomain(t *testing.T) {
	transaction := NewHTTPTransaction()
	transaction.Domain = "://invalid"
	transaction.Endpoint = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = &payload

	client := &http.Client{}

	err := transaction.Process(client)
	assert.Nil(t, err)
	assert.Equal(t, transaction.ErrorCount, 0)
}

func TestProcessNetworkError(t *testing.T) {
	transaction := NewHTTPTransaction()
	transaction.Domain = "http://localhost:1234"
	transaction.Endpoint = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = &payload

	client := &http.Client{}

	err := transaction.Process(client)
	assert.NotNil(t, err)
	assert.Equal(t, transaction.ErrorCount, 1)
}

func TestProcessHTTPError(t *testing.T) {
	errorCode := http.StatusNotFound

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(errorCode)
	}))
	defer ts.Close()

	transaction := NewHTTPTransaction()
	transaction.Domain = ts.URL
	transaction.Endpoint = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = &payload

	client := &http.Client{}

	err := transaction.Process(client)
	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "Error '404 Not Found' while sending transaction, rescheduling it")
	assert.Equal(t, transaction.ErrorCount, 1)

	errorCode = http.StatusBadRequest
	err = transaction.Process(client)
	assert.Nil(t, err)
	assert.Equal(t, transaction.ErrorCount, 1)

	errorCode = http.StatusRequestEntityTooLarge
	err = transaction.Process(client)
	assert.Nil(t, err)
	assert.Equal(t, transaction.ErrorCount, 1)
}
