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

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

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

func TestNewPrivateCredentialResolverRejectsFilesystemRoots(t *testing.T) {
	filesystemRoot := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	tests := []struct {
		name string
		path string
	}{
		{name: "filesystem root", path: filesystemRoot},
	}
	symlinkToFilesystemRoot := filepath.Join(t.TempDir(), "filesystem-root")
	if err := os.Symlink(filesystemRoot, symlinkToFilesystemRoot); err == nil {
		tests = append(tests, struct {
			name string
			path string
		}{name: "symlink to filesystem root", path: symlinkToFilesystemRoot})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPrivateCredentialResolver([]string{tt.path})

			require.ErrorIs(t, err, errCouldNotOpenScriptCredentialRoots)
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
	resolver, err := NewPrivateCredentialResolver(roots)
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
