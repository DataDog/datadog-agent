// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && !kubeapiserver && !windows

package cloudfoundry

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cloudfoundry/go-cfclient/v3/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fakeCCClientID     = "client-id"
	fakeCCClientSecret = "client-secret"
	fakeCCToken        = "fake-token"
	fakeCCPerPage      = 2
)

// fakeCloudController serves the subset of the Cloud Controller v3 API used by CFClient
type fakeCloudController struct {
	*httptest.Server
	mu       sync.Mutex
	requests []*http.Request
}

func newFakeCloudController(t *testing.T) *fakeCloudController {
	t.Helper()
	f := &fakeCloudController{}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeCloudController) recorded() []*http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*http.Request(nil), f.requests...)
}

// paginated renders a v3 list response, linking to the next page when there is one
func (f *fakeCloudController) paginated(r *http.Request, pages ...string) string {
	page := 1
	fmt.Sscan(r.URL.Query().Get("page"), &page)
	next := "null"
	if page < len(pages) {
		next = fmt.Sprintf(`{"href": "%s%s?page=%d&per_page=%s"}`, f.URL, r.URL.Path, page+1, r.URL.Query().Get("per_page"))
	}
	return fmt.Sprintf(`{"pagination": {"total_pages": %d, "next": %s}, "resources": [%s]}`, len(pages), next, pages[page-1])
}

