// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package service

import (
	"encoding/base32"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/proto/msgpgo"
	"github.com/stretchr/testify/assert"
)

func TestRemoteConfigKey(t *testing.T) {
	tests := []struct {
		input  string
		err    bool
		output *msgpgo.RemoteConfigKey
	}{
		{input: generateKey(t, 2, "datadoghq.com", "58d58c60b8ac337293ce2ca6b28b19eb"), output: &msgpgo.RemoteConfigKey{AppKey: "58d58c60b8ac337293ce2ca6b28b19eb", OrgID: 2, Datacenter: "datadoghq.com"}},
		{input: generateKey(t, 2, "datadoghq.com", ""), err: true},
		{input: generateKey(t, 2, "", "app_Key"), err: true},
		{input: generateKey(t, 0, "datadoghq.com", "app_Key"), err: true},
	}
	for _, test := range tests {
		t.Run(test.input, func(tt *testing.T) {
			output, err := parseRemoteConfigKey(test.input)
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
