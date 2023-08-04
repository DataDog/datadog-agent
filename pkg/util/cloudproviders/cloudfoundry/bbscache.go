// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package cloudfoundry

import (
	"context"
	"fmt"
	"regexp"
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
	// LastUpdated return the last time the cache was updated
	LastUpdated() time.Time

	// UpdatedOnce returns a channel that is closed once the cache has been updated
	// successfully at least once.  Successive calls to UpdatedOnce return the
	// same channel.  If the cache's context ends before an update occurs, this channel
	// will never close.
	UpdatedOnce() <-chan struct{}

	// GetActualLRPsForProcessGUID returns slice of pointers to ActualLRP objects for given process GUID
	GetActualLRPsForProcessGUID(processGUID string) ([]*ActualLRP, error)

	// GetActualLRPsForCell returns slice of pointers to ActualLRP objects for given cell GUID
	GetActualLRPsForCell(cellID string) ([]*ActualLRP, error)

	// GetDesiredLRPFor returns DesiredLRP for a specific app GUID
	GetDesiredLRPFor(appGUID string) (DesiredLRP, error)

	// GetAllLRPs returns all Actual LRPs (in mapping {appGuid: []ActualLRP}) and all Desired LRPs
	GetAllLRPs() (map[string][]*ActualLRP, map[string]*DesiredLRP)

	// GetTagsForNode returns tags for all container running on specified node
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
	envIncludeList     []*regexp.Regexp
	envExcludeList     []*regexp.Regexp
	// maps Desired LRPs' AppGUID to list of ActualLRPs (IOW this is list of running containers per app)
	actualLRPsByProcessGUID map[string][]*ActualLRP
	actualLRPsByCellID      map[string][]*ActualLRP
	desiredLRPs             map[string]*DesiredLRP
	tagsByCellID            map[string]map[string][]string
	lastUpdated             time.Time
	updatedOnce             chan struct{}
}

var (
	globalBBSCache     = &BBSCache{}
	globalBBSCacheLock sync.Mutex
)

