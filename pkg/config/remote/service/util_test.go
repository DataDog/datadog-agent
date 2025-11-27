// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package service

import (
	"encoding/base32"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/proto/msgpgo"
)

func TestAuthKeys(t *testing.T) {
	tests := []struct {
		rcKey  string
		apiKey string
		parJWT string
		err    bool
		output remoteConfigAuthKeys
	}{
		{apiKey: "37d58c60b8ac337293ce2ca6b28b19eb", rcKey: generateKey(t, 2, "datadoghq.com", "58d58c60b8ac337293ce2ca6b28b19eb"), output: remoteConfigAuthKeys{
			apiKey:   "37d58c60b8ac337293ce2ca6b28b19eb",
			rcKeySet: true,
			rcKey:    &msgpgo.RemoteConfigKey{AppKey: "58d58c60b8ac337293ce2ca6b28b19eb", OrgID: 2, Datacenter: "datadoghq.com"},
		}},
		{apiKey: "37d58c60b8ac337293ce2ca6b28b19eb", rcKey: "", output: remoteConfigAuthKeys{
			apiKey:   "37d58c60b8ac337293ce2ca6b28b19eb",
			rcKeySet: false,
		}},
		{rcKey: generateKey(t, 2, "datadoghq.com", ""), err: true},
		{rcKey: generateKey(t, 2, "", "app_Key"), err: true},
		{rcKey: generateKey(t, 0, "datadoghq.com", "app_Key"), err: true},
		{parJWT: "myJWT", err: false, output: remoteConfigAuthKeys{
			parJWT: "myJWT",
		}},
		{parJWT: "myJWT", apiKey: "myAPIKey", err: false, output: remoteConfigAuthKeys{
			parJWT: "myJWT",
			apiKey: "myAPIKey",
		}},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s|%s", test.apiKey, test.rcKey), func(tt *testing.T) {
			output, err := getRemoteConfigAuthKeys(test.apiKey, test.rcKey, test.parJWT)
			if test.err {
				assert.Error(tt, err)
			} else {
				assert.Equal(tt, test.output, output)
				assert.NoError(tt, err)
			}
		})
	}
}

func generateKey(t *testing.T, orgID int64, datacenter string, appKey string) string {
	key := msgpgo.RemoteConfigKey{
		AppKey:     appKey,
		OrgID:      orgID,
		Datacenter: datacenter,
	}
	rawKey, err := key.MarshalMsg(nil)
	if err != nil {
		t.Fatal(err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(rawKey)
}
