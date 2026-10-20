// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && !kubeapiserver

package cloudfoundry

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cloudfoundry-community/go-cfclient/v2"
	"golang.org/x/sync/singleflight"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CCCacheI is an interface for a structure that caches and automatically refreshes data from Cloud Foundry API
// it's useful mostly to be able to mock CCCache during unit tests
type CCCacheI interface {
	// LastUpdated return the last time the cache was updated
	LastUpdated() time.Time

	// UpdatedOnce returns a channel that is closed once the cache has been updated
	// successfully at least once.  Successive calls to UpdatedOnce return the
	// same channel.  If the cache's context ends before an update occurs, this channel
	// will never close.
	UpdatedOnce() <-chan struct{}

	// GetApp looks for an app with the given GUID in the cache
	GetApp(string) (*cfclient.V3App, error)

	// GetSpace looks for a space with the given GUID in the cache
	GetSpace(string) (*cfclient.V3Space, error)

	// GetOrg looks for an org with the given GUID in the cache
	GetOrg(string) (*cfclient.V3Organization, error)

	// GetOrgs returns all orgs in the cache
	GetOrgs() ([]*cfclient.V3Organization, error)

	// GetOrgQuotas returns all orgs quotas in the cache
	GetOrgQuotas() ([]*CFOrgQuota, error)

	// GetProcesses returns all processes for the given app guid in the cache
	GetProcesses(appGUID string) ([]*cfclient.Process, error)

	// GetCFApplication looks for a CF application with the given GUID in the cache
	GetCFApplication(string) (*CFApplication, error)

	// GetCFApplications returns all CF applications in the cache
	GetCFApplications() ([]*CFApplication, error)

	// GetSidecars returns all sidecars for the given application GUID in the cache
	GetSidecars(string) ([]*CFSidecar, error)

	// GetIsolationSegmentForSpace returns an isolation segment for the given GUID in the cache
	GetIsolationSegmentForSpace(string) (*cfclient.IsolationSegment, error)

	// GetIsolationSegmentForOrg returns an isolation segment for the given GUID in the cache
	GetIsolationSegmentForOrg(string) (*cfclient.IsolationSegment, error)

	// SidecarsTagsEnabled returns whether sidecar tags are enabled
	SidecarsTagsEnabled() bool

	// SegmentsTagsEnabled returns whether isolation segment tags are enabled
	SegmentsTagsEnabled() bool
}

// CCCache is a simple structure that caches and automatically refreshes data from Cloud Foundry API
type CCCache struct {
	sync.RWMutex
	cancelContext        context.Context
	configured           bool
	config               CCCacheConfig
	lastUpdated          time.Time
	updatedOnce          chan struct{}
	appsByGUID           map[string]*cfclient.V3App
	orgsByGUID           map[string]*cfclient.V3Organization
	orgQuotasByGUID      map[string]*CFOrgQuota
	spacesByGUID         map[string]*cfclient.V3Space
	processesByAppGUID   map[string][]*cfclient.Process
	cfApplicationsByGUID map[string]*CFApplication
	sidecarsByAppGUID    map[string][]*CFSidecar
	segmentBySpaceGUID   map[string]*cfclient.IsolationSegment
	segmentByOrgGUID     map[string]*cfclient.IsolationSegment
	requestGroup         singleflight.Group
	retryBackoff         time.Duration
}

// CCClientI is an interface for a Cloud Foundry Client that queries the Cloud Foundry API
type CCClientI interface {
	ListV3AppsByQuery(url.Values) ([]cfclient.V3App, error)
	ListV3OrganizationsByQuery(url.Values) ([]cfclient.V3Organization, error)
	ListV3SpacesByQuery(url.Values) ([]cfclient.V3Space, error)
	ListAllProcessesByQuery(url.Values) ([]cfclient.Process, error)
	ListOrgQuotasByQuery(url.Values) ([]cfclient.OrgQuota, error)
	ListSidecarsByApp(url.Values, string) ([]CFSidecar, error)
	ListIsolationSegmentsByQuery(url.Values) ([]cfclient.IsolationSegment, error)
	GetIsolationSegmentSpaceGUID(string) (string, error)
	GetIsolationSegmentOrganizationGUID(string) (string, error)
	GetV3AppByGUID(string) (*cfclient.V3App, error)
	GetV3SpaceByGUID(string) (*cfclient.V3Space, error)
	GetV3OrganizationByGUID(string) (*cfclient.V3Organization, error)
	ListProcessByAppGUID(url.Values, string) ([]cfclient.Process, error)
}

