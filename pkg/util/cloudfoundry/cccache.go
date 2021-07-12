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
}

type CCClientI interface {
	ListV3AppsByQuery(url.Values) ([]cfclient.V3App, error)
}

var globalCCCache = &CCCache{}

// ConfigureGlobalCCCache configures the global instance of CCCache from provided config
func ConfigureGlobalCCCache(ctx context.Context, ccURL, ccClientID, ccClientSecret string, skipSSLValidation bool, pollInterval time.Duration, testing CCClientI) (*CCCache, error) {
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

	query := url.Values{}
	query.Add("per_page", "5000")
	apps, err := ccc.ccAPIClient.ListV3AppsByQuery(query)
	if err != nil {
		log.Errorf("Failed listing apps from cloud controller: %v", err)
		return
	}
	appsByGUID := make(map[string]*CFApp, len(apps))
	for _, app := range apps {
		appsByGUID[app.GUID] = CFAppFromV3App(&app)
	}

	// put new apps in cache
	ccc.Lock()
	defer ccc.Unlock()
	ccc.appsByGUID = appsByGUID
	atomic.AddInt64(&ccc.pollSuccesses, 1)
	ccc.lastUpdated = time.Now()
}
