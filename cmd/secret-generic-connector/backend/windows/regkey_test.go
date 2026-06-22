// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

package windows

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows/registry"
)

func TestNewWindowsRegkeyBackendDefaultsToLocalMachine(t *testing.T) {
	backend, err := NewWindowsRegkeyBackend(nil)
	assert.NoError(t, err)
	assert.NotNil(t, backend)
	assert.Equal(t, registry.LOCAL_MACHINE, backend.RootKey)
}

func TestNewWindowsRegkeyBackendEmptyRootKeyDefaults(t *testing.T) {
	backend, err := NewWindowsRegkeyBackend(map[string]interface{}{"root_key": ""})
	assert.NoError(t, err)
	assert.Equal(t, registry.LOCAL_MACHINE, backend.RootKey)
}

func TestNewWindowsRegkeyBackendValidRootKeys(t *testing.T) {
	cases := map[string]registry.Key{
		"HKLM":                registry.LOCAL_MACHINE,
		"HKEY_LOCAL_MACHINE":  registry.LOCAL_MACHINE,
		"HKCU":                registry.CURRENT_USER,
		"HKEY_CURRENT_USER":   registry.CURRENT_USER,
		"HKCR":                registry.CLASSES_ROOT,
		"HKEY_CLASSES_ROOT":   registry.CLASSES_ROOT,
		"HKU":                 registry.USERS,
		"HKEY_USERS":          registry.USERS,
		"HKCC":                registry.CURRENT_CONFIG,
		"HKEY_CURRENT_CONFIG": registry.CURRENT_CONFIG,
	}
	for input, expected := range cases {
		t.Run(input, func(t *testing.T) {
			backend, err := NewWindowsRegkeyBackend(map[string]interface{}{"root_key": input})
			assert.NoError(t, err)
			assert.Equal(t, expected, backend.RootKey)
		})
	}
}

func TestNewWindowsRegkeyBackendIsCaseInsensitive(t *testing.T) {
	backend, err := NewWindowsRegkeyBackend(map[string]interface{}{"root_key": "hklm"})
	assert.NoError(t, err)
	assert.Equal(t, registry.LOCAL_MACHINE, backend.RootKey)

	backend, err = NewWindowsRegkeyBackend(map[string]interface{}{"root_key": "hKeY_cUrReNt_UsEr"})
	assert.NoError(t, err)
	assert.Equal(t, registry.CURRENT_USER, backend.RootKey)
}

func TestNewWindowsRegkeyBackendUnknownRootKey(t *testing.T) {
	backend, err := NewWindowsRegkeyBackend(map[string]interface{}{"root_key": "HKEY_NOT_A_REAL_HIVE"})
	assert.Nil(t, backend)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown registry root")
}

func TestNewWindowsRegkeyBackendInvalidConfigType(t *testing.T) {
	// root_key must decode to a string; passing an int triggers the
	// mapstructure decode error path.
	backend, err := NewWindowsRegkeyBackend(map[string]interface{}{"root_key": 42})
	assert.Nil(t, backend)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to map backend configuration")
}

func TestGetSecretOutputNoDelimiter(t *testing.T) {
	backend, err := NewWindowsRegkeyBackend(nil)
	assert.NoError(t, err)

	ctx := context.Background()
	out := backend.GetSecretOutput(ctx, "missing_delimiter_key")
	assert.Nil(t, out.Value)
	assert.NotNil(t, out.Error)
	assert.Contains(t, *out.Error, "no delimeter found")
}

func TestGetSecretOutputEmptyKey(t *testing.T) {
	backend, err := NewWindowsRegkeyBackend(nil)
	assert.NoError(t, err)

	ctx := context.Background()
	out := backend.GetSecretOutput(ctx, "")
	assert.Nil(t, out.Value)
	assert.NotNil(t, out.Error)
	assert.Contains(t, *out.Error, "no delimeter found")
}

// ProductName under this key is set by the OS installer and is present on
// every supported Windows build, so it's a stable target for an integration
// test against the live registry.
func TestGetSecretOutputValidKey(t *testing.T) {
	backend, err := NewWindowsRegkeyBackend(nil)
	assert.NoError(t, err)

	ctx := context.Background()
	out := backend.GetSecretOutput(ctx, `SOFTWARE\Microsoft\Windows NT\CurrentVersion:ProductName`)
	assert.Nil(t, out.Error)
	assert.NotNil(t, out.Value)
	assert.NotEmpty(t, *out.Value)
}

// Same registry value, but reached via the explicitly configured HKLM root —
// exercises the dynamic root_key path end-to-end.
func TestGetSecretOutputValidKeyExplicitHKLM(t *testing.T) {
	backend, err := NewWindowsRegkeyBackend(map[string]interface{}{"root_key": "HKLM"})
	assert.NoError(t, err)

	ctx := context.Background()
	out := backend.GetSecretOutput(ctx, `SOFTWARE\Microsoft\Windows NT\CurrentVersion:ProductName`)
	assert.Nil(t, out.Error)
	assert.NotNil(t, out.Value)
	assert.NotEmpty(t, *out.Value)
}

// Environment is always populated under HKCU on a logged-in user session and
// confirms a non-default root resolves correctly.
func TestGetSecretOutputValidKeyHKCU(t *testing.T) {
	backend, err := NewWindowsRegkeyBackend(map[string]interface{}{"root_key": "HKCU"})
	assert.NoError(t, err)

	ctx := context.Background()
	out := backend.GetSecretOutput(ctx, `Environment:TEMP`)
	assert.Nil(t, out.Error)
	assert.NotNil(t, out.Value)
	assert.NotEmpty(t, *out.Value)
}

func TestGetSecretOutputPathNotFound(t *testing.T) {
	backend, err := NewWindowsRegkeyBackend(nil)
	assert.NoError(t, err)

	ctx := context.Background()
	out := backend.GetSecretOutput(ctx, `SOFTWARE\Datadog\DoesNotExist_TestPath:SomeValue`)
	assert.Nil(t, out.Value)
	assert.NotNil(t, out.Error)
}

func TestGetSecretOutputValueNotFound(t *testing.T) {
	backend, err := NewWindowsRegkeyBackend(nil)
	assert.NoError(t, err)

	ctx := context.Background()
	out := backend.GetSecretOutput(ctx, `SOFTWARE\Microsoft\Windows NT\CurrentVersion:DoesNotExist_TestValue`)
	assert.Nil(t, out.Value)
	assert.NotNil(t, out.Error)
}