var globalCCCache = &CCCache{}

const (
	ccListAttempts     = 3
	ccListRetryBackoff = time.Second
)

// retryableCCErrorCodes are the Cloud Controller error codes that map to a 5xx or 429 HTTP status and
// can be returned by list endpoints, see https://github.com/cloudfoundry/cloud_controller_ng/tree/main/errors
var retryableCCErrorCodes = map[int]struct{}{
	10001: {}, // ServerError (500)
	10011: {}, // DatabaseError (500)
	10013: {}, // RateLimitExceeded (429)
	10014: {}, // IPBasedRateLimitExceeded (429)
	10015: {}, // ServiceUnavailable (503)
	10018: {}, // RateLimitV2APIExceeded (429)
	10081: {}, // InternalServerError (500)
	20004: {}, // UaaUnavailable (503)
	20008: {}, // UaaRateLimited (429)
}

// CCCacheConfig holds configuration for CCCache
type CCCacheConfig struct {
	CCAPIClient        CCClientI     // Cloud Controller API client
	PollInterval       time.Duration // Interval between cache refresh polls
	AppsBatchSize      int           // Number of apps to fetch per API request
	RefreshCacheOnMiss bool          // Whether to refresh cache on cache miss
	ServeNozzleData    bool          // Whether to prepare CFApplication data for the nozzle
	SidecarsTags       bool          // Whether to include sidecar tags
	SegmentsTags       bool          // Whether to include isolation segment tags
}

// ConfigureGlobalCCCache configures the global instance of CCCache from provided config
func ConfigureGlobalCCCache(ctx context.Context, config CCCacheConfig) (*CCCache, error) {
	globalCCCache.Lock()
	defer globalCCCache.Unlock()

	if globalCCCache.configured {
		return globalCCCache, nil
	}

	globalCCCache.config = config
	globalCCCache.lastUpdated = time.Time{} // zero time
	globalCCCache.updatedOnce = make(chan struct{})
	globalCCCache.cancelContext = ctx
	globalCCCache.retryBackoff = ccListRetryBackoff
	globalCCCache.configured = true

	go globalCCCache.start()

	return globalCCCache, nil
}

// GetGlobalCCCache returns the global instance of CCCache (or error if the instance is not configured yet)
func GetGlobalCCCache() (*CCCache, error) {
	globalCCCache.RLock()
	defer globalCCCache.RUnlock()
	if !globalCCCache.configured {
		return nil, errors.New("global CC Cache not configured")
	}
	return globalCCCache, nil
}

// LastUpdated return the last time the cache was updated
func (ccc *CCCache) LastUpdated() time.Time {
	ccc.RLock()
	defer ccc.RUnlock()
	return ccc.lastUpdated
}

// UpdatedOnce returns a channel that is closed once the cache has been updated
// successfully at least once.  Successive calls to UpdatedOnce return the
// same channel.  If the cache's context ends before an update occurs, this channel
// will never close.
func (ccc *CCCache) UpdatedOnce() <-chan struct{} {
	return ccc.updatedOnce
}

// getResource looks up the given resourceName/GUID in the CCCache
// If not found and refreshOnCacheMiss is enabled, it will use the fetchFn function to fetch the resource from the CAPI
func getResource[T any](ccc *CCCache, resourceName, guid string, cache map[string]T, fetchFn func(string) (T, error)) (T, error) {
	var resource T

	// check if the cccache is still warming up and read from cache
	ccc.RLock()
	updatedOnce := !ccc.lastUpdated.IsZero()
	if !updatedOnce {
		ccc.RUnlock()
		return resource, errors.New("cannot refresh cache on miss, cccache is still warming up")
	}
	resource, ok := cache[guid]
	ccc.RUnlock()

	if ok {
		return resource, nil
	}

	if !ccc.config.RefreshCacheOnMiss {
		return resource, fmt.Errorf("could not find resource '%s' with guid '%s' in cloud controller cache, consider enabling `refreshCacheOnMiss`", resourceName, guid)
	}

	// Note: even though `guid` is globally unique, in our case we have a collision between two resources
	// cfclient.V3App and CFapplications since they represent the same underlying resource
	// we need to use a key in the form `resourceName/guid` to prevent collisions
	key := resourceName + "/" + guid

	// Use singleflight to prevent duplicate API fetches for the same resource
	val, err, _ := ccc.requestGroup.Do(key, func() (interface{}, error) {
		// fetch the resource from the CAPI (without holding locks)
		fetchedResource, err := fetchFn(guid)
		if err != nil {
			return fetchedResource, err
		}

		// update cache under global Lock
		// A cache stays nil until its first successful poll, so skip storing rather than writing to a nil map.
		ccc.Lock()
		if cache != nil {
			cache[guid] = fetchedResource
		}
		ccc.Unlock()

		return fetchedResource, nil
	})

	if err != nil {
		return resource, err
	}

	return val.(T), nil
}

