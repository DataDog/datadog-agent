package cloudfoundry

import (
	"context"
	"fmt"
	"net/url"
	"sync"
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
	GetApps(string) (*cfclient.V3App, error)
}

// CCCache is a simple structure that caches and automatically refreshes data from Cloud Foundry API
type CCCache struct {
	sync.RWMutex
	cancelContext context.Context
	configured    bool
	ccAPIClient   *cfclient.Client
	pollInterval  time.Duration
	pollAttempts  int
	pollSuccesses int
	lastUpdated   time.Time
	appsByGuid    map[string]*cfclient.V3App
}

var (
	globalCCCache     = &CCCache{}
	globalCCCacheLock sync.Mutex
)

// ConfigureGlobalCCCache configures the global instance of CCCache from provided config
func ConfigureGlobalCCCache(ctx context.Context, ccURL, ccClientId, ccClientSecret string, skipSSLValidation bool, pollInterval time.Duration, testing *cfclient.Client) (*CCCache, error) {
	globalCCCacheLock.Lock()
	defer globalCCCacheLock.Unlock()

	if globalCCCache.configured {
		return globalCCCache, nil
	}

	if testing != nil {
		globalCCCache.ccAPIClient = testing
	} else {
		clientConfig := &cfclient.Config{
			ApiAddress:        ccURL,
			ClientID:          ccClientId,
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
func (ccc *CCCache) GetPollAttempts() int {
	ccc.RLock()
	defer ccc.RUnlock()
	return ccc.pollAttempts
}

// GetPollSuccesses returns the number of times the cache successfully queried the CC API
func (ccc *CCCache) GetPollSuccesses() int {
	ccc.RLock()
	defer ccc.RUnlock()
	return ccc.pollSuccesses
}

func (ccc *CCCache) GetApp(guid string) (*cfclient.V3App, error) {
	ccc.RLock()
	defer ccc.RUnlock()
	app, ok := ccc.appsByGuid[guid]
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
	ccc.Lock()
	ccc.pollAttempts++
	ccc.Unlock()

	query := url.Values{}
	query.Add("per_page", "5000")
	apps, err := ccc.ccAPIClient.ListV3AppsByQuery(query)
	if err != nil {
		_ = log.Errorf("Failed listing apps from cloud controller: %v", err)
		return
	}
	appsByGuid := make(map[string]*cfclient.V3App, len(apps))
	for i, app := range apps {
		appsByGuid[app.GUID] = &apps[i] // can't point to the for loop variable, as it is reused, use indexed value
	}

	// put new apps in cache
	ccc.Lock()
	defer ccc.Unlock()
	ccc.appsByGuid = appsByGuid
	ccc.pollSuccesses++
	ccc.lastUpdated = time.Now()
}
