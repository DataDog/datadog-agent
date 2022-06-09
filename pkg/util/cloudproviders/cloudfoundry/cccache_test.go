// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && !windows
// +build clusterchecks,!windows

package cloudfoundry

import (
	"net/url"
	"testing"

	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/stretchr/testify/assert"
)

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

func TestCCCachePolling(t *testing.T) {
	assert.NotZero(t, cc.LastUpdated())
}

func TestCCCache_GetApp(t *testing.T) {
	app1, _ := cc.GetApp("random_app_guid")
	assert.EqualValues(t, v3App1, *app1)
	app2, _ := cc.GetApp("guid2")
	assert.EqualValues(t, v3App2, *app2)
	_, err := cc.GetApp("not-existing-guid")
	assert.NotNil(t, err)
}

func TestCCCache_GetSpace(t *testing.T) {
	space1, _ := cc.GetSpace("space_guid_1")
	assert.EqualValues(t, v3Space1, *space1)
	space2, _ := cc.GetSpace("space_guid_2")
	assert.EqualValues(t, v3Space2, *space2)
	_, err := cc.GetSpace("not-existing-guid")
	assert.NotNil(t, err)
}

func TestCCCache_GetOrg(t *testing.T) {
	org1, _ := cc.GetOrg("org_guid_1")
	assert.EqualValues(t, v3Org1, *org1)
	org2, _ := cc.GetOrg("org_guid_2")
	assert.EqualValues(t, v3Org2, *org2)
	_, err := cc.GetOrg("not-existing-guid")
	assert.NotNil(t, err)
}

func TestCCCache_GetCFApplication(t *testing.T) {
	cc.readData()
	cfapp1, _ := cc.GetCFApplication("random_app_guid")
	assert.EqualValues(t, &cfApp1, cfapp1)
	cfapp2, _ := cc.GetCFApplication("guid2")
	assert.EqualValues(t, &cfApp2, cfapp2)
	_, err := cc.GetCFApplication("not-existing-guid")
	assert.NotNil(t, err)
}

func TestCCCache_GetCFApplications(t *testing.T) {
	cc.readData()
	cfapps, _ := cc.GetCFApplications()
	assert.EqualValues(t, 2, len(cfapps))
	assert.EqualValues(t, &cfApp1, cfapps[0])
	assert.EqualValues(t, &cfApp2, cfapps[1])
}

func TestCCCache_GetSidecars(t *testing.T) {
	cc.readData()
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
	cc.readData()
	segment1, _ := cc.GetIsolationSegmentForSpace("space_guid_1")
	assert.EqualValues(t, &cfIsolationSegment1, segment1)
	segment2, _ := cc.GetIsolationSegmentForSpace("space_guid_2")
	assert.EqualValues(t, &cfIsolationSegment2, segment2)
	_, err := cc.GetIsolationSegmentForSpace("invalid_space_guid")
	assert.NotNil(t, err)
}

func TestCCCache_GetIsolationSegmentForOrg(t *testing.T) {
	cc.readData()
	segment1, _ := cc.GetIsolationSegmentForOrg("org_guid_1")
	assert.EqualValues(t, &cfIsolationSegment1, segment1)
	segment2, _ := cc.GetIsolationSegmentForOrg("org_guid_2")
	assert.EqualValues(t, &cfIsolationSegment2, segment2)
	_, err := cc.GetIsolationSegmentForOrg("invalid_org_guid")
	assert.NotNil(t, err)
}