// GetOrgs returns all orgs in the cache
func (ccc *CCCache) GetOrgs() ([]*cfclient.V3Organization, error) {
	ccc.RLock()
	defer ccc.RUnlock()

	var orgs []*cfclient.V3Organization
	for _, org := range ccc.orgsByGUID {
		orgs = append(orgs, org)
	}

	return orgs, nil
}

// GetOrgQuotas returns all org quotas in the cache
func (ccc *CCCache) GetOrgQuotas() ([]*CFOrgQuota, error) {
	ccc.RLock()
	defer ccc.RUnlock()

	var orgQuotas []*CFOrgQuota
	for _, org := range ccc.orgQuotasByGUID {
		orgQuotas = append(orgQuotas, org)
	}

	return orgQuotas, nil
}

// GetCFApplications returns all CF applications in the cache
func (ccc *CCCache) GetCFApplications() ([]*CFApplication, error) {
	ccc.RLock()
	defer ccc.RUnlock()

	var cfapps []*CFApplication
	for _, cfapp := range ccc.cfApplicationsByGUID {
		cfapps = append(cfapps, cfapp)
	}

	return cfapps, nil
}

// GetProcesses returns all processes for the given app guid in the cache
func (ccc *CCCache) GetProcesses(appGUID string) ([]*cfclient.Process, error) {
	processes, err := getResource(ccc, "Processes", appGUID, ccc.processesByAppGUID, ccc.fetchProcessesByAppGUID)
	if err != nil {
		return nil, err
	}
	return processes, nil
}

// GetCFApplication looks for a CF application with the given GUID in the cache
func (ccc *CCCache) GetCFApplication(guid string) (*CFApplication, error) {
	cfapp, err := getResource(ccc, "CFApplication", guid, ccc.cfApplicationsByGUID, ccc.fetchCFApplicationByGUID)
	if err != nil {
		return nil, err
	}
	return cfapp, nil
}

// GetSidecars looks for sidecars of an app with the given GUID in the cache
func (ccc *CCCache) GetSidecars(guid string) ([]*CFSidecar, error) {
	ccc.RLock()
	defer ccc.RUnlock()

	sidecars, ok := ccc.sidecarsByAppGUID[guid]
	if !ok {
		return nil, fmt.Errorf("could not find sidecars for app %s in cloud controller cache", guid)
	}
	return sidecars, nil
}

// GetApp looks for an app with the given GUID in the cache
func (ccc *CCCache) GetApp(guid string) (*cfclient.V3App, error) {
	app, err := getResource(ccc, "App", guid, ccc.appsByGUID, ccc.config.CCAPIClient.GetV3AppByGUID)
	if err != nil {
		return nil, err
	}
	return app, nil
}

// GetSpace looks for a space with the given GUID in the cache
func (ccc *CCCache) GetSpace(guid string) (*cfclient.V3Space, error) {
	space, err := getResource(ccc, "Space", guid, ccc.spacesByGUID, ccc.config.CCAPIClient.GetV3SpaceByGUID)
	if err != nil {
		return nil, err
	}
	return space, nil
}

// GetOrg looks for an org with the given GUID in the cache
func (ccc *CCCache) GetOrg(guid string) (*cfclient.V3Organization, error) {
	org, err := getResource(ccc, "Org", guid, ccc.orgsByGUID, ccc.config.CCAPIClient.GetV3OrganizationByGUID)
	if err != nil {
		return nil, err
	}
	return org, nil
}

