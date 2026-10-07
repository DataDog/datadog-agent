// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package resolver

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

func TestResolveConnectionTokensV2FromConfig(t *testing.T) {
	config := configmock.New(t)
	config.Set(par.CredentialsValues, map[string]any{"api_token": map[string]any{"value": "secret-value"}}, model.SourceFile)
	resolver, err := NewPrivateCredentialResolver(config, nil)
	require.NoError(t, err)
	conn := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{{
			NameSegments: []string{"root_tokens", "token"},
			Source: &privateactionspb.ConnectionTokenV2_RunnerCredential_{
				RunnerCredential: &privateactionspb.ConnectionTokenV2_RunnerCredential{Key: "api_token"},
			},
		}},
	}

	credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.NoError(t, err)
	require.Len(t, credentials.Tokens, 1)
	assert.Equal(t, "token", credentials.Tokens[0].Name)
	assert.Equal(t, "secret-value", credentials.Tokens[0].Value)

	// Secret refresh replaces the compound setting through configAssignAtPath.
	config.Set(par.CredentialsValues, map[string]any{"api_token": map[string]any{"value": "rotated-value"}}, model.SourceSecret)
	credentials, err = resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.NoError(t, err)
	require.Len(t, credentials.Tokens, 1)
	assert.Equal(t, "rotated-value", credentials.Tokens[0].Value)

	// Invalid updates must fail instead of falling back to stale credentials.
	config.Set(par.CredentialsValues, map[string]any{"api_token": "invalid-secret-value"}, model.SourceSecret)
	credentials, err = resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.Error(t, err)
	assert.Nil(t, credentials)
	assert.NotContains(t, err.Error(), "invalid-secret-value")

	config.Set(par.CredentialsValues, map[string]any{}, model.SourceSecret)
	credentials, err = resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.ErrorContains(t, err, "requested runner credential is not available")
	assert.Nil(t, credentials)
}

func TestResolveConnectionTokensV2FromPlainText(t *testing.T) {
	resolver, err := NewPrivateCredentialResolver(nil, nil)
	require.NoError(t, err)
	conn := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{{
			NameSegments: []string{"root_tokens", "token"},
			Source: &privateactionspb.ConnectionTokenV2_PlainText_{
				PlainText: &privateactionspb.ConnectionTokenV2_PlainText{Value: "plain-value"},
			},
		}},
	}

	credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.NoError(t, err)
	require.Len(t, credentials.Tokens, 1)
	assert.Equal(t, "token", credentials.Tokens[0].Name)
	assert.Equal(t, "plain-value", credentials.Tokens[0].Value)
}

// refreshOnReadConfig simulates a secret refresh immediately after a config read.
type refreshOnReadConfig struct {
	model.Reader
	refresh func()
}

func (c *refreshOnReadConfig) Get(key string) any {
	value := c.Reader.Get(key)
	if key == par.CredentialsValues && c.refresh != nil {
		c.refresh()
		c.refresh = nil
	}
	return value
}

func TestResolveConnectionTokensV2UsesOneSnapshot(t *testing.T) {
	config := configmock.New(t)
	config.Set(par.CredentialsValues, map[string]any{"api_token": map[string]any{"value": "original"}}, model.SourceFile)
	liveConfig := &refreshOnReadConfig{Reader: config}
	resolver, err := NewPrivateCredentialResolver(liveConfig, nil)
	require.NoError(t, err)
	conn := &privateactionspb.ConnectionInfo{CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2}
	for _, name := range []string{"first", "second"} {
		conn.TokensV2 = append(conn.TokensV2, &privateactionspb.ConnectionTokenV2{
			NameSegments: []string{"root_tokens", name},
			Source: &privateactionspb.ConnectionTokenV2_RunnerCredential_{
				RunnerCredential: &privateactionspb.ConnectionTokenV2_RunnerCredential{Key: "api_token"},
			},
		})
	}
	liveConfig.refresh = func() {
		config.Set(par.CredentialsValues, map[string]any{"api_token": map[string]any{"value": "rotated"}}, model.SourceSecret)
	}

	for _, expected := range []string{"original", "rotated"} {
		credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
		require.NoError(t, err)
		require.Len(t, credentials.Tokens, 2)
		for _, token := range credentials.Tokens {
			assert.Equal(t, expected, token.Value)
		}
	}
}