// ConfigureGlobalBBSCache configures the global instance of BBSCache from provided config
func ConfigureGlobalBBSCache(ctx context.Context, bbsURL, cafile, certfile, keyfile string, pollInterval time.Duration, includeList, excludeList []*regexp.Regexp, testing bbs.Client) (*BBSCache, error) {
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
	globalBBSCache.updatedOnce = make(chan struct{})
	globalBBSCache.cancelContext = ctx
	globalBBSCache.envIncludeList = includeList
	globalBBSCache.envExcludeList = excludeList

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

// UpdatedOnce returns a channel that is closed once the cache has been updated
// successfully at least once.  Successive calls to UpdatedOnce return the
// same channel.  If the cache's context ends before an update occurs, this channel
// will never close.
func (bc *BBSCache) UpdatedOnce() <-chan struct{} {
	return bc.updatedOnce
}

// GetActualLRPsForProcessGUID returns slice of pointers to ActualLRP objects for given process GUID
func (bc *BBSCache) GetActualLRPsForProcessGUID(processGUID string) ([]*ActualLRP, error) {
	bc.RLock()
	defer bc.RUnlock()
	if val, ok := bc.actualLRPsByProcessGUID[processGUID]; ok {
		return val, nil
	}
	return []*ActualLRP{}, fmt.Errorf("actual LRPs for app %s not found", processGUID)
}

// GetActualLRPsForCell returns slice of pointers to ActualLRP objects for given cell GUID
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
	return bc.actualLRPsByProcessGUID, bc.desiredLRPs
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
	var wg sync.WaitGroup
	var actualLRPsByProcessGUID map[string][]*ActualLRP
	var actualLRPsByCellID map[string][]*ActualLRP
	var desiredLRPs map[string]*DesiredLRP
	var errActual, errDesired error

	wg.Add(2)
	go func() {
		defer wg.Done()
		actualLRPsByProcessGUID, actualLRPsByCellID, errActual = bc.readActualLRPs()
	}()
	go func() {
		defer wg.Done()
		desiredLRPs, errDesired = bc.readDesiredLRPs()
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
	bc.actualLRPsByProcessGUID = actualLRPsByProcessGUID
	bc.actualLRPsByCellID = actualLRPsByCellID
	bc.desiredLRPs = desiredLRPs
	tagsByCellID := map[string]map[string][]string{}
	for cellID, alrps := range actualLRPsByCellID {
		tagsByCellID[cellID] = bc.extractNodeTags(alrps, desiredLRPs)
	}
	bc.tagsByCellID = tagsByCellID
	firstUpdate := bc.lastUpdated.IsZero()
	bc.lastUpdated = time.Now()
	if firstUpdate {
		close(bc.updatedOnce)
	}
}

func (bc *BBSCache) readActualLRPs() (map[string][]*ActualLRP, map[string][]*ActualLRP, error) {
	actualLRPsByProcessGUID := map[string][]*ActualLRP{}
	actualLRPsByCellID := map[string][]*ActualLRP{}
	actualLRPsBBS, err := bc.bbsAPIClient.ActualLRPs(bc.bbsAPIClientLogger, models.ActualLRPFilter{})
	if err != nil {
		return actualLRPsByProcessGUID, actualLRPsByCellID, err
	}
	for _, lrp := range actualLRPsBBS {
		alrp := ActualLRPFromBBSModel(lrp)
		if _, ok := actualLRPsByProcessGUID[alrp.ProcessGUID]; ok {
			actualLRPsByProcessGUID[alrp.ProcessGUID] = append(actualLRPsByProcessGUID[alrp.ProcessGUID], &alrp)
		} else {
			actualLRPsByProcessGUID[alrp.ProcessGUID] = []*ActualLRP{&alrp}
		}
		if _, ok := actualLRPsByCellID[alrp.CellID]; ok {
			actualLRPsByCellID[alrp.CellID] = append(actualLRPsByCellID[alrp.CellID], &alrp)
		} else {
			actualLRPsByCellID[alrp.CellID] = []*ActualLRP{&alrp}
		}
	}
	log.Debugf("Successfully read %d Actual LRPs", len(actualLRPsBBS))
	return actualLRPsByProcessGUID, actualLRPsByCellID, nil
}

func (bc *BBSCache) readDesiredLRPs() (map[string]*DesiredLRP, error) {
	desiredLRPsBBS, err := bc.bbsAPIClient.DesiredLRPs(bc.bbsAPIClientLogger, models.DesiredLRPFilter{})
	if err != nil {
		return map[string]*DesiredLRP{}, err
	}
	desiredLRPs := make(map[string]*DesiredLRP, len(desiredLRPsBBS))
	for _, lrp := range desiredLRPsBBS {
		desiredLRP := DesiredLRPFromBBSModel(lrp, bc.envIncludeList, bc.envExcludeList)
		desiredLRPs[desiredLRP.ProcessGUID] = &desiredLRP
	}
	log.Debugf("Successfully read %d Desired LRPs", len(desiredLRPsBBS))
	return desiredLRPs, nil
}

// extractNodeTags extract all the container tags for each app in nodeActualLRPs
// and returns a mapping of tags by instance GUID
func (bc *BBSCache) extractNodeTags(nodeActualLRPs []*ActualLRP, desiredLRPsByProcessGUID map[string]*DesiredLRP) map[string][]string {
	tags := map[string][]string{}
	for _, alrp := range nodeActualLRPs {
		dlrp, ok := desiredLRPsByProcessGUID[alrp.ProcessGUID]
		if !ok {
			log.Debugf("Could not find desired LRP for process GUID %s", alrp.ProcessGUID)
			continue
		}
		tags[alrp.InstanceGUID] = []string{
			fmt.Sprintf("%s:%s_%d", ContainerNameTagKey, dlrp.AppName, alrp.Index),
			fmt.Sprintf("%s:%d", AppInstanceIndexTagKey, alrp.Index),
			fmt.Sprintf("%s:%s", AppInstanceGUIDTagKey, alrp.InstanceGUID),
		}
		tags[alrp.InstanceGUID] = append(tags[alrp.InstanceGUID], dlrp.GetTagsFromDLRP()...)
	}
	return tags
}
