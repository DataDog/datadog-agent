// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package service

import (
	"encoding/base32"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/go-tuf/data"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func setupCDNClient(t *testing.T, uptaneClient *mockCDNUptane, api *mockAPI) *HTTPClient {
	cfg := model.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))

	cfg.SetWithoutSource("hostname", "test-hostname")
	defer cfg.SetWithoutSource("hostname", "")

	dir := t.TempDir()
	cfg.SetWithoutSource("run_path", dir)
	serializedKey, _ := testRCKey.MarshalMsg(nil)
	cfg.SetWithoutSource("remote_configuration.key", base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(serializedKey))
	baseRawURL := "https://localhost"
	client, err := NewHTTPClient(cfg, baseRawURL, host, site, k, "", "9.9.9")
	require.NoError(t, err)
	if api != nil {
		client.api = api
	}
	if uptaneClient != nil {
		client.uptane = uptaneClient
	}
	return client
}

var (
	host = "test-host"
	site = "test-site"
	k    = "test-api-key"
)

// TestHTTPClientRecentUpdate tests that with a recent (<50s ago) last-update-time,
// the client will not fetch a new update and will return the cached state
func TestHTTPClientRecentUpdate(t *testing.T) {
	api := &mockAPI{}

	uptaneClient := &mockCDNUptane{}
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{
		DirectorRoot:    1,
		DirectorTargets: 1,
		ConfigRoot:      1,
		ConfigSnapshot:  1,
	}, nil)
	uptaneClient.On("DirectorRoot", uint64(1)).Return([]byte(`{"signatures": "testroot1", "signed": "one"}`), nil)
	uptaneClient.On("TargetsMeta").Return([]byte(`{"signatures": "testtargets", "signed": "stuff"}`), nil)
	uptaneClient.On("Targets").Return(
		data.TargetFiles{
			"datadog/2/TESTING1/id/1": {},
			"datadog/2/TESTING2/id/2": {},
		},
		nil,
	)
	uptaneClient.On("TargetFile", "datadog/2/TESTING1/id/1").Return([]byte(`testing_1`), nil)

	client := setupCDNClient(t, uptaneClient, api)
	client.lastUpdate = time.Now()

	u, err := client.GetCDNConfigUpdate([]string{"TESTING1"}, 0, 0, []*pbgo.TargetFileMeta{})
	require.NoError(t, err)
	uptaneClient.AssertExpectations(t)
	require.NotNil(t, u)
	require.Len(t, u.TargetFiles, 1)
	require.Equal(t, []byte(`testing_1`), u.TargetFiles["datadog/2/TESTING1/id/1"])
	require.Len(t, u.ClientConfigs, 1)
	require.Equal(t, "datadog/2/TESTING1/id/1", u.ClientConfigs[0])
	require.Len(t, u.TUFRoots, 1)
	require.Equal(t, []byte(`{"signatures":"testroot1","signed":"one"}`), u.TUFRoots[0])

	response := &pbgo.OrgStatusResponse{
		Enabled:    true,
		Authorized: true,
	}
	api.On("FetchOrgStatus", mock.Anything).Return(response, nil)
}