func TestNewPrivateCredentialResolverFromYAML(t *testing.T) {
	yaml := `
private_action_runner:
  credentials:
    values:
      api_token:
        value: resolved-value
`
	mockConfig := configmock.NewFromYAML(t, yaml)

	resolver, err := NewPrivateCredentialResolver(mockConfig, nil)
	require.NoError(t, err)
	conn := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{{
			NameSegments: []string{"root_tokens", "token"},
			Source: &privateactionspb.ConnectionTokenV2_RunnerCredential_{
				RunnerCredential: &privateactionspb.ConnectionTokenV2_RunnerCredential{Key: "api_token"},
			},
		}},
	}
	credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.NoError(t, err)
	require.Len(t, credentials.Tokens, 1)
	assert.Equal(t, "resolved-value", credentials.Tokens[0].Value)

	mockConfig.Set(par.CredentialsValues, map[string]any{"api_token": map[string]any{"value": "rotated-value"}}, model.SourceSecret)
	credentials, err = resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.NoError(t, err)
	require.Len(t, credentials.Tokens, 1)
	assert.Equal(t, "rotated-value", credentials.Tokens[0].Value)
}

func TestNewPrivateCredentialResolverInvalidCredentials(t *testing.T) {
	for _, test := range []struct {
		name string
		yaml string
	}{
		{
			name: "scalar credential",
			yaml: `private_action_runner:
  credentials:
    values:
      api_token: secret-value
`,
		},
		{
			name: "unsupported restriction",
			yaml: `private_action_runner:
  credentials:
    values:
      api_token:
        value: secret-value
        allowed_actions: [some-action]
`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolver, err := NewPrivateCredentialResolver(configmock.NewFromYAML(t, test.yaml), nil)
			require.Error(t, err)
			assert.Nil(t, resolver)
			assert.Contains(t, err.Error(), par.CredentialsValues)
			assert.NotContains(t, err.Error(), "secret-value")
		})
	}
}

func TestScriptCredentialFileResolution(t *testing.T) {
	allowedRoot := t.TempDir()
	validPath := filepath.Join(allowedRoot, "credentials.yaml")
	validContents := "schemaId: script-credentials-v1\n"
	require.NoError(t, os.WriteFile(validPath, []byte(validContents), 0o600))

	outsideRoot := t.TempDir()
	outsidePath := filepath.Join(outsideRoot, "credentials.yaml")
	require.NoError(t, os.WriteFile(outsidePath, []byte("outside secret"), 0o600))

	emptyPath := filepath.Join(allowedRoot, "empty.yaml")
	require.NoError(t, os.WriteFile(emptyPath, nil, 0o600))
	oversizedPath := filepath.Join(allowedRoot, "oversized.yaml")
	require.NoError(t, os.WriteFile(oversizedPath, bytes.Repeat([]byte("x"), maxCredentialsFileSize+1), 0o600))
	missingPath := filepath.Join(allowedRoot, "missing.yaml")
	traversalPath := allowedRoot + string(filepath.Separator) + "nested" + string(filepath.Separator) + ".." + string(filepath.Separator) + "credentials.yaml"

	symlinkPath := filepath.Join(allowedRoot, "symlink.yaml")
	symlinkErr := os.Symlink(outsidePath, symlinkPath)

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{name: "regular file inside allowed root", path: validPath, want: validContents},
		{name: "outside allowed roots", path: outsidePath, wantErr: true},
		{name: "dot dot traversal", path: traversalPath, wantErr: true},
		{name: "empty path", path: "", wantErr: true},
		{name: "empty file", path: emptyPath, wantErr: true},
		{name: "oversized file", path: oversizedPath, wantErr: true},
		{name: "non regular file", path: allowedRoot, wantErr: true},
		{name: "missing file", path: missingPath, wantErr: true},
	}
	if symlinkErr == nil {
		tests = append(tests, struct {
			name    string
			path    string
			want    string
			wantErr bool
		}{name: "symlink escape", path: symlinkPath, wantErr: true})
	}

	resolver := newTestResolver(t, []string{allowedRoot})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), scriptConnectionInfo(tt.path), nil)
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, "could not load script credential file", err.Error())
				if tt.path != "" {
					assert.NotContains(t, err.Error(), tt.path)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, credentials.AsTokenMap()["configFileLocation"])
		})
	}
}

func TestNewPrivateCredentialResolverSkipsInvalidRoots(t *testing.T) {
	validRoot := t.TempDir()
	validPath := filepath.Join(validRoot, "credentials.yaml")
	require.NoError(t, os.WriteFile(validPath, []byte("credentials"), 0o600))

	regularFile := filepath.Join(t.TempDir(), "credentials.yaml")
	require.NoError(t, os.WriteFile(regularFile, []byte("credentials"), 0o600))
	filesystemRoot := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)

	tests := []struct {
		name string
		root string
	}{
		{name: "relative", root: "private-action-runner"},
		{name: "filesystem root", root: filesystemRoot},
		{name: "nonexistent", root: filepath.Join(t.TempDir(), "missing")},
		{name: "regular file", root: regularFile},
	}
	symlinkToFilesystemRoot := filepath.Join(t.TempDir(), "filesystem-root")
	if err := os.Symlink(filesystemRoot, symlinkToFilesystemRoot); err == nil {
		tests = append(tests, struct {
			name string
			root string
		}{name: "symlink to filesystem root", root: symlinkToFilesystemRoot})
	} else {
		t.Logf("skipping symlink case: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := newTestResolver(t, []string{tt.root, validRoot})

			assert.Len(t, resolver.(*privateCredentialResolver).scriptCredentialFileRoots, 1)
			credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), scriptConnectionInfo(validPath), nil)
			require.NoError(t, err)
			assert.Equal(t, "credentials", credentials.AsTokenMap()["configFileLocation"])
		})
	}
}

