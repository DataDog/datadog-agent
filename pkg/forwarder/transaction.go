// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package forwarder

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
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

	createdAt time.Time
}

const (
	apiKeyReplacement = "api_key=*************************$1"
)

var apiKeyRegExp = regexp.MustCompile("api_key=*\\w+(\\w{5})")

// NewHTTPTransaction returns a new HTTPTransaction.
func NewHTTPTransaction() *HTTPTransaction {
	return &HTTPTransaction{
		createdAt:  time.Now(),
		ErrorCount: 0,
		Headers:    make(http.Header),
	}
}

// GetCreatedAt returns the creation time of the HTTPTransaction.
func (t *HTTPTransaction) GetCreatedAt() time.Time {
	return t.createdAt
}

// GetTarget return the url used by the transaction
func (t *HTTPTransaction) GetTarget() string {
	url := t.Domain + t.Endpoint
	return apiKeyRegExp.ReplaceAllString(url, apiKeyReplacement) // sanitized url that can be logged
}

// Process sends the Payload of the transaction to the right Endpoint and Domain.
func (t *HTTPTransaction) Process(ctx context.Context, client *http.Client) error {
	reader := bytes.NewReader(*t.Payload)
	url := t.Domain + t.Endpoint
	logURL := apiKeyRegExp.ReplaceAllString(url, apiKeyReplacement) // sanitized url that can be logged

	req, err := http.NewRequest("POST", url, reader)
	if err != nil {
		log.Errorf("Could not create request for transaction to invalid URL %q (dropping transaction): %s", logURL, err)
		transactionsExpvar.Add("Errors", 1)
		return nil
	}
	req = req.WithContext(ctx)
	req.Header = t.Headers
	resp, err := client.Do(req)

	if err != nil {
		// Do not requeue transaction if that one was canceled
		if ctx.Err() == context.Canceled {
			return nil
		}
		t.ErrorCount++
		transactionsExpvar.Add("Errors", 1)
		return fmt.Errorf("error while sending transaction, rescheduling it: %s", apiKeyRegExp.ReplaceAllString(err.Error(), apiKeyReplacement))
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Fail to read the response Body: %s", err)
		return err
	}

	if resp.StatusCode == 400 || resp.StatusCode == 404 || resp.StatusCode == 413 {
		log.Errorf("Error code %q received while sending transaction to %q: %s, dropping it", resp.Status, logURL, string(body))
		transactionsExpvar.Add("Dropped", 1)
		return nil
	} else if resp.StatusCode == 403 {
		log.Errorf("API Key invalid, dropping transaction for %s", logURL)
		transactionsExpvar.Add("Dropped", 1)
		return nil
	} else if resp.StatusCode > 400 {
		t.ErrorCount++
		transactionsExpvar.Add("Errors", 1)
		return fmt.Errorf("error %q while sending transaction to %q, rescheduling it", resp.Status, logURL)
	}

	successfulTransactions.Add(1)

	loggingFrequency := config.Datadog.GetInt64("logging_frequency")

	if successfulTransactions.Value() == 1 {
		log.Infof("Successfully posted payload to %q, the agent will only log transaction success every %d transactions", logURL, loggingFrequency)
		log.Debugf("Url: %q payload: %s", logURL, string(body))
		return nil
	}
	if successfulTransactions.Value()%loggingFrequency == 0 {
		log.Infof("Successfully posted payload to %q", logURL)
		log.Debugf("Payload: %s", logURL, string(body))
		return nil
	}
	log.Debugf("Successfully posted payload to %q: %s", logURL, string(body))
	return nil
}
