// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package endpoint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
)

func TestCheckAPIKeysResolved(t *testing.T) {
	for _, tc := range []struct {
		name    string
		keys    []string
		wantErr bool
	}{
		{name: "plain", keys: []string{"abcdef0123456789abcdef0123456789"}},
		{name: "handle", keys: []string{"ENC[api_key]"}, wantErr: true},
		{name: "handle with padding", keys: []string{" \tENC[api_key] "}, wantErr: true},
		{name: "handle in additional endpoint", keys: []string{"abcdef0123456789abcdef0123456789", "ENC[api_key]"}, wantErr: true},
		{name: "empty", keys: []string{""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eps := make([]apicfg.Endpoint, 0, len(tc.keys))
			for _, k := range tc.keys {
				eps = append(eps, apicfg.Endpoint{APIKey: k, ConfigSettingPath: "api_key"})
			}

			err := CheckAPIKeysResolved(eps)
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unresolved secret handle")
		})
	}
}