// TestHTTPClientNegativeOrgStatus tests that with a recent (<50s ago) last-update-time,
// the client will not fetch a new update and will return the cached state
func TestHTTPClientNegativeOrgStatus(t *testing.T) {
	var tests = []struct {
		enabled, authorized bool
		err                 error
	}{
		{false, true, nil},
		{true, false, nil},
		{false, false, nil},
		{true, true, fmt.Errorf("error")},
		{false, false, fmt.Errorf("error")},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("enabled=%t, authorized=%t, err=%v", tt.enabled, tt.authorized, tt.err), func(t *testing.T) {
			api := &mockAPI{}
			response := &pbgo.OrgStatusResponse{
				Enabled:    tt.enabled,
				Authorized: tt.authorized,
			}
			api.On("FetchOrgStatus", mock.Anything).Return(response, tt.err)
			uptaneClient := &mockCDNUptane{}
			uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{
				DirectorRoot:    1,
				DirectorTargets: 1,
				ConfigRoot:      1,
				ConfigSnapshot:  1,
			}, nil)
			uptaneClient.On("DirectorRoot", uint64(1)).Return([]byte(`{"signatures": "testroot1", "signed": "one"}`), nil)
			uptaneClient.On("TargetsMeta").Return([]byte(`{"signatures": "testtargets", "signed": "stuff"}`), nil)
			uptaneClient.On("Targets").Return(
				data.TargetFiles{
					"datadog/2/TESTING1/id/1": {},
					"datadog/2/TESTING2/id/2": {},
				},
				nil,
			)
			uptaneClient.On("TargetFile", "datadog/2/TESTING1/id/1").Return([]byte(`testing_1`), nil)

			client := setupCDNClient(t, uptaneClient, api)
			client.lastUpdate = time.Now().Add(time.Second * -60)

			u, err := client.GetCDNConfigUpdate([]string{"TESTING1"}, 0, 0, []*pbgo.TargetFileMeta{})
			require.NoError(t, err)
			uptaneClient.AssertExpectations(t)
			require.NotNil(t, u)
			require.Len(t, u.TargetFiles, 1)
			require.Equal(t, []byte(`testing_1`), u.TargetFiles["datadog/2/TESTING1/id/1"])
			require.Len(t, u.ClientConfigs, 1)
			require.Equal(t, "datadog/2/TESTING1/id/1", u.ClientConfigs[0])
			require.Len(t, u.TUFRoots, 1)
			require.Equal(t, []byte(`{"signatures":"testroot1","signed":"one"}`), u.TUFRoots[0])
		})
	}
}

// TestHTTPClientUpdateSuccess tests that a stale state will trigger an update of the cached state
// before returning the cached state. In the event that the Update fails, the stale state will be returned.
func TestHTTPClientUpdateSuccess(t *testing.T) {
	var tests = []struct {
		updateSucceeds bool
	}{
		{true},
		{false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("updateSucceeds=%t", tt.updateSucceeds), func(t *testing.T) {
			api := &mockAPI{}
			response := &pbgo.OrgStatusResponse{
				Enabled:    true,
				Authorized: true,
			}
			api.On("FetchOrgStatus", mock.Anything).Return(response, nil)
			uptaneClient := &mockCDNUptane{}
			uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{
				DirectorRoot:    1,
				DirectorTargets: 1,
				ConfigRoot:      1,
				ConfigSnapshot:  1,
			}, nil)
			uptaneClient.On("DirectorRoot", uint64(1)).Return([]byte(`{"signatures": "testroot1", "signed": "one"}`), nil)
			uptaneClient.On("TargetsMeta").Return([]byte(`{"signatures": "testtargets", "signed": "stuff"}`), nil)
			uptaneClient.On("Targets").Return(
				data.TargetFiles{
					"datadog/2/TESTING1/id/1": {},
					"datadog/2/TESTING2/id/2": {},
				},
				nil,
			)
			uptaneClient.On("TargetFile", "datadog/2/TESTING1/id/1").Return([]byte(`testing_1`), nil)

			updateErr := fmt.Errorf("uh oh")
			if tt.updateSucceeds {
				updateErr = nil
			}
			uptaneClient.On("Update").Return(updateErr)

			client := setupCDNClient(t, uptaneClient, api)
			client.lastUpdate = time.Now().Add(time.Second * -60)

			u, err := client.GetCDNConfigUpdate([]string{"TESTING1"}, 0, 0, []*pbgo.TargetFileMeta{})
			require.NoError(t, err)
			uptaneClient.AssertExpectations(t)
			require.NotNil(t, u)
			require.Len(t, u.TargetFiles, 1)
			require.Equal(t, []byte(`testing_1`), u.TargetFiles["datadog/2/TESTING1/id/1"])
			require.Len(t, u.ClientConfigs, 1)
			require.Equal(t, "datadog/2/TESTING1/id/1", u.ClientConfigs[0])
			require.Len(t, u.TUFRoots, 1)
			require.Equal(t, []byte(`{"signatures":"testroot1","signed":"one"}`), u.TUFRoots[0])
		})
	}
}
