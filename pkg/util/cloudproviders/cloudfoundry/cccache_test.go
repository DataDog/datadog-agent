// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && !kubeapiserver && !windows

package cloudfoundry

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/cloudfoundry-community/go-cfclient/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

var serverError = cfclient.CloudFoundryError{Code: 10001, ErrorCode: "UnknownError", Description: "An unknown error occurred."}

// flakyCCClient fails the configured list calls a given number of times before delegating to testCCClient
type flakyCCClient struct {
	testCCClient
	mu        sync.Mutex
	failures  map[string]int
	calls     map[string]int
	instances int
	err       error
}

func (f *flakyCCClient) fail(resource string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[resource]++
	if f.failures[resource] > 0 {
		f.failures[resource]--
		if f.err != nil {
			return f.err
		}
		return serverError
	}
	return nil
}

func (f *flakyCCClient) ListV3AppsByQuery(q url.Values) ([]cfclient.V3App, error) {
	if err := f.fail("apps"); err != nil {
		return nil, err
	}
	return f.testCCClient.ListV3AppsByQuery(q)
}

func (f *flakyCCClient) ListV3SpacesByQuery(q url.Values) ([]cfclient.V3Space, error) {
	if err := f.fail("spaces"); err != nil {
		return nil, err
	}
	return f.testCCClient.ListV3SpacesByQuery(q)
}

func (f *flakyCCClient) ListV3OrganizationsByQuery(q url.Values) ([]cfclient.V3Organization, error) {
	if err := f.fail("orgs"); err != nil {
		return nil, err
	}
	return f.testCCClient.ListV3OrganizationsByQuery(q)
}

func (f *flakyCCClient) ListAllProcessesByQuery(_ url.Values) ([]cfclient.Process, error) {
	if err := f.fail("processes"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p1 := cfProcess1
	p1.Instances = f.instances
	return []cfclient.Process{p1, cfProcess2}, nil
}

func newFlakyCCCache(t *testing.T, client *flakyCCClient) *CCCache {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	config := newTestCCCacheConfig()
	config.CCAPIClient = client
	return newTestCCCache(ctx, config)
}

func TestCCCache_FailedFetchFallsBackToCachedInput(t *testing.T) {
	for _, tc := range []struct {
		resource      string
		wantInstances int
	}{
		{resource: "apps", wantInstances: 5},
		{resource: "processes", wantInstances: 3},
		{resource: "spaces", wantInstances: 5},
		{resource: "orgs", wantInstances: 5},
	} {
		t.Run(tc.resource, func(t *testing.T) {
			client := &flakyCCClient{failures: map[string]int{}, calls: map[string]int{}, instances: 3}
			cc := newFlakyCCCache(t, client)
			before, err := cc.GetCFApplication(cfApp1.GUID)
			require.NoError(t, err)

			client.instances = 5
			client.failures[tc.resource] = ccListAttempts
			cc.readData()

			after, err := cc.GetCFApplication(cfApp1.GUID)
			require.NoError(t, err)
			assert.Equal(t, tc.wantInstances, after.Instances)
			assert.Equal(t, before.Name, after.Name)
			assert.Equal(t, before.SpaceName, after.SpaceName)
			assert.Equal(t, before.OrgName, after.OrgName)
		})
	}
}

func TestCCCache_NoApplicationsServedUntilEveryInputFetched(t *testing.T) {
	client := &flakyCCClient{failures: map[string]int{"processes": ccListAttempts}, calls: map[string]int{}, instances: 3}
	cc := newFlakyCCCache(t, client)

	cfapps, err := cc.GetCFApplications()
	require.NoError(t, err)
	assert.Empty(t, cfapps)

	cc.readData()

	cfapp, err := cc.GetCFApplication(cfApp1.GUID)
	require.NoError(t, err)
	assert.Equal(t, 3, cfapp.Instances)
}

func TestCCCache_RefreshOnMissBeforeEveryInputFetched(t *testing.T) {
	client := &flakyCCClient{failures: map[string]int{"orgs": ccListAttempts}, calls: map[string]int{}, instances: 3}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	config := newTestCCCacheConfig()
	config.CCAPIClient = client
	config.RefreshCacheOnMiss = true
	cc := newTestCCCache(ctx, config)

	cfapp, err := cc.GetCFApplication(cfApp1.GUID)
	require.NoError(t, err)
	assert.Equal(t, 3, cfapp.Instances)

	cc.readData()

	cfapps, err := cc.GetCFApplications()
	require.NoError(t, err)
	assert.Len(t, cfapps, 2)
}

func TestCCCache_FetchRetries(t *testing.T) {
	for _, tc := range []struct {
		name          string
		err           error
		failures      int
		wantCalls     int
		wantInstances int
	}{
		{name: "transient server error is retried", err: serverError, failures: ccListAttempts - 1, wantCalls: ccListAttempts, wantInstances: 5},
		{name: "permanent error is not retried", err: cfclient.CloudFoundryError{Code: 10003, ErrorCode: "CF-NotAuthorized"}, failures: ccListAttempts, wantCalls: 1, wantInstances: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &flakyCCClient{failures: map[string]int{}, calls: map[string]int{}, instances: 3}
			cc := newFlakyCCCache(t, client)

			client.err = tc.err
			client.failures["processes"] = tc.failures
			client.instances = 5
			client.calls["processes"] = 0
			cc.readData()

			cfapp, err := cc.GetCFApplication(cfApp1.GUID)
			require.NoError(t, err)
			assert.Equal(t, tc.wantInstances, cfapp.Instances)
			assert.Equal(t, tc.wantCalls, client.calls["processes"])
		})
	}
}

func TestIsRetryableCCError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "server error", err: serverError, want: true},
		{name: "rate limited", err: cfclient.CloudFoundryError{Code: 10013}, want: true},
		{name: "not authorized", err: cfclient.CloudFoundryError{Code: 10003}, want: false},
		{name: "not authenticated", err: cfclient.CloudFoundryError{Code: 10002}, want: false},
		{name: "non-JSON 502", err: cfclient.CloudFoundryHTTPError{StatusCode: 502}, want: true},
		{name: "non-JSON 404", err: cfclient.CloudFoundryHTTPError{StatusCode: 404}, want: false},
		{name: "wrapped server error", err: fmt.Errorf("listing processes: %w", serverError), want: true},
		{name: "connection reset", err: errors.New("read tcp 10.0.0.1:43690->10.0.0.2:443: read: connection reset by peer"), want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isRetryableCCError(tc.err))
		})
	}
}
