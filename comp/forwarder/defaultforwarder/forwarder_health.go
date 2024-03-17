// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"expvar"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	fakeAPIKey = "00000000000000000000000000000000"
)

var (
	apiKeyEndpointUnreachable  = expvar.String{}
	apiKeyUnexpectedStatusCode = expvar.String{}
	apiKeyRemove               = expvar.String{}
	apiKeyInvalid              = expvar.String{}
	apiKeyValid                = expvar.String{}
	apiKeyFake                 = expvar.String{}

	validateAPIKeyTimeout = 10 * time.Second

	apiKeyStatus  = expvar.Map{}
	apiKeyFailure = expvar.Map{}

	// domainURLRegexp determines if an URL belongs to Datadog or not. If the URL belongs to Datadog it's prefixed
	// with 'api.' (see computeDomainURLAPIKeyMap).
	domainURLRegexp = regexp.MustCompile(`([a-z]{2}\d\.)?(datadoghq\.[a-z]+|ddog-gov\.com)$`)
)

func init() {
	apiKeyEndpointUnreachable.Set("Unable to reach the API Key validation endpoint")
	apiKeyUnexpectedStatusCode.Set("Unexpected response code from the API Key validation endpoint")
	apiKeyInvalid.Set("API Key invalid")
	apiKeyValid.Set("API Key valid")
	apiKeyFake.Set("Fake API Key that skips validation")
}

func initForwarderHealthExpvars() {
	apiKeyStatus.Init()
	apiKeyFailure.Init()
	transaction.ForwarderExpvars.Set("APIKeyStatus", &apiKeyStatus)
	transaction.ForwarderExpvars.Set("APIKeyFailure", &apiKeyFailure)
}

// forwarderHealth report the health status of the Forwarder. A Forwarder is
// unhealthy if the API keys are not longer valid
type forwarderHealth struct {
	log                   log.Component
	config                config.Component
	health                *health.Handle
	stop                  chan bool
	stopped               chan struct{}
	timeout               time.Duration
	domainResolvers       map[string]resolver.DomainResolver
	keysPerAPIEndpoint    map[string][]string
	disableAPIKeyChecking bool
	validationInterval    time.Duration
	keyMapMutex           sync.Mutex
}