// GetIsolationSegmentForSpace returns an isolation segment for the given GUID in the cache
func (ccc *CCCache) GetIsolationSegmentForSpace(guid string) (*cfclient.IsolationSegment, error) {
	ccc.RLock()
	defer ccc.RUnlock()
	segment, ok := ccc.segmentBySpaceGUID[guid]
	if !ok {
		return nil, fmt.Errorf("could not find isolation segment for space %s in cloud controller cache", guid)
	}
	return segment, nil
}

// GetIsolationSegmentForOrg returns an isolation segment for the given GUID in the cache
func (ccc *CCCache) GetIsolationSegmentForOrg(guid string) (*cfclient.IsolationSegment, error) {
	ccc.RLock()
	defer ccc.RUnlock()
	segment, ok := ccc.segmentByOrgGUID[guid]
	if !ok {
		return nil, fmt.Errorf("could not find isolation segment for organization %s in cloud controller cache", guid)
	}
	return segment, nil
}

// SidecarsTagsEnabled returns whether sidecar tags are enabled
func (ccc *CCCache) SidecarsTagsEnabled() bool {
	return ccc.config.SidecarsTags
}

// SegmentsTagsEnabled returns whether isolation segment tags are enabled
func (ccc *CCCache) SegmentsTagsEnabled() bool {
	return ccc.config.SegmentsTags
}

func (ccc *CCCache) fetchProcessesByAppGUID(appGUID string) ([]*cfclient.Process, error) {
	query := url.Values{}
	query.Add("per_page", strconv.Itoa(ccc.config.AppsBatchSize))

	// fetch processes from the CAPI
	processes, err := ccc.config.CCAPIClient.ListProcessByAppGUID(query, appGUID)
	if err != nil {
		return nil, err
	}

	// convert to array of pointers
	res := make([]*cfclient.Process, 0, len(processes))
	for _, process := range processes {
		res = append(res, &process)
	}
	return res, nil
}

func (ccc *CCCache) fetchCFApplicationByGUID(guid string) (*CFApplication, error) {
	// fetch app from the CAPI
	app, err := ccc.GetApp(guid)
	if err != nil {
		return nil, err
	}

	// fill app data
	cfapp := CFApplication{}
	cfapp.extractDataFromV3App(*app)

	// extract GUIDs
	appGUID := cfapp.GUID
	spaceGUID := cfapp.SpaceGUID
	orgGUID := cfapp.OrgGUID

	// fill processes data
	processes, err := ccc.GetProcesses(appGUID)
	if err != nil {
		log.Info(err)
	} else {
		cfapp.extractDataFromV3Process(processes)
	}

	// fill space then org data. Order matters for labels and annotations.
	space, err := ccc.GetSpace(spaceGUID)
	if err != nil {
		log.Info(err)
	} else {
		cfapp.extractDataFromV3Space(space)
	}

	// fill org data
	org, err := ccc.GetOrg(orgGUID)
	if err != nil {
		log.Info(err)
	} else {
		cfapp.extractDataFromV3Org(org)
	}

	// fill sidecars data
	sidecars, err := ccc.GetSidecars(appGUID)
	if err != nil {
		log.Info(err)
	} else {
		for _, sidecar := range sidecars {
			cfapp.Sidecars = append(cfapp.Sidecars, *sidecar)
		}
	}
	return &cfapp, nil
}

func (ccc *CCCache) listApplications(wg *sync.WaitGroup, appsMap *map[string]*cfclient.V3App, sidecarsMap *map[string][]*CFSidecar) {
	wg.Add(1)

	var apps []cfclient.V3App
	var err error

	go func() {
		defer wg.Done()
		query := url.Values{}
		query.Add("per_page", strconv.Itoa(ccc.config.AppsBatchSize))
		apps, err = listWithRetry(ccc, "apps", func() ([]cfclient.V3App, error) { return ccc.config.CCAPIClient.ListV3AppsByQuery(query) })
		if err != nil {
			log.Errorf("Failed listing apps from cloud controller, keeping previously cached data: %v", err)
			return
		}
		*appsMap = make(map[string]*cfclient.V3App, len(apps))
		*sidecarsMap = make(map[string][]*CFSidecar)
		for _, app := range apps {
			v3App := app
			(*appsMap)[app.GUID] = &v3App

			if ccc.config.SidecarsTags {
				// list app sidecars
				var allSidecars []*CFSidecar
				sidecars, err := ccc.config.CCAPIClient.ListSidecarsByApp(query, app.GUID)
				if err != nil {
					log.Errorf("Failed listing sidecars from cloud controller: %v", err)
					continue
				}
				// skip apps without sidecars
				if len(sidecars) == 0 {
					continue
				}
				for _, sidecar := range sidecars {
					s := sidecar
					allSidecars = append(allSidecars, &s)
				}
				(*sidecarsMap)[app.GUID] = allSidecars
			}
		}
	}()
}