func (f *fakeCloudController) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r)
	f.mu.Unlock()

	if r.URL.Path == "/oauth/token" {
		user, password, ok := r.BasicAuth()
		if !ok || user != fakeCCClientID || password != fakeCCClientSecret {
			http.Error(w, "bad credentials", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token": %q, "token_type": "bearer", "expires_in": 3600}`, fakeCCToken)
		return
	}
	if r.URL.Path != "/" && r.Header.Get("Authorization") != "Bearer "+fakeCCToken {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	var body string
	switch r.URL.Path {
	case "/":
		body = fmt.Sprintf(`{"links": {"self": {"href": %[1]q}, "login": {"href": %[1]q}, "uaa": {"href": %[1]q}}}`, f.URL)
	case "/v3/apps":
		body = f.paginated(r,
			`{"guid": "app_guid_1", "name": "app_1", "lifecycle": {"type": "buildpack", "data": {"buildpacks": ["ruby_buildpack"]}}, "relationships": {"space": {"data": {"guid": "space_guid_1"}}}, "metadata": {"labels": {"tags.datadoghq.com/env": "prod"}, "annotations": {}}}`,
			`{"guid": "app_guid_2", "name": "app_2", "lifecycle": {"type": "docker", "data": {}}, "relationships": {"space": {"data": {"guid": "space_guid_1"}}}}`)
	case "/v3/apps/app_guid_1":
		body = `{"guid": "app_guid_1", "name": "app_1", "relationships": {"space": {"data": {"guid": "space_guid_1"}}}}`
	case "/v3/apps/app_guid_1/processes":
		body = f.paginated(r, `{"guid": "process_guid_1", "type": "web", "instances": 2, "memory_in_mb": 256, "disk_in_mb": 1024}`)
	case "/v3/apps/app_guid_1/sidecars":
		body = f.paginated(r, `{"guid": "sidecar_guid_1", "name": "sidecar_1"}`)
	case "/v3/apps/unavailable":
		w.Header().Set("Content-Type", "text/html")
		http.Error(w, "<html>502 Bad Gateway</html>", http.StatusBadGateway)
		return
	case "/v3/apps/forbidden":
		w.WriteHeader(http.StatusForbidden)
		body = `{"errors": [{"code": 10003, "title": "CF-NotAuthorized", "detail": "You are not authorized to perform the requested action"}]}`
	case "/v3/processes":
		body = f.paginated(r,
			`{"guid": "process_guid_1", "type": "web", "links": {"app": {"href": "https://api.example.com/v3/apps/app_guid_1"}}}`,
			`{"guid": "process_guid_2", "type": "worker", "links": {"app": {"href": "https://api.example.com/v3/apps/app_guid_1"}}}`)
	case "/v3/spaces":
		body = f.paginated(r, `{"guid": "space_guid_1", "name": "space_1", "relationships": {"organization": {"data": {"guid": "org_guid_1"}}}}`)
	case "/v3/spaces/space_guid_1":
		body = `{"guid": "space_guid_1", "name": "space_1", "relationships": {"organization": {"data": {"guid": "org_guid_1"}}}}`
	case "/v3/organizations":
		body = f.paginated(r, `{"guid": "org_guid_1", "name": "org_1", "suspended": false, "relationships": {"quota": {"data": {"guid": "quota_guid_1"}}}}`)
	case "/v3/organizations/org_guid_1":
		body = `{"guid": "org_guid_1", "name": "org_1", "relationships": {"quota": {"data": {"guid": "quota_guid_1"}}}}`
	case "/v3/organization_quotas":
		body = f.paginated(r,
			`{"guid": "quota_guid_1", "name": "default", "apps": {"total_memory_in_mb": 10240}}`,
			`{"guid": "quota_guid_2", "name": "unlimited", "apps": {"total_memory_in_mb": null}}`)
	case "/v3/isolation_segments":
		body = f.paginated(r, `{"guid": "segment_guid_1", "name": "segment_1"}`)
	case "/v3/isolation_segments/segment_guid_1/relationships/spaces":
		body = `{"data": [{"guid": "space_guid_1"}, {"guid": "space_guid_2"}]}`
	case "/v3/isolation_segments/segment_guid_1/relationships/organizations":
		body = `{"data": []}`
	default:
		w.WriteHeader(http.StatusNotFound)
		body = `{"errors": [{"code": 10010, "title": "CF-ResourceNotFound", "detail": "Not found"}]}`
	}
	fmt.Fprint(w, body)
}

func newTestCFClient(t *testing.T) (*CFClient, *fakeCloudController) {
	t.Helper()
	cc := newFakeCloudController(t)
	client, err := NewCFClient(cc.URL, fakeCCClientID, fakeCCClientSecret, false)
	require.NoError(t, err)
	return client, cc
}

func TestNewCFClient_DiscoversUAAFromAPIRoot(t *testing.T) {
	client, cc := newTestCFClient(t)
	_, err := client.ListOrganizations(fakeCCPerPage)
	require.NoError(t, err)

	var paths []string
	for _, r := range cc.recorded() {
		paths = append(paths, r.URL.Path)
	}
	assert.Equal(t, []string{"/", "/oauth/token", "/v3/organizations"}, paths)
}

func TestNewCFClient_FailsWhenAPIRootIsUnreachable(t *testing.T) {
	cc := newFakeCloudController(t)
	cc.Close()
	_, err := NewCFClient(cc.URL, fakeCCClientID, fakeCCClientSecret, false)
	assert.Error(t, err)
}

func TestCFClient_ListFollowsPaginationAndSendsPageSize(t *testing.T) {
	client, cc := newTestCFClient(t)

	apps, err := client.ListApps(fakeCCPerPage)
	require.NoError(t, err)
	require.Len(t, apps, 2)
	assert.Equal(t, "app_guid_1", apps[0].GUID)
	assert.Equal(t, "app_guid_2", apps[1].GUID)

	var pages []string
	for _, r := range cc.recorded() {
		if r.URL.Path == "/v3/apps" {
			assert.Equal(t, fmt.Sprint(fakeCCPerPage), r.URL.Query().Get("per_page"))
			pages = append(pages, r.URL.Query().Get("page"))
		}
	}
	assert.Equal(t, []string{"1", "2"}, pages)
}

func TestCFClient_ListMethods(t *testing.T) {
	client, _ := newTestCFClient(t)

	orgs, err := client.ListOrganizations(fakeCCPerPage)
	require.NoError(t, err)
	require.Len(t, orgs, 1)
	assert.Equal(t, "quota_guid_1", orgs[0].Relationships.Quota.Data.GUID)

	spaces, err := client.ListSpaces(fakeCCPerPage)
	require.NoError(t, err)
	require.Len(t, spaces, 1)
	assert.Equal(t, "org_guid_1", spaceOrgGUID(spaces[0]))

	processes, err := client.ListProcesses(fakeCCPerPage)
	require.NoError(t, err)
	require.Len(t, processes, 2)
	assert.Equal(t, "https://api.example.com/v3/apps/app_guid_1", processes[1].Links["app"].Href)

	quotas, err := client.ListOrgQuotas(fakeCCPerPage)
	require.NoError(t, err)
	require.Len(t, quotas, 2)
	assert.Equal(t, 10240, *quotas[0].Apps.TotalMemoryInMB)
	assert.Nil(t, quotas[1].Apps.TotalMemoryInMB)

	segments, err := client.ListIsolationSegments(fakeCCPerPage)
	require.NoError(t, err)
	require.Len(t, segments, 1)
	assert.Equal(t, "segment_1", segments[0].Name)

	sidecars, err := client.ListSidecarsByApp("app_guid_1", fakeCCPerPage)
	require.NoError(t, err)
	assert.Equal(t, []CFSidecar{{Name: "sidecar_1", GUID: "sidecar_guid_1"}}, sidecars)

	appProcesses, err := client.ListProcessesByApp("app_guid_1", fakeCCPerPage)
	require.NoError(t, err)
	require.Len(t, appProcesses, 1)
	assert.Equal(t, 2, appProcesses[0].Instances)
}

func TestCFClient_GetMethods(t *testing.T) {
	client, _ := newTestCFClient(t)

	app, err := client.GetApp("app_guid_1")
	require.NoError(t, err)
	assert.Equal(t, "space_guid_1", appSpaceGUID(app))

	space, err := client.GetSpace("space_guid_1")
	require.NoError(t, err)
	assert.Equal(t, "space_1", space.Name)

	org, err := client.GetOrganization("org_guid_1")
	require.NoError(t, err)
	assert.Equal(t, "org_1", org.Name)

	_, err = client.GetApp("missing_app_guid")
	assert.Error(t, err)
}

func TestCFClient_IsolationSegmentRelationships(t *testing.T) {
	client, _ := newTestCFClient(t)

	// only the first entitled space is used, as before the v3 client migration
	spaceGUID, err := client.GetIsolationSegmentSpaceGUID("segment_guid_1")
	require.NoError(t, err)
	assert.Equal(t, "space_guid_1", spaceGUID)

	orgGUID, err := client.GetIsolationSegmentOrganizationGUID("segment_guid_1")
	require.NoError(t, err)
	assert.Empty(t, orgGUID)
}

func TestCFClient_ExtractsAppData(t *testing.T) {
	client, _ := newTestCFClient(t)

	apps, err := client.ListApps(fakeCCPerPage)
	require.NoError(t, err)
	require.Len(t, apps, 2)

	buildpackApp := CFApplication{}
	buildpackApp.extractDataFromV3App(apps[0])
	assert.Equal(t, []string{"ruby_buildpack"}, buildpackApp.Buildpacks)
	assert.Equal(t, map[string]string{"tags.datadoghq.com/env": "prod"}, buildpackApp.Labels)

	dockerApp := CFApplication{}
	dockerApp.extractDataFromV3App(apps[1])
	assert.Nil(t, dockerApp.Buildpacks)
	assert.Empty(t, dockerApp.Labels)
	assert.Empty(t, dockerApp.Annotations)
}

func TestCFClient_ErrorsAreClassifiedForRetries(t *testing.T) {
	client, _ := newTestCFClient(t)

	_, err := client.GetApp("unavailable")
	require.Error(t, err)
	var httpErr resource.CloudFoundryHTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusBadGateway, httpErr.StatusCode)
	assert.True(t, isRetryableCCError(err))

	_, err = client.GetApp("forbidden")
	require.Error(t, err)
	var cfErr resource.CloudFoundryError
	require.ErrorAs(t, err, &cfErr)
	assert.Equal(t, 10003, cfErr.Code)
	assert.False(t, isRetryableCCError(err))
}

func TestCFClient_NeverCallsV2API(t *testing.T) {
	client, cc := newTestCFClient(t)

	_, err := client.ListApps(fakeCCPerPage)
	require.NoError(t, err)
	_, err = client.ListOrgQuotas(fakeCCPerPage)
	require.NoError(t, err)
	_, err = client.GetIsolationSegmentSpaceGUID("segment_guid_1")
	require.NoError(t, err)

	for _, r := range cc.recorded() {
		assert.False(t, strings.HasPrefix(r.URL.Path, "/v2/"), "unexpected v2 request: %s", r.URL.Path)
	}
}

func TestCFClient_SetsUserAgent(t *testing.T) {
	client, cc := newTestCFClient(t)
	_, err := client.ListApps(fakeCCPerPage)
	require.NoError(t, err)

	for _, r := range cc.recorded() {
		if strings.HasPrefix(r.URL.Path, "/v3/") {
			assert.Equal(t, "datadog-cluster-agent", r.Header.Get("User-Agent"))
		}
	}
}

func TestNewCFClient_RequestTimeout(t *testing.T) {
	client, _ := newTestCFClient(t)
	assert.Equal(t, ccRequestTimeout, client.client.Config.HTTPAuthClient().Timeout)
	assert.Equal(t, ccRequestTimeout, client.client.Config.HTTPClient().Timeout)
}
