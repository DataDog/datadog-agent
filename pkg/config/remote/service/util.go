// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"encoding/base32"
	"encoding/json"
	"errors"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	"github.com/DataDog/datadog-agent/pkg/proto/msgpgo"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

type remoteConfigAuthKeys struct {
	apiKey string

	parJWT string

	rcKeySet bool
	rcKey    *msgpgo.RemoteConfigKey
}

func (k *remoteConfigAuthKeys) apiAuth() api.Auth {
	auth := api.Auth{
		APIKey: k.apiKey,
		PARJWT: k.parJWT,
	}
	if k.rcKeySet {
		auth.UseAppKey = true
		auth.AppKey = k.rcKey.AppKey
	}
	return auth
}

func getRemoteConfigAuthKeys(apiKey string, rcKey string, parJWT string) (remoteConfigAuthKeys, error) {
	if rcKey == "" {
		return remoteConfigAuthKeys{
			apiKey: apiKey,
			parJWT: parJWT,
		}, nil
	}

	// Legacy auth with RC specific keys
	rcKey = strings.TrimPrefix(rcKey, "DDRCM_")
	encoding := base32.StdEncoding.WithPadding(base32.NoPadding)
	rawKey, err := encoding.DecodeString(rcKey)
	if err != nil {
		return remoteConfigAuthKeys{}, err
	}
	var key msgpgo.RemoteConfigKey
	_, err = key.UnmarshalMsg(rawKey)
	if err != nil {
		return remoteConfigAuthKeys{}, err
	}
	if key.AppKey == "" || key.Datacenter == "" || key.OrgID == 0 {
		return remoteConfigAuthKeys{}, errors.New("invalid remote config key")
	}
	return remoteConfigAuthKeys{
		apiKey:   apiKey,
		parJWT:   parJWT,
		rcKeySet: true,
		rcKey:    &key,
	}, nil
}

func buildLatestConfigsRequest(hostname string, agentVersion string, tags []string, traceAgentEnv string, orgUUID string, state uptane.TUFVersions, activeClients []*pbgo.Client, products map[data.Product]struct{}, newProducts map[data.Product]struct{}, lastUpdateErr error, clientState []byte) *pbgo.LatestConfigsRequest {
	productsList := make([]data.Product, len(products))
	i := 0
	for k := range products {
		productsList[i] = k
		i++
	}
	newProductsList := make([]data.Product, len(newProducts))
	i = 0
	for k := range newProducts {
		newProductsList[i] = k
		i++
	}

	lastUpdateErrString := ""
	if lastUpdateErr != nil {
		lastUpdateErrString = lastUpdateErr.Error()
	}
	return &pbgo.LatestConfigsRequest{
		Hostname:                     hostname,
		AgentUuid:                    uuid.GetUUID(),
		AgentVersion:                 agentVersion,
		Products:                     data.ProductListToString(productsList),
		NewProducts:                  data.ProductListToString(newProductsList),
		CurrentConfigSnapshotVersion: state.ConfigSnapshot,
		CurrentConfigRootVersion:     state.ConfigRoot,
		CurrentDirectorRootVersion:   state.DirectorRoot,
		ActiveClients:                activeClients,
		BackendClientState:           clientState,
		HasError:                     lastUpdateErr != nil,
		Error:                        lastUpdateErrString,
		TraceAgentEnv:                traceAgentEnv,
		OrgUuid:                      orgUUID,
		Tags:                         tags,
	}
}

type targetsCustom struct {
	OpaqueBackendState   []byte `json:"opaque_backend_state"`
	AgentRefreshInterval int64  `json:"agent_refresh_interval"`
}

func parseTargetsCustom(rawTargetsCustom []byte) (targetsCustom, error) {
	if len(rawTargetsCustom) == 0 {
		return targetsCustom{}, nil
	}
	var custom targetsCustom
	err := json.Unmarshal(rawTargetsCustom, &custom)
	if err != nil {
		return targetsCustom{}, err
	}
	return custom, nil
}
