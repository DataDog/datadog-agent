// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package forwarder

import (
	"expvar"
	"fmt"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	apiKeyStatusUnknown = expvar.String{}
	apiKeyInvalid       = expvar.String{}
	apiKeyValid         = expvar.String{}

	validateAPIKeyTimeout = 10 * time.Second

	apiKeyStatus = expvar.Map{}
)

func init() {
	apiKeyStatusUnknown.Set("Unable to validate API Key")
	apiKeyInvalid.Set("API Key invalid")
	apiKeyValid.Set("API Key valid")
}

func initForwarderHealthExpvars() {
	apiKeyStatus.Init()
	forwarderExpvars.Set("APIKeyStatus", &apiKeyStatus)
}

// forwarderHealth report the health status of the Forwarder. A Forwarder is
// unhealthy if the API keys are not longer valid or if to many transactions
// were dropped
type forwarderHealth struct {
	health  *health.Handle
	stop    chan bool
	stopped chan struct{}
	ddURL   string
	timeout time.Duration
}

func (fh *forwarderHealth) init(keysPerDomains map[string][]string) {
	fh.stop = make(chan bool, 1)
	fh.stopped = make(chan struct{})
	fh.ddURL = config.Datadog.GetString("dd_url")

	// Since timeout is the maximum duration we can wait, we need to divide it
	// by the total number of api keys to obtain the max duration for each key
	apiKeyCount := 0
	for _, apiKeys := range keysPerDomains {
		apiKeyCount += len(apiKeys)
	}

	fh.timeout = validateAPIKeyTimeout
	if apiKeyCount != 0 {
		fh.timeout /= time.Duration(apiKeyCount)
	}
}

func (fh *forwarderHealth) Start(keysPerDomains map[string][]string) {
	fh.health = health.Register("forwarder")
	fh.init(keysPerDomains)
	go fh.healthCheckLoop(keysPerDomains)
}

func (fh *forwarderHealth) Stop() {
	fh.health.Deregister()
	fh.stop <- true
	<-fh.stopped
}

func (fh *forwarderHealth) healthCheckLoop(keysPerDomains map[string][]string) {
	log.Debug("Waiting for APIkey validity to be confirmed.")

	validateTicker := time.NewTicker(time.Hour * 1)
	defer validateTicker.Stop()
	defer close(fh.stopped)

	valid := fh.hasValidAPIKey(keysPerDomains)
	// If no key is valid, no need to keep checking, they won't magicaly become valid
	if !valid {
		log.Errorf("No valid api key found, reporting the forwarder as unhealthy.")
		return
	}

	for {
		select {
		case <-fh.stop:
			return
		case <-validateTicker.C:
			valid := fh.hasValidAPIKey(keysPerDomains)
			if !valid {
				log.Errorf("No valid api key found, reporting the forwarder as unhealthy.")
				return
			}
		case <-fh.health.C:
			if transactionsDroppedOnInput.Value() != 0 {
				log.Errorf("Detected dropped transaction, reporting the forwarder as unhealthy: %v.", transactionsDroppedOnInput)
				return
			}
		}
	}
}

func (fh *forwarderHealth) setAPIKeyStatus(apiKey string, domain string, status expvar.Var) {
	if len(apiKey) > 5 {
		apiKey = apiKey[len(apiKey)-5:]
	}
	obfuscatedKey := fmt.Sprintf("API key ending by %s on endpoint %s", apiKey, domain)
	apiKeyStatus.Set(obfuscatedKey, status)
}

func (fh *forwarderHealth) validateAPIKey(apiKey, domain string) (bool, error) {
	url := fmt.Sprintf("%s%s?api_key=%s", fh.ddURL, v1ValidateEndpoint, apiKey)

	transport := util.CreateHTTPTransport()

	client := &http.Client{
		Transport: transport,
		Timeout:   fh.timeout,
	}

	resp, err := client.Get(url)
	if err != nil {
		fh.setAPIKeyStatus(apiKey, domain, &apiKeyStatusUnknown)
		return false, err
	}
	defer resp.Body.Close()

	// Server will respond 200 if the key is valid or 403 if invalid
	if resp.StatusCode == 200 {
		fh.setAPIKeyStatus(apiKey, domain, &apiKeyValid)
		return true, nil
	} else if resp.StatusCode == 403 {
		fh.setAPIKeyStatus(apiKey, domain, &apiKeyInvalid)
		return false, nil
	}

	fh.setAPIKeyStatus(apiKey, domain, &apiKeyStatusUnknown)
	return false, fmt.Errorf("Unexpected response code from the apikey validation endpoint: %v", resp.StatusCode)
}

func (fh *forwarderHealth) hasValidAPIKey(keysPerDomains map[string][]string) bool {
	validKey := false
	apiError := false

	for domain, apiKeys := range keysPerDomains {
		for _, apiKey := range apiKeys {
			v, err := fh.validateAPIKey(apiKey, domain)
			if err != nil {
				log.Debug(err)
				apiError = true
			} else if v {
				validKey = true
			}
		}
	}

	// If there is an error during the api call, we assume that there is a
	// valid key to avoid killing lots of agent on an outage.
	if apiError {
		return true
	}
	return validKey
}