func (ccc *CCCache) listSpaces(wg *sync.WaitGroup, spacesMap *map[string]*cfclient.V3Space) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := url.Values{}
		query.Add("per_page", strconv.Itoa(ccc.config.AppsBatchSize))
		spaces, err := listWithRetry(ccc, "spaces", func() ([]cfclient.V3Space, error) { return ccc.config.CCAPIClient.ListV3SpacesByQuery(query) })
		if err != nil {
			log.Errorf("Failed listing spaces from cloud controller, keeping previously cached data: %v", err)
			return
		}
		*spacesMap = make(map[string]*cfclient.V3Space, len(spaces))
		for _, space := range spaces {
			v3Space := space
			(*spacesMap)[space.GUID] = &v3Space
		}
	}()
}

func (ccc *CCCache) listOrgs(wg *sync.WaitGroup, orgsMap *map[string]*cfclient.V3Organization) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := url.Values{}
		query.Add("per_page", strconv.Itoa(ccc.config.AppsBatchSize))
		orgs, err := listWithRetry(ccc, "orgs", func() ([]cfclient.V3Organization, error) {
			return ccc.config.CCAPIClient.ListV3OrganizationsByQuery(query)
		})
		if err != nil {
			log.Errorf("Failed listing orgs from cloud controller, keeping previously cached data: %v", err)
			return
		}
		*orgsMap = make(map[string]*cfclient.V3Organization, len(orgs))
		for _, org := range orgs {
			v3Org := org
			(*orgsMap)[org.GUID] = &v3Org
		}
	}()
}

func (ccc *CCCache) listOrgQuotas(wg *sync.WaitGroup, orgQuotasMap *map[string]*CFOrgQuota) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := url.Values{}
		query.Add("per_page", strconv.Itoa(ccc.config.AppsBatchSize))
		orgQuotas, err := listWithRetry(ccc, "org quotas", func() ([]cfclient.OrgQuota, error) { return ccc.config.CCAPIClient.ListOrgQuotasByQuery(query) })
		if err != nil {
			log.Errorf("Failed listing org quotas from cloud controller, keeping previously cached data: %v", err)
			return
		}
		*orgQuotasMap = make(map[string]*CFOrgQuota, len(orgQuotas))
		for _, orgQuota := range orgQuotas {
			q := CFOrgQuota{
				GUID:        orgQuota.Guid,
				MemoryLimit: orgQuota.MemoryLimit,
			}
			(*orgQuotasMap)[orgQuota.Guid] = &q
		}
	}()
}

func (ccc *CCCache) listProcesses(wg *sync.WaitGroup, processesMap *map[string][]*cfclient.Process) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := url.Values{}
		query.Add("per_page", strconv.Itoa(ccc.config.AppsBatchSize))
		processes, err := listWithRetry(ccc, "processes", func() ([]cfclient.Process, error) { return ccc.config.CCAPIClient.ListAllProcessesByQuery(query) })
		if err != nil {
			log.Errorf("Failed listing processes from cloud controller, keeping previously cached data: %v", err)
			return
		}
		// group all processes per app
		*processesMap = make(map[string][]*cfclient.Process)
		for _, process := range processes {
			v3Process := process
			parts := strings.Split(v3Process.Links.App.Href, "/")
			appGUID := parts[len(parts)-1]
			appProcesses, exists := (*processesMap)[appGUID]
			if exists {
				appProcesses = append(appProcesses, &v3Process)
			} else {
				appProcesses = []*cfclient.Process{&v3Process}
			}
			(*processesMap)[appGUID] = appProcesses
		}
	}()
}