func (fh *forwarderHealth) init() {
	fh.stop = make(chan bool, 1)
	fh.stopped = make(chan struct{})

	// build map of keys based upon the domain resolvers
	fh.keysPerAPIEndpoint = make(map[string][]string)
	fh.computeDomainURLAPIKeyMap()

	// Since timeout is the maximum duration we can wait, we need to divide it
	// by the total number of api keys to obtain the max duration for each key
	apiKeyCount := 0
	for _, dr := range fh.domainResolvers {
		apiKeyCount += len(dr.GetAPIKeys())
	}

	fh.timeout = validateAPIKeyTimeout
	if apiKeyCount != 0 {
		fh.timeout /= time.Duration(apiKeyCount)
	}

	// when the config updates the "api_key", process that change
	if fh.config != nil {
		fh.config.OnUpdate(func(setting string, oldValue, newValue any) {
			if setting != "api_key" {
				return
			}
			oldAPIKey, ok1 := oldValue.(string)
			newAPIKey, ok2 := newValue.(string)
			if ok1 && ok2 {
				fh.updateAPIKey(oldAPIKey, newAPIKey)
			}
		})
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
	fh.log.Debug("Waiting for APIkey validity to be confirmed.")

	validateTicker := time.NewTicker(fh.validationInterval)
	defer validateTicker.Stop()
	defer close(fh.stopped)

	valid := fh.checkValidAPIKey()
	// If no key is valid, no need to keep checking, they won't magically become valid
	if !valid {
		fh.log.Errorf("No valid api key found, reporting the forwarder as unhealthy.")
		return
	}

	for {
		select {
		case <-fh.stop:
			return
		case <-validateTicker.C:
			valid := fh.checkValidAPIKey()
			if !valid {
				fh.log.Errorf("No valid api key found, reporting the forwarder as unhealthy.")
				return
			}
		case <-fh.health.C:
		}
	}
}

func (fh *forwarderHealth) updateAPIKey(oldKey, newKey string) {
	fh.keyMapMutex.Lock()
	for domainURL, keyList := range fh.keysPerAPIEndpoint {
		replaceList := make([]string, 0, len(keyList))
		for _, currKey := range keyList {
			if currKey == oldKey {
				replaceList = append(replaceList, newKey)
			} else {
				replaceList = append(replaceList, currKey)
			}
		}
		fh.keysPerAPIEndpoint[domainURL] = replaceList
	}
	fh.keyMapMutex.Unlock()
	// remove old key messages, then check apiKey validity and update the messages
	fh.setAPIKeyStatus(oldKey, "", &apiKeyRemove)
	fh.checkValidAPIKey()
}

// computeDomainURLAPIKeyMap populates a map containing API Endpoints per API keys that belongs to the forwarderHealth struct
func (fh *forwarderHealth) computeDomainURLAPIKeyMap() {
	fh.keyMapMutex.Lock()
	for domain, dr := range fh.domainResolvers {
		if domainURLRegexp.MatchString(domain) {
			domain = "https://api." + domainURLRegexp.FindString(domain)
		}
		fh.keysPerAPIEndpoint[domain] = append(fh.keysPerAPIEndpoint[domain], dr.GetAPIKeys()...)
	}
	fh.keyMapMutex.Unlock()
}

func (fh *forwarderHealth) setAPIKeyStatus(apiKey string, _ string, status *expvar.String) {
	if len(apiKey) > 5 {
		apiKey = apiKey[len(apiKey)-5:]
	}
	obfuscatedKey := fmt.Sprintf("API key ending with %s", apiKey)
	if status == &apiKeyRemove {
		apiKeyStatus.Delete(obfuscatedKey)
		apiKeyFailure.Delete(obfuscatedKey)
	} else if status == &apiKeyInvalid {
		apiKeyFailure.Set(obfuscatedKey, status)
		apiKeyStatus.Delete(obfuscatedKey)
	} else {
		apiKeyStatus.Set(obfuscatedKey, status)
		apiKeyFailure.Delete(obfuscatedKey)
	}
}

func (fh *forwarderHealth) validateAPIKey(apiKey, domain string) (bool, error) {
	if apiKey == fakeAPIKey {
		fh.setAPIKeyStatus(apiKey, domain, &apiKeyFake)
		return true, nil
	}

	url := fmt.Sprintf("%s%s?api_key=%s", domain, endpoints.V1ValidateEndpoint, apiKey)

	transport := httputils.CreateHTTPTransport(fh.config)

	client := &http.Client{
		Transport: transport,
		Timeout:   fh.timeout,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fh.setAPIKeyStatus(apiKey, domain, &apiKeyEndpointUnreachable)
		return false, err
	}

	req.Header.Set(useragentHTTPHeaderKey, fmt.Sprintf("datadog-agent/%s", version.AgentVersion))

	resp, err := client.Do(req)
	if err != nil {
		fh.setAPIKeyStatus(apiKey, domain, &apiKeyEndpointUnreachable)
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

	fh.setAPIKeyStatus(apiKey, domain, &apiKeyUnexpectedStatusCode)
	return false, fmt.Errorf("unexpected response code from the apikey validation endpoint: %v", resp.StatusCode)
}

// check if any of the endpoints have a valid apiKey, updating health state
func (fh *forwarderHealth) checkValidAPIKey() bool {
	validKey := false
	apiError := false

	// mutex just to copy the map, to avoid holding onto it for too long
	keysPerDomain := make(map[string][]string)
	fh.keyMapMutex.Lock()
	for domain, apiKeys := range fh.keysPerAPIEndpoint {
		keysPerDomain[domain] = slices.Clone(apiKeys)
	}
	fh.keyMapMutex.Unlock()

	for domain, apiKeys := range keysPerDomain {
		for _, apiKey := range apiKeys {
			v, err := fh.validateAPIKey(apiKey, domain)
			if err != nil {
				fh.log.Debugf(
					"api_key '%s' for domain %s could not be validated: %s",
					apiKey,
					domain,
					err.Error(),
				)
				apiError = true
			} else if v {
				fh.log.Debugf("api_key '%s' for domain %s is valid", apiKey, domain)
				validKey = true
			} else {
				fh.log.Warnf("api_key '%s' for domain %s is invalid", apiKey, domain)
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
