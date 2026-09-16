// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package snmp

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

const credentialsYAML = `
network_devices:
  snmp_credentials:
    - id: v2c-public
      snmp_version: "2c"
      community_string: public
    - id: v3-full
      snmp_version: "3"
      user: test-user
      authProtocol: SHA
      authKey: test-auth-key
      privProtocol: AES
      privKey: test-priv-key
      context_name: test-context
`

func TestLoadIndexesTheConfiguredCredentialsByID(t *testing.T) {
	store := newCredentialStore(configmock.NewFromYAML(t, credentialsYAML))

	creds, err := store.load()
	require.NoError(t, err)
	require.Len(t, creds, 2)

	assert.Equal(t, credential{
		ID:              "v2c-public",
		SNMPVersion:     "2c",
		CommunityString: "public",
	}, creds["v2c-public"])

	assert.Equal(t, credential{
		ID:           "v3-full",
		SNMPVersion:  "3",
		User:         "test-user",
		AuthProtocol: "SHA",
		AuthKey:      "test-auth-key",
		PrivProtocol: "AES",
		PrivKey:      "test-priv-key",
		ContextName:  "test-context",
	}, creds["v3-full"])
}

func TestLoadOfAnAbsentSectionIsEmptyAndNotAnError(t *testing.T) {
	store := newCredentialStore(configmock.New(t))

	creds, err := store.load()
	require.NoError(t, err)
	assert.Empty(t, creds)
}

func TestLoadSkipsAnEntryWithNoID(t *testing.T) {
	store := newCredentialStore(configmock.NewFromYAML(t, `
network_devices:
  snmp_credentials:
    - snmp_version: "2c"
      community_string: public
    - id: kept
      snmp_version: "2c"
      community_string: public
`))

	creds, err := store.load()
	require.NoError(t, err)
	assert.Equal(t, []string{"kept"}, keysOf(creds))
}

func TestLoadKeepsTheFirstOfTwoEntriesSharingAnID(t *testing.T) {
	store := newCredentialStore(configmock.NewFromYAML(t, `
network_devices:
  snmp_credentials:
    - id: dup
      snmp_version: "2c"
      community_string: first
    - id: dup
      snmp_version: "2c"
      community_string: second
`))

	creds, err := store.load()
	require.NoError(t, err)
	assert.Equal(t, "first", creds["dup"].CommunityString)
}

func TestLoadRereadsTheConfigurationEveryTime(t *testing.T) {
	cfg := configmock.NewFromYAML(t, credentialsYAML)
	store := newCredentialStore(cfg)

	first, err := store.load()
	require.NoError(t, err)
	assert.Equal(t, "public", first["v2c-public"].CommunityString)

	cfg.Set("network_devices.snmp_credentials", []map[string]string{
		{"id": "v2c-public", "snmp_version": "2c", "community_string": "rotated"},
	}, model.SourceAgentRuntime)

	second, err := store.load()
	require.NoError(t, err)
	assert.Equal(t, "rotated", second["v2c-public"].CommunityString)
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cred    credential
		wantErr string
	}{
		{name: "v1", cred: credential{ID: "c", SNMPVersion: "1", CommunityString: "public"}},
		{name: "v2c", cred: credential{ID: "c", SNMPVersion: "2c", CommunityString: "public"}},
		{name: "v3 with no protocols", cred: credential{ID: "c", SNMPVersion: "3", User: "test-user"}},
		{
			name: "v3 with both protocols",
			cred: credential{ID: "c", SNMPVersion: "3", User: "test-user", AuthProtocol: "SHA", PrivProtocol: "AES"},
		},
		{
			name:    "an empty version",
			cred:    credential{ID: "c"},
			wantErr: `unknown SNMP version ""`,
		},
		{
			name:    "an unknown version",
			cred:    credential{ID: "c", SNMPVersion: "2"},
			wantErr: `unknown SNMP version "2"`,
		},
		{
			name:    "an unsupported auth protocol",
			cred:    credential{ID: "c", SNMPVersion: "3", User: "u", AuthProtocol: "NOPE"},
			wantErr: `unsupported authProtocol "NOPE"`,
		},
		{
			name:    "an unsupported priv protocol",
			cred:    credential{ID: "c", SNMPVersion: "3", User: "u", PrivProtocol: "NOPE"},
			wantErr: `unsupported privProtocol "NOPE"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(tc.cred)
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidateNeverNamesACredentialValue(t *testing.T) {
	err := validate(credential{
		ID:              "c",
		SNMPVersion:     "bogus",
		CommunityString: "s3cret-community",
		AuthKey:         "s3cret-auth",
		PrivKey:         "s3cret-priv",
	})

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "s3cret-community")
	assert.NotContains(t, err.Error(), "s3cret-auth")
	assert.NotContains(t, err.Error(), "s3cret-priv")
}

// keysOf returns a credential map's ids, sorted.
func keysOf(creds map[string]credential) []string {
	ids := make([]string, 0, len(creds))
	for id := range creds {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
