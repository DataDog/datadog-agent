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

var v3App1 = cfclient.V3App{
	Name:          "name_of_app_cc",
	State:         "running",
	Lifecycle:     cfclient.V3Lifecycle{},
	GUID:          "random_app_guid",
	CreatedAt:     "",
	UpdatedAt:     "",
	Relationships: nil,
	Links:         nil,
	Metadata:      cfclient.V3Metadata{},
}

var v3App2 = cfclient.V3App{
	Name:          "app2",
	State:         "running",
	Lifecycle:     cfclient.V3Lifecycle{},
	GUID:          "guid2",
	CreatedAt:     "",
	UpdatedAt:     "",
	Relationships: nil,
	Links:         nil,
	Metadata:      cfclient.V3Metadata{},
}

func (t testCCClient) ListV3AppsByQuery(_ url.Values) ([]cfclient.V3App, error) {
	return []cfclient.V3App{v3App1, v3App2}, nil
}

func TestCCCachePolling(t *testing.T) {
	assert.NotZero(t, cc.GetPollAttempts())
	assert.NotZero(t, cc.GetPollSuccesses())
}

func TestCCCache_GetApp(t *testing.T) {
	app1, _ := cc.GetApp("random_app_guid")
	assert.EqualValues(t, v3App1, *app1)
}