func (ccc *CCCache) listIsolationSegments(wg *sync.WaitGroup, segmentBySpaceGUID *map[string]*cfclient.IsolationSegment, segmentByOrgGUID *map[string]*cfclient.IsolationSegment) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := url.Values{}
		query.Add("per_page", strconv.Itoa(ccc.config.AppsBatchSize))
		segments, err := listWithRetry(ccc, "isolation segments", func() ([]cfclient.IsolationSegment, error) {
			return ccc.config.CCAPIClient.ListIsolationSegmentsByQuery(query)
		})
		if err != nil {
			log.Errorf("Failed listing isolation segments from cloud controller, keeping previously cached data: %v", err)
			return
		}
		*segmentBySpaceGUID = make(map[string]*cfclient.IsolationSegment)
		*segmentByOrgGUID = make(map[string]*cfclient.IsolationSegment)
		for _, segment := range segments {
			s := segment
			spaceGUID, err := ccc.config.CCAPIClient.GetIsolationSegmentSpaceGUID(segment.GUID)
			if err == nil {
				if spaceGUID != "" {
					(*segmentBySpaceGUID)[spaceGUID] = &s
				}
			} else {
				log.Errorf("Failed listing isolation segment space for segment %s: %v", segment.Name, err)
			}

			orgGUID, err := ccc.config.CCAPIClient.GetIsolationSegmentOrganizationGUID(segment.GUID)
			if err == nil {
				if orgGUID != "" {
					(*segmentByOrgGUID)[orgGUID] = &s
				}
			} else {
				log.Errorf("Failed listing isolation segment organization for segment %s: %v", segment.Name, err)
			}

		}
	}()
}

func (ccc *CCCache) prepareCFApplications(appsMap map[string]*cfclient.V3App, processesMap map[string][]*cfclient.Process, spacesMap map[string]*cfclient.V3Space, orgsMap map[string]*cfclient.V3Organization, sidecarsMap map[string][]*CFSidecar) map[string]*CFApplication {
	cfApplicationsByGUID := make(map[string]*CFApplication, len(appsMap))

	for _, cfapp := range appsMap {
		// fill app metadata
		updatedApp := CFApplication{}
		updatedApp.extractDataFromV3App(*cfapp)
		appGUID := updatedApp.GUID
		spaceGUID := updatedApp.SpaceGUID

		// fill processes
		if processes, exists := processesMap[appGUID]; exists {
			updatedApp.extractDataFromV3Process(processes)
		} else {
			log.Infof("could not fetch processes info for app guid %s", appGUID)
		}

		// fill space then org data. Order matters for labels and annotations.
		if space, exists := spacesMap[spaceGUID]; exists {
			updatedApp.extractDataFromV3Space(space)
		} else {
			log.Infof("could not fetch space info for space guid %s", spaceGUID)
		}

		orgGUID := updatedApp.OrgGUID
		if org, exists := orgsMap[orgGUID]; exists {
			updatedApp.extractDataFromV3Org(org)
		} else {
			log.Infof("could not fetch org info for org guid %s", orgGUID)
		}

		// fill sidecars
		for _, sidecar := range sidecarsMap[appGUID] {
			updatedApp.Sidecars = append(updatedApp.Sidecars, *sidecar)
		}

		cfApplicationsByGUID[appGUID] = &updatedApp
	}
	return cfApplicationsByGUID
}

