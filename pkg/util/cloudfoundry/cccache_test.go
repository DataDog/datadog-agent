// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build clusterchecks

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

func TestCCCachePolling(t *testing.T) {
	assert.NotZero(t, cc.GetPollAttempts())
	assert.NotZero(t, cc.GetPollSuccesses())
}

func TestCCCache_GetApp(t *testing.T) {
	app1, _ := cc.GetApp("random_app_guid")
	assert.EqualValues(t, cfApp1, *app1)
	app2, _ := cc.GetApp("guid2")
	assert.EqualValues(t, cfApp2, *app2)
	_, err := cc.GetApp("not-existing-guid")
	assert.NotNil(t, err)
}
