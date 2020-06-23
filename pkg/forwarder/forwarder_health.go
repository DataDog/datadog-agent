// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import (
	"expvar"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/status/health"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	fakeAPIKey = "00000000000000000000000000000000"
)

var (
	apiKeyStatusUnknown = expvar.String{}
	apiKeyInvalid       = expvar.String{}
	apiKeyValid         = expvar.String{}
	apiKeyFake          = expvar.String{}

	validateAPIKeyTimeout = 10 * time.Second

	apiKeyStatus = expvar.Map{}
)

func init() {
	apiKeyStatusUnknown.Set("Unable to validate API Key")
	apiKeyInvalid.Set("API Key invalid")
	apiKeyValid.Set("API Key valid")
	apiKeyFake.Set("Fake API Key that skips validation")
}

func initForwarderHealthExpvars() {
	apiKeyStatus.Init()
	forwarderExpvars.Set("APIKeyStatus", &apiKeyStatus)
}

// forwarderHealth report the health status of the Forwarder. A Forwarder is
// unhealthy if the API keys are not longer valid
type forwarderHealth struct {
	health                *health.Handle
	stop                  chan bool
	stopped               chan struct{}
	timeout               time.Duration
	keysPerDomains        map[string][]string
	keysPerAPIEndpoint    map[string][]string
	disableAPIKeyChecking bool
	validationInterval    time.Duration
}

func (fh *forwarderHealth) init() {
	fh.stop = make(chan bool, 1)
	fh.stopped = make(chan struct{})

	fh.keysPerAPIEndpoint = make(map[string][]string)
	fh.computeDomainsURL()

	// Since timeout is the maximum duration we can wait, we need to divide it
	// by the total number of api keys to obtain the max duration for each key
	apiKeyCount := 0
	for _, apiKeys := range fh.keysPerDomains {
		apiKeyCount += len(apiKeys)
	}

	fh.timeout = validateAPIKeyTimeout
	if apiKeyCount != 0 {
		fh.timeout /= time.Duration(apiKeyCount)
	}
}

func (fh *forwarderHealth) Start() {
	if fh.disableAPIKeyChecking {
		return
	}

	fh.health = health.RegisterReadiness("forwarder")
	fh.init()
	go fh.healthCheckLoop()
}

func (fh *forwarderHealth) Stop() {
	if fh.disableAPIKeyChecking {
		return
	}

	fh.health.Deregister() //nolint:errcheck
	fh.stop <- true
	<-fh.stopped
}

func (fh *forwarderHealth) healthCheckLoop() {
	log.Debug("Waiting for APIkey validity to be confirmed.")

	validateTicker := time.NewTicker(fh.validationInterval)
	defer validateTicker.Stop()
	defer close(fh.stopped)

	valid := fh.hasValidAPIKey()
	// If no key is valid, no need to keep checking, they won't magically become valid
	if !valid {
		log.Errorf("No valid api key found, reporting the forwarder as unhealthy.")
		return
	}

	for {
		select {
		case <-fh.stop:
			return
		case <-validateTicker.C:
			valid := fh.hasValidAPIKey()
			if !valid {
				log.Errorf("No valid api key found, reporting the forwarder as unhealthy.")
				return
			}
		case <-fh.health.C:
		}
	}
}

// computeDomainsURL populates a map containing API Endpoints per API keys that belongs to the forwarderHealth struct
func (fh *forwarderHealth) computeDomainsURL() {
	for domain, apiKeys := range fh.keysPerDomains {
		apiDomain := ""
		re := regexp.MustCompile("datadoghq.[a-z]*")
		if re.MatchString(domain) {
			apiDomain = "https://api." + re.FindString(domain)
		} else {
			apiDomain = domain
		}
		fh.keysPerAPIEndpoint[apiDomain] = append(fh.keysPerAPIEndpoint[apiDomain], apiKeys...)
	}
}

func (fh *forwarderHealth) setAPIKeyStatus(apiKey string, domain string, status expvar.Var) {
	if len(apiKey) > 5 {
		apiKey = apiKey[len(apiKey)-5:]
	}
	obfuscatedKey := fmt.Sprintf("API key ending with %s", apiKey)
	apiKeyStatus.Set(obfuscatedKey, status)
}

func (fh *forwarderHealth) validateAPIKey(apiKey, domain string) (bool, error) {
	if apiKey == fakeAPIKey {
		fh.setAPIKeyStatus(apiKey, domain, &apiKeyFake)
		return true, nil
	}

	url := fmt.Sprintf("%s%s?api_key=%s", domain, v1ValidateEndpoint, apiKey)

	transport := httputils.CreateHTTPTransport()

	client := &http.Client{
		Transport: transport,
		Timeout:   fh.timeout,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fh.setAPIKeyStatus(apiKey, domain, &apiKeyStatusUnknown)
		return false, err
	}

	req.Header.Set(useragentHTTPHeaderKey, fmt.Sprintf("datadog-agent/%s", version.AgentVersion))

	resp, err := client.Do(req)
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

func (fh *forwarderHealth) hasValidAPIKey() bool {
	validKey := false
	apiError := false

	for domain, apiKeys := range fh.keysPerAPIEndpoint {
		for _, apiKey := range apiKeys {
			v, err := fh.validateAPIKey(apiKey, domain)
			if err != nil {
				log.Debug(err)
				apiError = true
			} else if v {
				log.Debugf("api_key '%s' for domain %s is valid", apiKey, domain)
				validKey = true
			} else {
				log.Warnf("api_key '%s' for domain %s is invalid", apiKey, domain)
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
