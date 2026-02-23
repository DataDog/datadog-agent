// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// credLogger captures Warn calls so tests can assert that the resolver logs the right
// messages when credentials are missing or files cannot be closed.
type credLogger struct {
	mu           sync.Mutex
	warnMessages []string
	warnFields   [][]log.Field
}

func (l *credLogger) Debug(msg string, fields ...log.Field)     {}
func (l *credLogger) Debugf(format string, args ...interface{}) {}
func (l *credLogger) Info(msg string, fields ...log.Field)      {}
func (l *credLogger) Infof(format string, args ...interface{})  {}
func (l *credLogger) Error(msg string, fields ...log.Field)     {}
func (l *credLogger) Errorf(format string, args ...interface{}) {}
func (l *credLogger) Warn(msg string, fields ...log.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warnMessages = append(l.warnMessages, msg)
	l.warnFields = append(l.warnFields, fields)
}
func (l *credLogger) Warnf(format string, args ...interface{}) {}
func (l *credLogger) With(fields ...log.Field) log.Logger      { return l }

func (l *credLogger) warns() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.warnMessages))
	copy(out, l.warnMessages)
	return out
}

// writeCredFile writes content to a temp file and returns its path.
func writeCredFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "creds-*.json")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

// --- validateAndReadFile ---

