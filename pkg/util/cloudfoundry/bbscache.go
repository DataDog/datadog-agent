// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks

package cloudfoundry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"code.cloudfoundry.org/bbs"
	"code.cloudfoundry.org/bbs/models"
	"code.cloudfoundry.org/lager"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// BBSCacheI is an interface for a structure that caches and automatically refreshes data from Cloud Foundry BBS API
// it's useful mostly to be able to mock BBSCache during unit tests
type BBSCacheI interface {
	LastUpdated() time.Time
	GetPollAttempts() int
	GetPollSuccesses() int
	GetActualLRPFor(instanceGUID string) ActualLRP
	GetActualLRPsFor(appGUID string) []ActualLRP
	GetDesiredLRPFor(appGUID string) DesiredLRP
	GetAllLRPs() (map[string][]ActualLRP, map[string]DesiredLRP)
	ExtractTags() map[string][]string
}

// BBSCache is a simple structure that caches and automatically refreshes data from Cloud Foundry BBS API
type BBSCache struct {
	sync.RWMutex
	cancelContext      context.Context
	configured         bool
	bbsAPIClient       bbs.Client
	bbsAPIClientLogger lager.Logger
	pollInterval       time.Duration
	pollAttempts       int
	pollSuccesses      int
	// maps Desired LRPs' AppGUID to list of ActualLRPs (IOW this is list of running containers per app)
	actualLRPs              map[string][]ActualLRP
	actualLRPByInstanceGUID map[string]*ActualLRP
	desiredLRPs             map[string]DesiredLRP
	lastUpdated             time.Time
}

var (
	globalBBSCache     = &BBSCache{}
	globalBBSCacheLock sync.Mutex
)

// ConfigureGlobalBBSCache configures the global instance of BBSCache from provided config
func ConfigureGlobalBBSCache(ctx context.Context, bbsURL, cafile, certfile, keyfile string, pollInterval time.Duration, testing bbs.Client) (*BBSCache, error) {
	globalBBSCacheLock.Lock()
	defer globalBBSCacheLock.Unlock()

	if globalBBSCache.configured {
		return globalBBSCache, nil
	}

	globalBBSCache.configured = true
	if testing != nil {
		globalBBSCache.bbsAPIClient = testing
	} else {
		clientConfig := bbs.ClientConfig{
			URL:                    bbsURL,
			IsTLS:                  true,
			CAFile:                 cafile,
			CertFile:               certfile,
			KeyFile:                keyfile,
			ClientSessionCacheSize: 0,
			MaxIdleConnsPerHost:    0,
			InsecureSkipVerify:     false,
			Retries:                10,
			RequestTimeout:         5 * time.Second,
		}
		var err error
		globalBBSCache.bbsAPIClient, err = bbs.NewClientWithConfig(clientConfig)
		if err != nil {
			return nil, err
		}
	}

	globalBBSCache.bbsAPIClientLogger = lager.NewLogger("bbs")
	globalBBSCache.pollInterval = pollInterval
	globalBBSCache.lastUpdated = time.Time{} // zero time
	globalBBSCache.cancelContext = ctx

	go globalBBSCache.start()

	return globalBBSCache, nil
}

// GetGlobalBBSCache returns the global instance of BBSCache (or error if the instance is not configured yet)
func GetGlobalBBSCache() (*BBSCache, error) {
	if !globalBBSCache.configured {
		return nil, fmt.Errorf("global BBS Cache not configured")
	}
	return globalBBSCache, nil
}

// LastUpdated return the last time the cache was updated
func (bc *BBSCache) LastUpdated() time.Time {
	bc.RLock()
	defer bc.RUnlock()
	return bc.lastUpdated
}

// GetPollAttempts returns the number of times the cache queried the BBS API
func (bc *BBSCache) GetPollAttempts() int {
	bc.RLock()
	defer bc.RUnlock()
	return bc.pollAttempts
}

// GetPollSuccesses returns the number of times the cache successfully queried the BBS API
func (bc *BBSCache) GetPollSuccesses() int {
	bc.RLock()
	defer bc.RUnlock()
	return bc.pollSuccesses
}

// GetActualLRPFor returns the App GUID associated to an instance GUID
func (bc *BBSCache) GetActualLRPFor(instanceGUID string) ActualLRP {
	bc.RLock()
	defer bc.RUnlock()
	if val, ok := bc.actualLRPByInstanceGUID[instanceGUID]; ok {
		return *val
	}
	return ActualLRP{}
}

// GetActualLRPsFor returns slice of ActualLRP objects for given App GUID
func (bc *BBSCache) GetActualLRPsFor(appGUID string) []ActualLRP {
	bc.RLock()
	defer bc.RUnlock()
	if val, ok := bc.actualLRPs[appGUID]; ok {
		return val
	}
	return []ActualLRP{}
}

// GetDesiredLRPFor returns DesiredLRP for a specific app GUID
func (bc *BBSCache) GetDesiredLRPFor(appGUID string) DesiredLRP {
	bc.RLock()
	defer bc.RUnlock()
	return bc.desiredLRPs[appGUID]
}