func (ccc *CCCache) start() {
	ccc.readData()
	dataRefreshTicker := time.NewTicker(ccc.config.PollInterval)
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
	var wg sync.WaitGroup

	// list applications
	var appsByGUID map[string]*cfclient.V3App
	var sidecarsByAppGUID map[string][]*CFSidecar
	ccc.listApplications(&wg, &appsByGUID, &sidecarsByAppGUID)

	// list spaces
	var spacesByGUID map[string]*cfclient.V3Space
	ccc.listSpaces(&wg, &spacesByGUID)

	// list orgs
	var orgsByGUID map[string]*cfclient.V3Organization
	ccc.listOrgs(&wg, &orgsByGUID)

	// list orgQuotas
	var orgQuotasByGUID map[string]*CFOrgQuota
	ccc.listOrgQuotas(&wg, &orgQuotasByGUID)

	// list processes
	var processesByAppGUID map[string][]*cfclient.Process
	ccc.listProcesses(&wg, &processesByAppGUID)

	// list isolation segments
	var segmentBySpaceGUID map[string]*cfclient.IsolationSegment
	var segmentByOrgGUID map[string]*cfclient.IsolationSegment
	if ccc.config.SegmentsTags {
		ccc.listIsolationSegments(&wg, &segmentBySpaceGUID, &segmentByOrgGUID)
	}

	// wait for resources acquisition
	wg.Wait()

	// update cache in-place instead of swapping entire maps
	ccc.Lock()
	defer ccc.Unlock()

	ccc.appsByGUID = updateMapInPlace(ccc.appsByGUID, appsByGUID)
	ccc.spacesByGUID = updateMapInPlace(ccc.spacesByGUID, spacesByGUID)
	ccc.orgsByGUID = updateMapInPlace(ccc.orgsByGUID, orgsByGUID)
	ccc.orgQuotasByGUID = updateMapInPlace(ccc.orgQuotasByGUID, orgQuotasByGUID)
	ccc.processesByAppGUID = updateMapInPlace(ccc.processesByAppGUID, processesByAppGUID)
	ccc.sidecarsByAppGUID = updateMapInPlace(ccc.sidecarsByAppGUID, sidecarsByAppGUID)
	ccc.segmentBySpaceGUID = updateMapInPlace(ccc.segmentBySpaceGUID, segmentBySpaceGUID)
	ccc.segmentByOrgGUID = updateMapInPlace(ccc.segmentByOrgGUID, segmentByOrgGUID)

	// prepare CFApplications for the nozzle from the cached maps rather than this poll's results:
	// the list functions leave their map nil when a fetch fails and updateMapInPlace then keeps the
	// cached one, so a failed input falls back to its last known data while the others are refreshed.
	// Wait until every input has been fetched once, otherwise the missing fields would be served as zero.
	cachedInputsReady := ccc.appsByGUID != nil && ccc.processesByAppGUID != nil && ccc.spacesByGUID != nil && ccc.orgsByGUID != nil
	if ccc.config.ServeNozzleData && cachedInputsReady {
		cfApplicationsByGUID := ccc.prepareCFApplications(ccc.appsByGUID, ccc.processesByAppGUID, ccc.spacesByGUID, ccc.orgsByGUID, ccc.sidecarsByAppGUID)
		ccc.cfApplicationsByGUID = updateMapInPlace(ccc.cfApplicationsByGUID, cfApplicationsByGUID)
	}

	firstUpdate := ccc.lastUpdated.IsZero()
	ccc.lastUpdated = time.Now()
	if firstUpdate {
		close(ccc.updatedOnce)
	}
}

// listWithRetry retries a Cloud Controller list call with exponential backoff, since a single
// failed request otherwise costs a whole refresh.
func listWithRetry[T any](ccc *CCCache, resourceName string, fetch func() (T, error)) (T, error) {
	backoff := ccc.retryBackoff
	for attempt := 1; ; attempt++ {
		result, err := fetch()
		if err == nil || attempt == ccListAttempts || !isRetryableCCError(err) {
			return result, err
		}
		log.Warnf("Failed listing %s from cloud controller (attempt %d/%d), retrying in %s: %v", resourceName, attempt, ccListAttempts, backoff, err)
		select {
		case <-time.After(backoff):
		case <-ccc.cancelContext.Done():
			return result, err
		}
		backoff *= 2
	}
}

// isRetryableCCError reports whether err is a server-side or transport failure that may succeed on retry.
// go-cfclient drops the HTTP status when the Cloud Controller returns a JSON error body, so those are
// classified by Cloud Controller error code instead.
func isRetryableCCError(err error) bool {
	var cfErr cfclient.CloudFoundryError
	if errors.As(err, &cfErr) {
		_, ok := retryableCCErrorCodes[cfErr.Code]
		return ok
	}
	var httpErr cfclient.CloudFoundryHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500 || httpErr.StatusCode == 429
	}
	return true
}

// updateMapInPlace updates the cache map in-place with new data.
// It adds new entries, updates existing ones, and removes entries that are no longer present.
// If newData is nil, the cache is returned unchanged (to handle fetch failures gracefully).
// If cache is nil, it initializes and returns a new map with the newData content.
func updateMapInPlace[K comparable, V any](cache map[K]V, newData map[K]V) map[K]V {
	if newData == nil {
		return cache
	}

	if cache == nil {
		cache = make(map[K]V, len(newData))
	}

	// Remove entries that no longer exist
	for key := range cache {
		if _, exists := newData[key]; !exists {
			delete(cache, key)
		}
	}

	// Add or update entries
	maps.Copy(cache, newData)

	return cache
}
