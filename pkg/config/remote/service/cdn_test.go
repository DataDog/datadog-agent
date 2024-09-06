// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package service

import (
	"encoding/base32"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/go-tuf/data"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func setupCDNClient(t *testing.T, uptaneClient *uptane.CdnClient, api *mockAPI) *HTTPClient {
	cfg := model.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))

	cfg.SetWithoutSource("hostname", "test-hostname")
	defer cfg.SetWithoutSource("hostname", "")

	dir := t.TempDir()
	cfg.SetWithoutSource("run_path", dir)
	serializedKey, _ := testRCKey.MarshalMsg(nil)
	cfg.SetWithoutSource("remote_configuration.key", base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(serializedKey))
	baseRawURL := "https://localhost"
	opts := make([]HTTPClientOption, 0)
	client, err := NewHTTPClient(cfg, baseRawURL, host, site, k, opts...)
	require.NoError(t, err)
	if api != nil {
		client.api = api
	}
	if uptaneClient != nil {
		client.uptane = uptaneClient
	}
	return client
}

// TODO use mock uptane client and values
var (
	host = "remote-config.datad0g.com"
	site = "datad0g.com"
	k    = "" // TODO don't hardcode
)

func getTestOrgUUIDProvider(orgID int) uptane.OrgUUIDProvider {
	return func() (string, error) {
		return getTestOrgUUIDFromID(orgID), nil
	}
}

func getTestOrgUUIDFromID(orgID int) string {
	return fmt.Sprintf("org-%d-uuid", orgID)
}

// TODO use mock uptane client
func setupHTTPUptaneClient(t *testing.T) *uptane.CdnClient {

	dbPath := path.Join("/opt/tim/datadog-agent", "remote-cdn-config.db")
	db, err := openCacheDB(dbPath, agentVersion, k)
	require.NoError(t, err)
	opts := make([]uptane.CDNClientOption, 0)
	httpCDNClient, err := uptane.NewHTTPClient(db, host, site, k, getTestOrgUUIDProvider(2), opts...)
	require.NoError(t, err)
	return httpCDNClient
}

func TestHTTPClient_Update(t *testing.T) {
	response := &pbgo.OrgStatusResponse{
		Enabled:    true,
		Authorized: true,
	}
	api := &mockAPI{}
	api.On("FetchOrgStatus", mock.Anything).Return(response, nil)

	uptaneClient := setupHTTPUptaneClient(t)
	client := setupCDNClient(t, uptaneClient, api)
	err := client.update()
	require.NoError(t, err)
}

func TestHTTPClient_GetUpdates(t *testing.T) {
	response := &pbgo.OrgStatusResponse{
		Enabled:    true,
		Authorized: true,
	}
	api := &mockAPI{}
	api.On("FetchOrgStatus", mock.Anything).Return(response, nil)

	uptaneClient := setupHTTPUptaneClient(t)
	client := setupCDNClient(t, uptaneClient, api)
	u, err := client.GetCDNConfigUpdate([]string{"DEBUG"}, 0, 0, []*pbgo.TargetFileMeta{})
	require.NoError(t, err)
	require.NotNil(t, u)
	require.Equal(t, 1, len(u.TargetFiles))
	require.Equal(t, 1, len(u.ClientConfigs))
	require.Equal(t, 22, len(u.TUFRoots))

	var signedTargets data.Signed
	err = json.Unmarshal(u.TUFTargets, &signedTargets)
	require.NoError(t, err)
	var targets data.Targets
	err = json.Unmarshal(signedTargets.Signed, &targets)
	require.NoError(t, err)
	require.Equal(t, 1, len(targets.Targets))

	targetsFileVersion := targets.Version
	var cachedTargetFiles []*pbgo.TargetFileMeta
	for p, target := range targets.Targets {
		var hashes []*pbgo.TargetFileHash
		for algo, hash := range target.Hashes {
			hashes = append(hashes, &pbgo.TargetFileHash{
				Algorithm: algo,
				Hash:      string(hash),
			})
			targetFile := &pbgo.TargetFileMeta{
				Path:   p,
				Hashes: hashes,
			}
			cachedTargetFiles = append(cachedTargetFiles, targetFile)
		}
	}
	require.Equal(t, 1, len(cachedTargetFiles))

	u, err = client.GetCDNConfigUpdate([]string{"DEBUG"}, 0, 0, []*pbgo.TargetFileMeta{})
	require.NoError(t, err)
	require.NotNil(t, u)
	require.Equal(t, 1, len(u.TargetFiles))
	require.Equal(t, 1, len(u.ClientConfigs))
	require.Equal(t, 22, len(u.TUFRoots))

	u, err = client.GetCDNConfigUpdate([]string{"DEBUG"}, uint64(targetsFileVersion), 22, cachedTargetFiles)
	require.NoError(t, err)
	require.Nil(t, u)
}