func TestScriptCredentialFileResolutionRejectsAuthTokenPoC(t *testing.T) {
	path := "/etc/datadog-agent/auth_token"
	resolver := newTestResolver(t, []string{t.TempDir()})

	_, err := resolver.ResolveConnectionInfoToCredential(context.Background(), scriptConnectionInfo(path), nil)

	require.Error(t, err)
	assert.Equal(t, "could not load script credential file", err.Error())
	assert.NotContains(t, err.Error(), path)
}

func TestGenericFileSecretRemainsIndependentFromScriptRoots(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "credentials.json")
	require.NoError(t, os.WriteFile(secretPath, []byte(`{"auth_type":"Token Auth","credentials":[{"tokenName":"apiKey","tokenValue":"secret"}]}`), 0o600))
	resolver := newTestResolver(t, []string{t.TempDir()})
	conn := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_TOKEN_AUTH,
		Tokens: []*privateactionspb.ConnectionToken{
			privateconnection.NewFileSecretToken([]string{privateconnection.RootTokenGroupName, "apiKey"}, secretPath),
		},
	}

	credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)

	require.NoError(t, err)
	assert.Equal(t, "secret", credentials.AsTokenMap()["apiKey"])
}

func TestGenericFileSecretErrorsAreSanitized(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		missing  bool
	}{
		{name: "missing file", missing: true},
		{name: "invalid JSON", contents: `{"secret":"sensitive-value"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sensitive-path.json")
			if !tt.missing {
				require.NoError(t, os.WriteFile(path, []byte(tt.contents), 0o600))
			}
			resolver := newTestResolver(t, nil)
			conn := &privateactionspb.ConnectionInfo{
				CredentialsType: privateactionspb.CredentialsType_TOKEN_AUTH,
				Tokens: []*privateactionspb.ConnectionToken{
					privateconnection.NewFileSecretToken([]string{privateconnection.RootTokenGroupName, "apiKey"}, path),
				},
			}

			_, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)

			require.Error(t, err)
			assert.NotContains(t, err.Error(), path)
			assert.NotContains(t, err.Error(), "sensitive-value")
		})
	}
}

func TestReadCredentialFileUsesOneBoundedHandle(t *testing.T) {
	file := &countingCredentialFile{}
	openCalls := 0

	_, err := readCredentialFile(context.Background(), func() (credentialFile, error) {
		openCalls++
		return file, nil
	}, errCouldNotLoadCredentialFile)

	require.ErrorIs(t, err, errCouldNotLoadCredentialFile)
	assert.Equal(t, 1, openCalls)
	assert.Equal(t, maxCredentialsFileSize+1, file.bytesRead)
	assert.True(t, file.closed)
}

func scriptConnectionInfo(path string) *privateactionspb.ConnectionInfo {
	return &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_TOKEN_AUTH,
		Tokens: []*privateactionspb.ConnectionToken{
			privateconnection.NewYamlFileToken([]string{privateconnection.RootTokenGroupName, "configFileLocation"}, path),
		},
	}
}

func newTestResolver(t *testing.T, roots []string) PrivateCredentialResolver {
	t.Helper()
	resolver, err := NewPrivateCredentialResolver(nil, roots)
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, root := range resolver.(*privateCredentialResolver).scriptCredentialFileRoots {
			assert.NoError(t, root.root.Close())
		}
	})
	return resolver
}

type countingCredentialFile struct {
	bytesRead int
	closed    bool
}

func (f *countingCredentialFile) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	f.bytesRead += len(p)
	return len(p), nil
}

func (f *countingCredentialFile) Stat() (os.FileInfo, error) {
	return staticFileInfo{size: 1, mode: 0o600}, nil
}

func (f *countingCredentialFile) Close() error {
	f.closed = true
	return nil
}

type staticFileInfo struct {
	size int64
	mode os.FileMode
}

func (i staticFileInfo) Name() string       { return "credentials" }
func (i staticFileInfo) Size() int64        { return i.size }
func (i staticFileInfo) Mode() os.FileMode  { return i.mode }
func (i staticFileInfo) ModTime() time.Time { return time.Time{} }
func (i staticFileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i staticFileInfo) Sys() interface{}   { return nil }
