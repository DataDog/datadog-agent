package forwarder

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	log "github.com/cihub/seelog"
)

// HTTPTransaction represents one Payload for one Endpoint on one Domain.
type HTTPTransaction struct {
	// Domain represents the domain target by the HTTPTransaction.
	Domain string
	// Endpoint is the API Endpoint used by the HTTPTransaction.
	Endpoint string
	// Headers are the HTTP headers used by the HTTPTransaction.
	Headers http.Header
	// Payload is the content delivered to the backend.
	Payload *[]byte
	// ErrorCount is the number of times this HTTPTransaction failed to be processed.
	ErrorCount int

	nextFlush time.Time
	createdAt time.Time
}

const (
	retryInterval    time.Duration = 20 * time.Second
	maxRetryInterval time.Duration = 90 * time.Second
)

// NewHTTPTransaction returns a new HTTPTransaction.
func NewHTTPTransaction() *HTTPTransaction {
	return &HTTPTransaction{
		nextFlush:  time.Now(),
		createdAt:  time.Now(),
		ErrorCount: 0,
		Headers:    make(http.Header),
	}
}

// GetNextFlush returns the next time when this HTTPTransaction expect to be processed.
func (t *HTTPTransaction) GetNextFlush() time.Time {
	return t.nextFlush
}

// GetCreatedAt returns the creation time of the HTTPTransaction.
func (t *HTTPTransaction) GetCreatedAt() time.Time {
	return t.createdAt
}

// Process sends the Payload of the transaction to the right Endpoint and Domain.
func (t *HTTPTransaction) Process(client *http.Client) error {
	reader := bytes.NewReader(*t.Payload)
	req, err := http.NewRequest("POST", t.Domain+t.Endpoint, reader)
	if err != nil {
		log.Errorf("Could not create request for transaction to invalid URL '%s' (dropping transaction): %s", t.Domain+t.Endpoint, err)
		transactionsCreation.Add("Errors", 1)
		return nil
	}
	req.Header = t.Headers
	resp, err := client.Do(req)
	if err != nil {
		t.ErrorCount++
		transactionsCreation.Add("Errors", 1)
		return fmt.Errorf("Error while sending transaction, rescheduling it: %s", err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode == 400 || resp.StatusCode == 413 {
		log.Errorf("Error code '%s' received while sending transaction to '%s': %s, dropping it", resp.Status, t.Domain+t.Endpoint, string(body))
		transactionsCreation.Add("Dropped", 1)
		return nil
	} else if resp.StatusCode > 400 {
		t.ErrorCount++
		transactionsCreation.Add("Errors", 1)
		return fmt.Errorf("Error '%s' while sending transaction, rescheduling it", resp.Status)
	}

	transactionsCreation.Add("Success", 1)
	log.Debugf("successfully posted payload to '%s': %s", t.Domain+t.Endpoint, string(body))
	return nil
}

// Reschedule update nextFlush time according to the number of ErrorCount. This
// will increase gaps between each retry as the ErrorCount increase.
func (t *HTTPTransaction) Reschedule() {
	if t.ErrorCount == 0 {
		return
	}

	newInterval := time.Duration(t.ErrorCount) * retryInterval
	if newInterval > maxRetryInterval {
		newInterval = maxRetryInterval
	}
	t.nextFlush = time.Now().Add(newInterval)
}
