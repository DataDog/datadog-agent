// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && !windows

package cloudfoundry

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/cloudfoundry-community/go-cfclient/v2"
	"github.com/stretchr/testify/assert"
)

// testCCClient implements CCClientI for testing
type testCCClient struct{}

func (t testCCClient) ListV3AppsByQuery(_ url.Values) ([]cfclient.V3App, error) {
	return []cfclient.V3App{v3App1, v3App2}, nil
}

func (t testCCClient) ListV3OrganizationsByQuery(_ url.Values) ([]cfclient.V3Organization, error) {
	return []cfclient.V3Organization{v3Org1, v3Org2}, nil
}

func (t testCCClient) ListV3SpacesByQuery(_ url.Values) ([]cfclient.V3Space, error) {
	return []cfclient.V3Space{v3Space1, v3Space2}, nil
}

func (t testCCClient) ListAllProcessesByQuery(_ url.Values) ([]cfclient.Process, error) {
	return []cfclient.Process{cfProcess1, cfProcess2}, nil
}

func (t testCCClient) ListOrgQuotasByQuery(_ url.Values) ([]cfclient.OrgQuota, error) {
	return []cfclient.OrgQuota{cfOrgQuota1, cfOrgQuota2}, nil
}

func (t testCCClient) ListSidecarsByApp(_ url.Values, guid string) ([]CFSidecar, error) {
	if guid == "random_app_guid" {
		return []CFSidecar{cfSidecar1}, nil
	} else if guid == "guid2" {
		return []CFSidecar{cfSidecar2}, nil
	}
	return nil, nil
}

func (t testCCClient) ListIsolationSegmentsByQuery(_ url.Values) ([]cfclient.IsolationSegment, error) {
	return []cfclient.IsolationSegment{cfIsolationSegment1, cfIsolationSegment2}, nil
}

func (t testCCClient) GetIsolationSegmentSpaceGUID(guid string) (string, error) {
	if guid == "isolation_segment_guid_1" {
		return "space_guid_1", nil
	} else if guid == "isolation_segment_guid_2" {
		return "space_guid_2", nil
	}
	return "", nil
}

func (t testCCClient) GetIsolationSegmentOrganizationGUID(guid string) (string, error) {
	if guid == "isolation_segment_guid_1" {
		return "org_guid_1", nil
	} else if guid == "isolation_segment_guid_2" {
		return "org_guid_2", nil
	}
	return "", nil
}

func (t testCCClient) GetV3AppByGUID(guid string) (*cfclient.V3App, error) {
	switch guid {
	case v3App1.GUID:
		return &v3App1, nil
	case v3App2.GUID:
		return &v3App2, nil
	}
	return nil, fmt.Errorf("could not find V3App with guid %s", guid)
}

func (t testCCClient) GetV3SpaceByGUID(guid string) (*cfclient.V3Space, error) {
	switch guid {
	case v3Space1.GUID:
		return &v3Space1, nil
	case v3Space2.GUID:
		return &v3Space2, nil
	}
	return nil, fmt.Errorf("could not find V3Space with guid %s", guid)
}

func (t testCCClient) GetV3OrganizationByGUID(guid string) (*cfclient.V3Organization, error) {
	switch guid {
	case v3Org1.GUID:
		return &v3Org1, nil
	case v3Org2.GUID:
		return &v3Org2, nil
	}
	return nil, fmt.Errorf("could not find V3Organization with guid %s", guid)
}

//nolint:revive // TODO(PLINT) Fix revive linter
func (t testCCClient) ListProcessByAppGUID(query url.Values, guid string) ([]cfclient.Process, error) {
	if guid == v3App1.GUID {
		return []cfclient.Process{cfProcess1, cfProcess2}, nil
	}
	return nil, fmt.Errorf("could not find processes for app with guid %s", guid)
}

// newTestCCCacheConfig creates a default CCCacheConfig for testing
func newTestCCCacheConfig() CCCacheConfig {
	return CCCacheConfig{
		CCAPIClient:     testCCClient{},
		PollInterval:    time.Hour, // Long interval to prevent automatic polling during tests
		AppsBatchSize:   1,
		ServeNozzleData: true,
		SidecarsTags:    true,
		SegmentsTags:    true,
	}
}

