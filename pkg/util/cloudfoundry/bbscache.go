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
	GetActualLRPsForApp(appGUID string) ([]*ActualLRP, error)
	GetActualLRPsForCell(cellID string) ([]*ActualLRP, error)
	GetDesiredLRPFor(appGUID string) (DesiredLRP, error)
	GetAllLRPs() (map[string][]*ActualLRP, map[string]*DesiredLRP)
	GetTagsForNode(nodename string) (map[string][]string, error)
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
	actualLRPsByAppGUID map[string][]*ActualLRP
	actualLRPsByCellID  map[string][]*ActualLRP
	desiredLRPs         map[string]*DesiredLRP
	tagsByCellID        map[string]map[string][]string
	lastUpdated         time.Time
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

// GetActualLRPsForApp returns slice of pointers to ActualLRP objects for given App GUID
func (bc *BBSCache) GetActualLRPsForApp(appGUID string) ([]*ActualLRP, error) {
	bc.RLock()
	defer bc.RUnlock()
	if val, ok := bc.actualLRPsByAppGUID[appGUID]; ok {
		return val, nil
	}
	return []*ActualLRP{}, fmt.Errorf("actual LRPs for app %s not found", appGUID)
}

// GetActualLRPsForCell returns slice of pointers to ActualLRP objects for given App GUID
func (bc *BBSCache) GetActualLRPsForCell(cellID string) ([]*ActualLRP, error) {
	bc.RLock()
	defer bc.RUnlock()
	if val, ok := bc.actualLRPsByCellID[cellID]; ok {
		return val, nil
	}
	return []*ActualLRP{}, fmt.Errorf("actual LRPs for cell %s not found", cellID)
}

// GetDesiredLRPFor returns DesiredLRP for a specific app GUID
func (bc *BBSCache) GetDesiredLRPFor(appGUID string) (DesiredLRP, error) {
	bc.RLock()
	defer bc.RUnlock()
	if val, ok := bc.desiredLRPs[appGUID]; ok {
		return *val, nil
	}
	return DesiredLRP{}, fmt.Errorf("desired LRP for app %s not found", appGUID)
}

// GetAllLRPs returns all Actual LRPs (in mapping {appGuid: []ActualLRP}) and all Desired LRPs
func (bc *BBSCache) GetAllLRPs() (map[string][]*ActualLRP, map[string]*DesiredLRP) {
	bc.RLock()
	defer bc.RUnlock()
	return bc.actualLRPsByAppGUID, bc.desiredLRPs
}

// GetTagsForNode returns tags for all container running on specified node
func (bc *BBSCache) GetTagsForNode(nodename string) (map[string][]string, error) {
	bc.RLock()
	defer bc.RUnlock()
	if tags, ok := bc.tagsByCellID[nodename]; ok {
		return tags, nil
	}
	return map[string][]string{}, fmt.Errorf("could not find tags for node %s", nodename)
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
	var actualLRPsByAppGUID map[string][]*ActualLRP
	var actualLRPsByCellID map[string][]*ActualLRP
	var desiredLRPs map[string]*DesiredLRP
	var errActual, errDesired error

	wg.Add(2)

	go func() {
		actualLRPsByAppGUID, actualLRPsByCellID, errActual = bc.readActualLRPs()
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
	bc.actualLRPsByAppGUID = actualLRPsByAppGUID
	bc.actualLRPsByCellID = actualLRPsByCellID
	bc.desiredLRPs = desiredLRPs
	tagsByCellID := map[string]map[string][]string{}
	for cellID, alrps := range actualLRPsByCellID {
		tagsByCellID[cellID] = bc.extractNodeTags(alrps, desiredLRPs)
	}
	bc.tagsByCellID = tagsByCellID
	bc.lastUpdated = time.Now()
	bc.pollSuccesses++
}

func (bc *BBSCache) readActualLRPs() (map[string][]*ActualLRP, map[string][]*ActualLRP, error) {
	actualLRPsByAppGUID := map[string][]*ActualLRP{}
	actualLRPsByCellID := map[string][]*ActualLRP{}
	actualLRPsBBS, err := bc.bbsAPIClient.ActualLRPs(bc.bbsAPIClientLogger, models.ActualLRPFilter{})
	if err != nil {
		return actualLRPsByAppGUID, actualLRPsByCellID, err
	}
	for _, lrp := range actualLRPsBBS {
		alrp := ActualLRPFromBBSModel(lrp)
		if _, ok := actualLRPsByAppGUID[alrp.AppGUID]; ok {
			actualLRPsByAppGUID[alrp.AppGUID] = append(actualLRPsByAppGUID[alrp.AppGUID], &alrp)
		} else {
			actualLRPsByAppGUID[alrp.AppGUID] = []*ActualLRP{&alrp}
		}
		if _, ok := actualLRPsByCellID[alrp.CellID]; ok {
			actualLRPsByCellID[alrp.CellID] = append(actualLRPsByCellID[alrp.CellID], &alrp)
		} else {
			actualLRPsByCellID[alrp.CellID] = []*ActualLRP{&alrp}
		}
	}
	log.Debugf("Successfully read %d Actual LRPs", len(actualLRPsBBS))
	return actualLRPsByAppGUID, actualLRPsByCellID, nil
}

func (bc *BBSCache) readDesiredLRPs() (map[string]*DesiredLRP, error) {
	desiredLRPsBBS, err := bc.bbsAPIClient.DesiredLRPs(bc.bbsAPIClientLogger, models.DesiredLRPFilter{})
	if err != nil {
		return map[string]*DesiredLRP{}, err
	}
	desiredLRPs := make(map[string]*DesiredLRP, len(desiredLRPsBBS))
	for _, lrp := range desiredLRPsBBS {
		desiredLRP := DesiredLRPFromBBSModel(lrp)
		desiredLRPs[desiredLRP.AppGUID] = &desiredLRP
	}
	log.Debugf("Successfully read %d Desired LRPs", len(desiredLRPsBBS))
	return desiredLRPs, nil
}

// extractNodeTags extract all the container tags for each app in nodeActualLRPs
// and returns a mapping of tags by instance GUID
func (bc *BBSCache) extractNodeTags(nodeActualLRPs []*ActualLRP, desiredLRPsByAppGUID map[string]*DesiredLRP) map[string][]string {
	tags := map[string][]string{}
	for _, alrp := range nodeActualLRPs {
		dlrp, ok := desiredLRPsByAppGUID[alrp.AppGUID]
		if !ok {
			log.Debugf("Could not find desired LRP for app GUID %s", alrp.AppGUID)
			continue
		}
		vcApp := dlrp.EnvVcapApplication
		appName, ok := vcApp[ApplicationNameKey]
		if !ok {
			log.Debugf("Could not find application_name of app %s", alrp.AppGUID)
			continue
		}
		tags[alrp.InstanceGUID] = []string{
			fmt.Sprintf("%s:%s_%d", ContainerNameTagKey, appName, alrp.Index),
			fmt.Sprintf("%s:%s", AppNameTagKey, appName),
			fmt.Sprintf("%s:%s", AppGUIDTagKey, alrp.AppGUID),
			fmt.Sprintf("%s:%d", AppInstanceIndexTagKey, alrp.Index),
			fmt.Sprintf("%s:%s", AppInstanceGUIDTagKey, alrp.InstanceGUID),
		}
	}
	return tags
}