// TestValidateAndReadFile_EmptyPath verifies that passing an empty path is rejected with
// a clear error rather than attempting to open the filesystem root.
func TestValidateAndReadFile_EmptyPath(t *testing.T) {
	_, err := validateAndReadFile(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path is empty")
}

// TestValidateAndReadFile_NonExistentFile verifies that a missing file produces an error.
func TestValidateAndReadFile_NonExistentFile(t *testing.T) {
	_, err := validateAndReadFile(context.Background(), "/tmp/no-such-file-xyz-12345.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not open")
}

// TestValidateAndReadFile_EmptyFile verifies that an empty file is rejected because an
// empty credentials file is always a misconfiguration.
func TestValidateAndReadFile_EmptyFile(t *testing.T) {
	path := writeCredFile(t, "")
	_, err := validateAndReadFile(context.Background(), path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// TestValidateAndReadFile_FileTooLarge verifies that a file exceeding 1 MB is rejected
// to prevent accidental or malicious over-sized credentials.
func TestValidateAndReadFile_FileTooLarge(t *testing.T) {
	path := writeCredFile(t, strings.Repeat("x", maxCredentialsFileSize+1))
	_, err := validateAndReadFile(context.Background(), path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

// TestValidateAndReadFile_ValidFile verifies that a non-empty file within the size limit
// is read and returned successfully.
func TestValidateAndReadFile_ValidFile(t *testing.T) {
	content := `{"auth_type":"Token Auth","credentials":[]}`
	path := writeCredFile(t, content)

	data, err := validateAndReadFile(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

// --- loadConnectionCredentials ---

// TestLoadConnectionCredentials_InvalidJSON verifies that a file that is not valid JSON
// produces an unmarshal error.
func TestLoadConnectionCredentials_InvalidJSON(t *testing.T) {
	path := writeCredFile(t, "not json {{{")
	_, err := loadConnectionCredentials(context.Background(), path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

// TestLoadConnectionCredentials_TokenAuth verifies that a valid token-auth config file is
// parsed into the correct structure.
func TestLoadConnectionCredentials_TokenAuth(t *testing.T) {
	cfg := PrivateConnectionConfig{
		AuthType: privateconnection.TokenAuthType,
		Credentials: []Credential{
			{TokenName: "api-key", TokenValue: "secret-value"},
		},
	}
	data, _ := json.Marshal(cfg)
	path := writeCredFile(t, string(data))

	result, err := loadConnectionCredentials(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, privateconnection.TokenAuthType, result.AuthType)
	require.Len(t, result.Credentials, 1)
	assert.Equal(t, "api-key", result.Credentials[0].TokenName)
	assert.Equal(t, "secret-value", result.Credentials[0].TokenValue)
}

// TestLoadConnectionCredentials_BasicAuth verifies parsing of a basic-auth config file.
func TestLoadConnectionCredentials_BasicAuth(t *testing.T) {
	cfg := PrivateConnectionConfig{
		AuthType: privateconnection.BasicAuthType,
		Credentials: []Credential{
			{Username: "admin", Password: "hunter2"},
		},
	}
	data, _ := json.Marshal(cfg)
	path := writeCredFile(t, string(data))

	result, err := loadConnectionCredentials(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, privateconnection.BasicAuthType, result.AuthType)
	require.Len(t, result.Credentials, 1)
	assert.Equal(t, "admin", result.Credentials[0].Username)
	assert.Equal(t, "hunter2", result.Credentials[0].Password)
}

// --- getSecretFromDockerLocation ---

// TestGetSecretFromDockerLocation_TokenAuth_Found verifies that a matching tokenName returns
// the correct tokenValue.
func TestGetSecretFromDockerLocation_TokenAuth_Found(t *testing.T) {
	cfg := PrivateConnectionConfig{
		AuthType: privateconnection.TokenAuthType,
		Credentials: []Credential{
			{TokenName: "my-token", TokenValue: "super-secret"},
			{TokenName: "other-token", TokenValue: "other-value"},
		},
	}
	data, _ := json.Marshal(cfg)
	path := writeCredFile(t, string(data))

	secret, err := getSecretFromDockerLocation(context.Background(), path, "my-token")
	require.NoError(t, err)
	assert.Equal(t, "super-secret", secret)
}

// TestGetSecretFromDockerLocation_TokenAuth_NotFound verifies that when the token name is
// absent from the credentials file, the resolver logs a Warn (visible in operator dashboards)
// and returns an empty string rather than an error.
func TestGetSecretFromDockerLocation_TokenAuth_NotFound(t *testing.T) {
	capture := &credLogger{}
	ctx := log.ContextWithLogger(context.Background(), capture)

	cfg := PrivateConnectionConfig{
		AuthType:    privateconnection.TokenAuthType,
		Credentials: []Credential{{TokenName: "other-token", TokenValue: "val"}},
	}
	data, _ := json.Marshal(cfg)
	path := writeCredFile(t, string(data))

	secret, err := getSecretFromDockerLocation(ctx, path, "missing-token")
	require.NoError(t, err)
	assert.Empty(t, secret, "missing token should return empty string")

	warns := capture.warns()
	require.Len(t, warns, 1, "a warning must be logged so operators notice the missing credential")
	assert.Contains(t, warns[0], "credential not found")
}

// TestGetSecretFromDockerLocation_BasicAuth_Found verifies that a username-keyed lookup in
// a basic-auth file returns the corresponding password.
func TestGetSecretFromDockerLocation_BasicAuth_Found(t *testing.T) {
	cfg := PrivateConnectionConfig{
		AuthType: privateconnection.BasicAuthType,
		Credentials: []Credential{
			{Username: "admin", Password: "s3cret"},
		},
	}
	data, _ := json.Marshal(cfg)
	path := writeCredFile(t, string(data))

	secret, err := getSecretFromDockerLocation(context.Background(), path, "admin")
	require.NoError(t, err)
	assert.Equal(t, "s3cret", secret)
}

// TestGetSecretFromDockerLocation_UnsupportedAuthType verifies that an unrecognised
// auth_type in the credentials file returns an error rather than silently succeeding.
func TestGetSecretFromDockerLocation_UnsupportedAuthType(t *testing.T) {
	content := `{"auth_type":"OAuth2","credentials":[]}`
	path := writeCredFile(t, content)

	_, err := getSecretFromDockerLocation(context.Background(), path, "token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

// TestGetSecretFromDockerLocation_FileNotFound verifies that a missing path propagates
// the file-open error.
func TestGetSecretFromDockerLocation_FileNotFound(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-file.json")
	_, err := getSecretFromDockerLocation(context.Background(), missing, "token")
	require.Error(t, err)
}

// TestGetSecretFromDockerLocation_MultipleTokensSelectsCorrectOne verifies that when
// multiple tokens exist only the one with the matching name is returned.
func TestGetSecretFromDockerLocation_MultipleTokensSelectsCorrectOne(t *testing.T) {
	creds := make([]Credential, 10)
	for i := range creds {
		creds[i] = Credential{
			TokenName:  fmt.Sprintf("token-%d", i),
			TokenValue: fmt.Sprintf("value-%d", i),
		}
	}
	cfg := PrivateConnectionConfig{
		AuthType:    privateconnection.TokenAuthType,
		Credentials: creds,
	}
	data, _ := json.Marshal(cfg)
	path := writeCredFile(t, string(data))

	secret, err := getSecretFromDockerLocation(context.Background(), path, "token-7")
	require.NoError(t, err)
	assert.Equal(t, "value-7", secret)
}