// GetAllLRPs returns all Actual LRPs (in mapping {appGuid: []ActualLRP}) and all Desired LRPs
func (bc *BBSCache) GetAllLRPs() (map[string][]ActualLRP, map[string]DesiredLRP) {
	bc.RLock()
	defer bc.RUnlock()
	return bc.actualLRPs, bc.desiredLRPs
}

func (bc *BBSCache) start() {
	bc.readData()
	dataRefreshTicker := time.NewTicker(bc.pollInterval)
	for {
		select {
		case <-dataRefreshTicker.C:
			bc.readData()
		case <-bc.cancelContext.Done():
			dataRefreshTicker.Stop()
			return
		}
	}
}

func (bc *BBSCache) readData() {
	log.Debug("Reading data from BBS API")
	bc.Lock()
	bc.pollAttempts++
	bc.Unlock()
	var wg sync.WaitGroup
	var actualLRPs map[string][]ActualLRP
	var actualLRPByInstanceGUID map[string]*ActualLRP
	var desiredLRPs map[string]DesiredLRP
	var errActual, errDesired error

	wg.Add(2)

	go func() {
		actualLRPs, actualLRPByInstanceGUID, errActual = bc.readActualLRPs()
		wg.Done()
	}()
	go func() {
		desiredLRPs, errDesired = bc.readDesiredLRPs()
		wg.Done()
	}()
	wg.Wait()
	if errActual != nil {
		log.Errorf("Failed reading Actual LRP data from BBS API: %s", errActual.Error())
		return
	}
	if errDesired != nil {
		log.Errorf("Failed reading Desired LRP data from BBS API: %s", errDesired.Error())
		return
	}

	// put new values in cache
	bc.Lock()
	defer bc.Unlock()
	log.Debug("Data from BBS API read successfully, refreshing the cache")
	bc.actualLRPs = actualLRPs
	bc.actualLRPByInstanceGUID = actualLRPByInstanceGUID
	bc.desiredLRPs = desiredLRPs
	bc.lastUpdated = time.Now()
	bc.pollSuccesses++
}

func (bc *BBSCache) readActualLRPs() (map[string][]ActualLRP, map[string]*ActualLRP, error) {
	actualLRPs := map[string][]ActualLRP{}
	actualLRPByInstanceGUID := map[string]*ActualLRP{}
	actualLRPsBBS, err := bc.bbsAPIClient.ActualLRPs(bc.bbsAPIClientLogger, models.ActualLRPFilter{})
	if err != nil {
		return actualLRPs, actualLRPByInstanceGUID, err
	}
	for _, lrp := range actualLRPsBBS {
		alrp := ActualLRPFromBBSModel(lrp)
		if lrpList, ok := actualLRPs[alrp.AppGUID]; ok {
			actualLRPs[alrp.AppGUID] = append(lrpList, alrp)
			actualLRPByInstanceGUID[alrp.InstanceGUID] = &actualLRPs[alrp.AppGUID][len(lrpList)]
		} else {
			actualLRPs[alrp.AppGUID] = []ActualLRP{alrp}
			actualLRPByInstanceGUID[alrp.InstanceGUID] = &actualLRPs[alrp.AppGUID][0]
		}
	}
	log.Debugf("Successfully read %d Actual LRPs", len(actualLRPsBBS))
	return actualLRPs, actualLRPByInstanceGUID, nil
}

func (bc *BBSCache) readDesiredLRPs() (map[string]DesiredLRP, error) {
	desiredLRPsBBS, err := bc.bbsAPIClient.DesiredLRPs(bc.bbsAPIClientLogger, models.DesiredLRPFilter{})
	if err != nil {
		return map[string]DesiredLRP{}, err
	}
	desiredLRPs := make(map[string]DesiredLRP, len(desiredLRPsBBS))
	for _, lrp := range desiredLRPsBBS {
		desiredLRP := DesiredLRPFromBBSModel(lrp)
		desiredLRPs[desiredLRP.AppGUID] = desiredLRP
	}
	log.Debugf("Successfully read %d Desired LRPs", len(desiredLRPsBBS))
	return desiredLRPs, nil
}

// ExtractTags extract all the container tags for each app in the cache an returns a mapping of tags by instance GUID
func (bc *BBSCache) ExtractTags() map[string][]string {
	tags := map[string][]string{}
	alrps, dlrps := bc.GetAllLRPs()
	for appGUID, dlrp := range dlrps {
		alrpsForApp, ok := alrps[appGUID]
		if !ok {
			log.Debugf("Could not find actual LRPs for app GUID %s", appGUID)
			continue
		}
		vcApp := dlrp.EnvVcapApplication
		appName, ok := vcApp["application_name"]
		if !ok {
			log.Debugf("Could not find application_name of app %s", appGUID)
			continue
		}
		for _, alrp := range alrpsForApp {
			tags[alrp.InstanceGUID] = []string{
				fmt.Sprintf("container_name:%s_%d", appName, alrp.Index),
				fmt.Sprintf("app_name:%s", appName),
				fmt.Sprintf("app_guid:%s", appGUID),
				fmt.Sprintf("app_instance_index:%d", alrp.Index),
				fmt.Sprintf("app_instance_guid:%s", alrp.InstanceGUID),
			}
		}
	}
	return tags
}
