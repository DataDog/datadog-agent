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
credentials:
  - name: v2c-public
    snmp_version: "2c"
    community_string: public
  - name: v3-full
    snmp_version: "3"
    user: test-user
    authProtocol: SHA
    authKey: test-auth-key
    privProtocol: AES
    privKey: test-priv-key
    context_name: test-context
`

// newTestConfig returns a configuration whose confd_path holds the given
// credential files, keyed by their name under snmp.d/credentials.
func newTestConfig(t *testing.T, files map[string]string) model.BuildableConfig {
	t.Helper()
	cfg := configmock.New(t)
	confd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(confd, credentialsDir), 0o755))
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(confd, credentialsDir, name), []byte(body), 0o600))
	}
	cfg.SetInTest("confd_path", confd)
	return cfg
}

func TestLoadIndexesTheCredentialsByName(t *testing.T) {
	store := newCredentialStore(newTestConfig(t, map[string]string{"creds.yaml": credentialsYAML}))

	creds, err := store.load()
	require.NoError(t, err)
	require.Len(t, creds, 2)

	assert.Equal(t, credential{
		Name:            "v2c-public",
		SNMPVersion:     "2c",
		CommunityString: "public",
	}, creds["v2c-public"])

	assert.Equal(t, credential{
		Name:         "v3-full",
		SNMPVersion:  "3",
		User:         "test-user",
		AuthProtocol: "SHA",
		AuthKey:      "test-auth-key",
		PrivProtocol: "AES",
		PrivKey:      "test-priv-key",
		ContextName:  "test-context",
	}, creds["v3-full"])
}

func TestLoadReadsEveryFileOfTheDirectory(t *testing.T) {
	store := newCredentialStore(newTestConfig(t, map[string]string{
		"a.yaml": "credentials:\n  - name: from-a\n    snmp_version: \"2c\"\n",
		"b.yaml": "credentials:\n  - name: from-b\n    snmp_version: \"2c\"\n",
	}))

	creds, err := store.load()
	require.NoError(t, err)
	assert.Equal(t, []string{"from-a", "from-b"}, namesOf(creds))
}

func TestLoadIgnoresAFileThatIsNotYAML(t *testing.T) {
	store := newCredentialStore(newTestConfig(t, map[string]string{
		"kept.yaml":    "credentials:\n  - name: kept\n    snmp_version: \"2c\"\n",
		"ignored.yml":  "credentials:\n  - name: ignored-yml\n    snmp_version: \"2c\"\n",
		"ignored.json": `{"credentials":[{"name":"ignored-json"}]}`,
	}))

	creds, err := store.load()
	require.NoError(t, err)
	assert.Equal(t, []string{"kept"}, namesOf(creds))
}

func TestLoadOfAnAbsentDirectoryIsEmptyAndNotAnError(t *testing.T) {
	store := newCredentialStore(configmock.New(t))

	creds, err := store.load()
	require.NoError(t, err)
	assert.Empty(t, creds)
}

func TestLoadSkipsAnEntryWithNoName(t *testing.T) {
	store := newCredentialStore(newTestConfig(t, map[string]string{"creds.yaml": `
credentials:
  - snmp_version: "2c"
    community_string: public
  - name: kept
    snmp_version: "2c"
    community_string: public
`}))

	creds, err := store.load()
	require.NoError(t, err)
	assert.Equal(t, []string{"kept"}, namesOf(creds))
}

func TestLoadKeepsTheFirstOfTwoEntriesSharingAName(t *testing.T) {
	store := newCredentialStore(newTestConfig(t, map[string]string{"creds.yaml": `
credentials:
  - name: dup
    snmp_version: "2c"
    community_string: first
  - name: dup
    snmp_version: "2c"
    community_string: second
`}))

	creds, err := store.load()
	require.NoError(t, err)
	assert.Equal(t, "first", creds["dup"].CommunityString)
}

func TestLoadKeepsTheOtherFilesWhenOneCannotBeParsed(t *testing.T) {
	store := newCredentialStore(newTestConfig(t, map[string]string{
		"a-broken.yaml": "credentials: [\n",
		"b-good.yaml":   "credentials:\n  - name: kept\n    snmp_version: \"2c\"\n",
	}))

	creds, err := store.load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a-broken.yaml")
	assert.Equal(t, []string{"kept"}, namesOf(creds))
}

func TestLoadErrorNamesNoFileContent(t *testing.T) {
	store := newCredentialStore(newTestConfig(t, map[string]string{
		"broken.yaml": "credentials:\n  - name: c\n    community_string: [s3cret-community\n",
	}))

	_, err := store.load()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "s3cret-community")
}

func TestLoadRereadsTheFilesEveryTime(t *testing.T) {
	cfg := newTestConfig(t, map[string]string{"creds.yaml": credentialsYAML})
	store := newCredentialStore(cfg)

	first, err := store.load()
	require.NoError(t, err)
	assert.Equal(t, "public", first["v2c-public"].CommunityString)

	path := filepath.Join(cfg.GetString("confd_path"), credentialsDir, "creds.yaml")
	require.NoError(t, os.WriteFile(path, []byte("credentials:\n  - name: v2c-public\n    snmp_version: \"2c\"\n    community_string: rotated\n"), 0o600))

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
		{name: "v1", cred: credential{Name: "c", SNMPVersion: "1", CommunityString: "public"}},
		{name: "v2c", cred: credential{Name: "c", SNMPVersion: "2c", CommunityString: "public"}},
		{name: "v3 with no protocols", cred: credential{Name: "c", SNMPVersion: "3", User: "test-user"}},
		{
			name: "v3 with both protocols",
			cred: credential{Name: "c", SNMPVersion: "3", User: "test-user", AuthProtocol: "SHA", PrivProtocol: "AES"},
		},
		{
			name:    "an empty version",
			cred:    credential{Name: "c"},
			wantErr: `unknown SNMP version ""`,
		},
		{
			name:    "an unknown version",
			cred:    credential{Name: "c", SNMPVersion: "2"},
			wantErr: `unknown SNMP version "2"`,
		},
		{
			name:    "an unsupported auth protocol",
			cred:    credential{Name: "c", SNMPVersion: "3", User: "u", AuthProtocol: "NOPE"},
			wantErr: `unsupported authProtocol "NOPE"`,
		},
		{
			name:    "an unsupported priv protocol",
			cred:    credential{Name: "c", SNMPVersion: "3", User: "u", PrivProtocol: "NOPE"},
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
		Name:            "c",
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

// namesOf returns a credential map's names, sorted.
func namesOf(creds map[string]credential) []string {
	names := make([]string, 0, len(creds))
	for name := range creds {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
