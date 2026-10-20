// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package snmp

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

const credentialsYAML = `
init_config:
instances:
  - tags:
    - credential-name:v2c-public
    network_address: 1.2.3.4/32
    ignored_ip_addresses:
      - 1.2.3.4
    snmp_version: "2c"
    community_string: public
  - tags:
    - credential-name:v3-full
    network_address: 1.2.3.4/32
    ignored_ip_addresses:
      - 1.2.3.4
    snmp_version: "3"
    user: test-user
    authProtocol: SHA
    authKey: test-auth-key
    privProtocol: AES
    privKey: test-priv-key
    context_name: test-context
`

// newTestConfig returns a config whose confd_path holds the given credential
// file, or no credential file at all when body is empty.
func newTestConfig(t *testing.T, body string) model.BuildableConfig {
	t.Helper()

	confd := t.TempDir()
	if body != "" {
		writeCredentials(t, confd, body)
	}

	cfg := configmock.New(t)
	cfg.Set("confd_path", confd, model.SourceAgentRuntime)
	return cfg
}

func writeCredentials(t *testing.T, confd, body string) {
	t.Helper()

	path := filepath.Join(confd, credentialsFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

func TestLoadIndexesTheCredentialFileByID(t *testing.T) {
	store := newCredentialStore(newTestConfig(t, credentialsYAML))

	creds, err := store.load()
	require.NoError(t, err)
	require.Len(t, creds, 2)

	assert.Equal(t, credential{
		ID:              "v2c-public",
		Tags:            []string{"credential-name:v2c-public"},
		SNMPVersion:     "2c",
		CommunityString: "public",
	}, creds["v2c-public"])

	assert.Equal(t, credential{
		ID:           "v3-full",
		Tags:         []string{"credential-name:v3-full"},
		SNMPVersion:  "3",
		User:         "test-user",
		AuthProtocol: "SHA",
		AuthKey:      "test-auth-key",
		PrivProtocol: "AES",
		PrivKey:      "test-priv-key",
		ContextName:  "test-context",
	}, creds["v3-full"])
}

func TestLoadAcceptsAnIntegerSNMPVersion(t *testing.T) {
	store := newCredentialStore(newTestConfig(t, `
init_config:
instances:
  - tags:
    - credential-name:int-version
    network_address: 1.2.3.4/32
    ignored_ip_addresses:
      - 1.2.3.4
    snmp_version: 2
    community_string: public
`))

	creds, err := store.load()
	require.NoError(t, err)
	assert.Equal(t, version("2"), creds["int-version"].SNMPVersion)
	assert.NoError(t, validate(creds["int-version"]))
}

func TestLoadIgnoresTagsOtherThanTheCredentialName(t *testing.T) {
	store := newCredentialStore(newTestConfig(t, `
init_config:
instances:
  - tags:
    - env:prod
    - credential-name:tagged
    - team:ndm
    network_address: 1.2.3.4/32
    ignored_ip_addresses:
      - 1.2.3.4
    snmp_version: 2
    community_string: public
`))

	creds, err := store.load()
	require.NoError(t, err)
	assert.Equal(t, []string{"tagged"}, keysOf(creds))
}

func TestLoadReadsTheFileUnderConfdPath(t *testing.T) {
	cfg := newTestConfig(t, credentialsYAML)

	assert.Equal(t,
		filepath.Join(cfg.GetString("confd_path"), "snmp.d", "snmp_credentials.yaml"),
		newCredentialStore(cfg).path(),
	)
}

func TestLoadOfAnAbsentFileIsEmptyAndNotAnError(t *testing.T) {
	store := newCredentialStore(newTestConfig(t, ""))

	creds, err := store.load()
	require.NoError(t, err)
	assert.Empty(t, creds)
}

func TestLoadOfAnEmptyFileIsEmptyAndNotAnError(t *testing.T) {
	store := newCredentialStore(newTestConfig(t, "\n"))

	creds, err := store.load()
	require.NoError(t, err)
	assert.Empty(t, creds)
}

func TestLoadOfAMalformedFileErrorsWithoutQuotingItsContent(t *testing.T) {
	store := newCredentialStore(newTestConfig(t, `
instances:
  - tags:
    - credential-name:v2c
    community_string: s3cret-community
    snmp_version: [not, a, string]
`))

	_, err := store.load()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "s3cret-community")
}

func TestLoadSkipsAnInstanceWithNoCredentialNameTag(t *testing.T) {
	store := newCredentialStore(newTestConfig(t, `
instances:
  - snmp_version: "2c"
    community_string: public
  - tags:
    - credential-name:kept
    snmp_version: "2c"
    community_string: public
`))

	creds, err := store.load()
	require.NoError(t, err)
	assert.Equal(t, []string{"kept"}, keysOf(creds))
}

func TestLoadKeepsTheFirstOfTwoEntriesSharingAnID(t *testing.T) {
	store := newCredentialStore(newTestConfig(t, `
instances:
  - tags:
    - credential-name:dup
    snmp_version: "2c"
    community_string: first
  - tags:
    - credential-name:dup
    snmp_version: "2c"
    community_string: second
`))

	creds, err := store.load()
	require.NoError(t, err)
	assert.Equal(t, "first", creds["dup"].CommunityString)
}

func TestLoadRereadsTheFileEveryTime(t *testing.T) {
	cfg := newTestConfig(t, credentialsYAML)
	store := newCredentialStore(cfg)

	first, err := store.load()
	require.NoError(t, err)
	assert.Equal(t, "public", first["v2c-public"].CommunityString)

	writeCredentials(t, cfg.GetString("confd_path"), `
instances:
  - tags:
    - credential-name:v2c-public
    snmp_version: "2c"
    community_string: rotated
`)

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
		{name: "v2", cred: credential{ID: "c", SNMPVersion: "2", CommunityString: "public"}},
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
			cred:    credential{ID: "c", SNMPVersion: "4"},
			wantErr: `unknown SNMP version "4"`,
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
