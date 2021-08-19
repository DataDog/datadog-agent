// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build clusterchecks

package cloudfoundry

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cloudfoundry-community/go-cfclient"
)

// CCCacheI is an interface for a structure that caches and automatically refreshes data from Cloud Foundry API
// it's useful mostly to be able to mock CCCache during unit tests
type CCCacheI interface {
	LastUpdated() time.Time
	GetPollAttempts() int
	GetPollSuccesses() int
	GetApp(string) (*cfclient.V3App, error)
}

// CCCache is a simple structure that caches and automatically refreshes data from Cloud Foundry API
type CCCache struct {
	sync.RWMutex
	pollAttempts  int64
	pollSuccesses int64
	cancelContext context.Context
	configured    bool
	ccAPIClient   CCClientI
	pollInterval  time.Duration
	lastUpdated   time.Time
	appsByGUID    map[string]*CFApp
	orgsByGUID    map[string]*CFOrg
	spacesByGUID  map[string]*CFSpace
	appsBatchSize int
}

type CCClientI interface {
	ListV3AppsByQuery(url.Values) ([]cfclient.V3App, error)
	ListV3OrganizationsByQuery(url.Values) ([]cfclient.V3Organization, error)
	ListV3SpacesByQuery(url.Values) ([]cfclient.V3Space, error)
}

var globalCCCache = &CCCache{}

// ConfigureGlobalCCCache configures the global instance of CCCache from provided config
func ConfigureGlobalCCCache(ctx context.Context, ccURL, ccClientID, ccClientSecret string, skipSSLValidation bool, pollInterval time.Duration, appsBatchSize int, testing CCClientI) (*CCCache, error) {
	globalCCCache.Lock()
	defer globalCCCache.Unlock()

	if globalCCCache.configured {
		return globalCCCache, nil
	}

	if testing != nil {
		globalCCCache.ccAPIClient = testing
	} else {
		clientConfig := &cfclient.Config{
			ApiAddress:        ccURL,
			ClientID:          ccClientID,
			ClientSecret:      ccClientSecret,
			SkipSslValidation: skipSSLValidation,
		}
		var err error
		globalCCCache.ccAPIClient, err = cfclient.NewClient(clientConfig)
		if err != nil {
			return nil, err
		}
	}

	globalCCCache.pollInterval = pollInterval
	globalCCCache.appsBatchSize = appsBatchSize
	globalCCCache.lastUpdated = time.Time{} // zero time
	globalCCCache.cancelContext = ctx
	globalCCCache.configured = true

	go globalCCCache.start()

	return globalCCCache, nil
}

// GetGlobalCCCache returns the global instance of CCCache (or error if the instance is not configured yet)
func GetGlobalCCCache() (*CCCache, error) {
	globalCCCache.Lock()
	defer globalCCCache.Unlock()
	if !globalCCCache.configured {
		return nil, fmt.Errorf("global CC Cache not configured")
	}
	return globalCCCache, nil
}

// LastUpdated return the last time the cache was updated
func (ccc *CCCache) LastUpdated() time.Time {
	ccc.RLock()
	defer ccc.RUnlock()
	return ccc.lastUpdated
}

// GetPollAttempts returns the number of times the cache queried the CC API
func (ccc *CCCache) GetPollAttempts() int64 {
	return atomic.LoadInt64(&ccc.pollAttempts)
}

// GetPollSuccesses returns the number of times the cache successfully queried the CC API
func (ccc *CCCache) GetPollSuccesses() int64 {
	return atomic.LoadInt64(&ccc.pollSuccesses)
}

func (ccc *CCCache) GetApp(guid string) (*CFApp, error) {
	ccc.RLock()
	defer ccc.RUnlock()
	app, ok := ccc.appsByGUID[guid]
	if !ok {
		return nil, fmt.Errorf("could not find app %s in cloud controller cache", guid)
	}
	return app, nil
}

func (ccc *CCCache) GetSpace(guid string) (*CFSpace, error) {
	ccc.RLock()
	defer ccc.RUnlock()
	space, ok := ccc.spacesByGUID[guid]
	if !ok {
		return nil, fmt.Errorf("could not find space %s in cloud controller cache", guid)
	}
	return space, nil
}

func (ccc *CCCache) GetOrg(guid string) (*CFOrg, error) {
	ccc.RLock()
	defer ccc.RUnlock()
	org, ok := ccc.orgsByGUID[guid]
	if !ok {
		return nil, fmt.Errorf("could not find org %s in cloud controller cache", guid)
	}
	return org, nil
}

func (ccc *CCCache) start() {
	ccc.readData()
	dataRefreshTicker := time.NewTicker(ccc.pollInterval)
	for {
		select {
		case <-dataRefreshTicker.C:
			ccc.readData()
		case <-ccc.cancelContext.Done():
			dataRefreshTicker.Stop()
			return
		}
	}
}

func (ccc *CCCache) readData() {
	log.Debug("Reading data from CC API")
	atomic.AddInt64(&ccc.pollAttempts, 1)
	var wg sync.WaitGroup

	// List applications
	wg.Add(1)
	var appsByGUID map[string]*CFApp
	go func() {
		defer wg.Done()
		query := url.Values{}
		query.Add("per_page", fmt.Sprintf("%d", ccc.appsBatchSize))
		apps, err := ccc.ccAPIClient.ListV3AppsByQuery(query)
		if err != nil {
			log.Errorf("Failed listing apps from cloud controller: %v", err)
			return
		}
		appsByGUID = make(map[string]*CFApp, len(apps))
		for _, app := range apps {
			appsByGUID[app.GUID] = CFAppFromV3App(&app)
		}
	}()

	// List spaces
	wg.Add(1)
	var spacesByGUID map[string]*CFSpace
	go func() {
		defer wg.Done()
		query := url.Values{}
		query.Add("per_page", fmt.Sprintf("%d", ccc.appsBatchSize))
		spaces, err := ccc.ccAPIClient.ListV3SpacesByQuery(query)
		if err != nil {
			log.Errorf("Failed listing spaces from cloud controller: %v", err)
			return
		}
		spacesByGUID = make(map[string]*CFSpace, len(spaces))
		for _, space := range spaces {
			spacesByGUID[space.GUID] = CFSpaceFromV3Space(&space)
		}
	}()

	// List orgs
	wg.Add(1)
	var orgsByGUID map[string]*CFOrg
	go func() {
		defer wg.Done()
		query := url.Values{}
		query.Add("per_page", fmt.Sprintf("%d", ccc.appsBatchSize))
		orgs, err := ccc.ccAPIClient.ListV3OrganizationsByQuery(query)
		if err != nil {
			log.Errorf("Failed listing orgs from cloud controller: %v", err)
			return
		}
		orgsByGUID = make(map[string]*CFOrg, len(orgs))
		for _, org := range orgs {
			orgsByGUID[org.GUID] = CFOrgFromV3Organization(&org)
		}
	}()

	// put new data in cache
	wg.Wait()
	ccc.Lock()
	defer ccc.Unlock()
	ccc.appsByGUID = appsByGUID
	ccc.spacesByGUID = spacesByGUID
	ccc.orgsByGUID = orgsByGUID
	atomic.AddInt64(&ccc.pollSuccesses, 1)
	ccc.lastUpdated = time.Now()
}