// newTestCCCache creates a new CCCache instance for testing
func newTestCCCache(ctx context.Context, config CCCacheConfig) *CCCache {
	cache := &CCCache{
		cancelContext: ctx,
		configured:    true,
		config:        config,
		updatedOnce:   make(chan struct{}),
	}

	// Populate the cache using readData which calls the mock client
	cache.readData()

	return cache
}

// setupCCCache is a test helper that creates a CCCache with automatic cleanup
func setupCCCache(t *testing.T, refreshOnMiss bool) *CCCache {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	config := newTestCCCacheConfig()
	config.RefreshCacheOnMiss = refreshOnMiss

	return newTestCCCache(ctx, config)
}

func TestCCCachePolling(t *testing.T) {
	cc := setupCCCache(t, false)
	assert.NotZero(t, cc.LastUpdated())
}

func TestCCCache_GetApp(t *testing.T) {
	cc := setupCCCache(t, false)

	app1, err := cc.GetApp("random_app_guid")
	assert.Nil(t, err)
	assert.EqualValues(t, v3App1, *app1)
	app2, err := cc.GetApp("guid2")
	assert.Nil(t, err)
	assert.EqualValues(t, v3App2, *app2)
	_, err = cc.GetApp("not-existing-guid")
	assert.NotNil(t, err)
}

func TestCCCache_GetSpace(t *testing.T) {
	cc := setupCCCache(t, false)

	space1, _ := cc.GetSpace("space_guid_1")
	assert.EqualValues(t, v3Space1, *space1)
	space2, _ := cc.GetSpace("space_guid_2")
	assert.EqualValues(t, v3Space2, *space2)
	_, err := cc.GetSpace("not-existing-guid")
	assert.NotNil(t, err)
}

func TestCCCache_GetOrg(t *testing.T) {
	cc := setupCCCache(t, false)

	org1, _ := cc.GetOrg("org_guid_1")
	assert.EqualValues(t, v3Org1, *org1)
	org2, _ := cc.GetOrg("org_guid_2")
	assert.EqualValues(t, v3Org2, *org2)
	_, err := cc.GetOrg("not-existing-guid")
	assert.NotNil(t, err)
}

func TestCCCache_GetCFApplication(t *testing.T) {
	cc := setupCCCache(t, false)

	cfapp1, _ := cc.GetCFApplication("random_app_guid")
	assert.EqualValues(t, &cfApp1, cfapp1)
	cfapp2, _ := cc.GetCFApplication("guid2")
	assert.EqualValues(t, &cfApp2, cfapp2)
	_, err := cc.GetCFApplication("not-existing-guid")
	assert.NotNil(t, err)
}

func TestCCCache_GetCFApplications(t *testing.T) {
	cc := setupCCCache(t, false)

	cfapps, _ := cc.GetCFApplications()
	sort.Slice(cfapps, func(i, j int) bool {
		return cfapps[i].GUID > cfapps[j].GUID
	})
	assert.EqualValues(t, 2, len(cfapps))
	assert.EqualValues(t, &cfApp1, cfapps[0])
	assert.EqualValues(t, &cfApp2, cfapps[1])
}

func TestCCCache_GetSidecars(t *testing.T) {
	cc := setupCCCache(t, false)

	sidecar1, _ := cc.GetSidecars("random_app_guid")
	assert.EqualValues(t, 1, len(sidecar1))
	assert.EqualValues(t, &cfSidecar1, sidecar1[0])
	sidecar2, _ := cc.GetSidecars("guid2")
	assert.EqualValues(t, 1, len(sidecar2))
	assert.EqualValues(t, &cfSidecar2, sidecar2[0])
	_, err := cc.GetSidecars("not-existing-guid")
	assert.NotNil(t, err)
}

func TestCCCache_GetIsolationSegmentForSpace(t *testing.T) {
	cc := setupCCCache(t, false)

	segment1, _ := cc.GetIsolationSegmentForSpace("space_guid_1")
	assert.EqualValues(t, &cfIsolationSegment1, segment1)
	segment2, _ := cc.GetIsolationSegmentForSpace("space_guid_2")
	assert.EqualValues(t, &cfIsolationSegment2, segment2)
	_, err := cc.GetIsolationSegmentForSpace("invalid_space_guid")
	assert.NotNil(t, err)
}

func TestCCCache_GetIsolationSegmentForOrg(t *testing.T) {
	cc := setupCCCache(t, false)

	segment1, _ := cc.GetIsolationSegmentForOrg("org_guid_1")
	assert.EqualValues(t, &cfIsolationSegment1, segment1)
	segment2, _ := cc.GetIsolationSegmentForOrg("org_guid_2")
	assert.EqualValues(t, &cfIsolationSegment2, segment2)
	_, err := cc.GetIsolationSegmentForOrg("invalid_org_guid")
	assert.NotNil(t, err)
}

func TestCCCache_GetProcesses(t *testing.T) {
	cc := setupCCCache(t, false)

	processes, err := cc.GetProcesses("random_app_guid")
	assert.Nil(t, err)

	expected := []cfclient.Process{cfProcess1, cfProcess2}
	// maps in go do not guarantee order
	sort.Slice(expected, func(i, j int) bool {
		return expected[i].GUID > expected[j].GUID
	})

	assert.EqualValues(t, &cfProcess1, processes[0])
	assert.EqualValues(t, &cfProcess2, processes[1])

	_, err = cc.GetProcesses("missing_app_guid")
	assert.NotNil(t, err)
}

func TestCCCache_RefreshCacheOnMiss_GetProcesses(t *testing.T) {
	cc := setupCCCache(t, true)

	// clear the processes cache to trigger cache misses
	cc.Lock()
	cc.processesByAppGUID = make(map[string][]*cfclient.Process)
	cc.Unlock()

	// concurrent requests should all succeed
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			processes, err := cc.GetProcesses(v3App1.GUID)
			assert.Nil(t, err)
			assert.NotEmpty(t, processes)
		}()
	}
	wg.Wait()
}

func TestCCCache_RefreshCacheOnMiss_GetApp(t *testing.T) {
	cc := setupCCCache(t, true)

	// clear the apps cache to trigger cache misses
	cc.Lock()
	cc.appsByGUID = make(map[string]*cfclient.V3App)
	cc.Unlock()

	// concurrent requests should all succeed
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			app, err := cc.GetApp(v3App1.GUID)
			assert.Nil(t, err)
			assert.NotNil(t, app)
		}()
	}
	wg.Wait()
}

func TestCCCache_RefreshCacheOnMiss_GetSpace(t *testing.T) {
	cc := setupCCCache(t, true)

	// clear the spaces cache to trigger cache misses
	cc.Lock()
	cc.spacesByGUID = make(map[string]*cfclient.V3Space)
	cc.Unlock()

	// concurrent requests should all succeed
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			space, err := cc.GetSpace(v3Space1.GUID)
			assert.Nil(t, err)
			assert.NotNil(t, space)
		}()
	}
	wg.Wait()
}

func TestCCCache_RefreshCacheOnMiss_GetOrg(t *testing.T) {
	cc := setupCCCache(t, true)

	// clear the orgs cache to trigger cache misses
	cc.Lock()
	cc.orgsByGUID = make(map[string]*cfclient.V3Organization)
	cc.Unlock()

	// concurrent requests should all succeed
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			org, err := cc.GetOrg(v3Org1.GUID)
			assert.Nil(t, err)
			assert.NotNil(t, org)
		}()
	}
	wg.Wait()
}

func TestCCCache_RefreshCacheOnMiss_GetCFApplication(t *testing.T) {
	cc := setupCCCache(t, true)

	// clear multiple caches to trigger cache misses
	cc.Lock()
	cc.cfApplicationsByGUID = make(map[string]*CFApplication)
	cc.appsByGUID = make(map[string]*cfclient.V3App)
	cc.spacesByGUID = make(map[string]*cfclient.V3Space)
	cc.orgsByGUID = make(map[string]*cfclient.V3Organization)
	cc.processesByAppGUID = make(map[string][]*cfclient.Process)
	cc.Unlock()

	// concurrent requests should all succeed
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfapp, err := cc.GetCFApplication(cfApp1.GUID)
			assert.Nil(t, err)
			assert.NotNil(t, cfapp)
		}()
	}
	wg.Wait()
}
