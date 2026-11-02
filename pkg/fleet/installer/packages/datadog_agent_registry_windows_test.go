// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package packages

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func explicitRegistryAllowMasks(t *testing.T, path string, sid *windows.SID) []uint32 {
	t.Helper()
	key, err := registry.OpenKey(registry.CURRENT_USER, path, windows.READ_CONTROL)
	require.NoError(t, err)
	defer key.Close()
	sd, err := windows.GetSecurityInfo(windows.Handle(key), windows.SE_REGISTRY_KEY, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	control, _, err := sd.Control()
	require.NoError(t, err)
	require.Zero(t, control&windows.SE_DACL_PROTECTED, "registry key should retain inheritance")
	dacl, _, err := sd.DACL()
	require.NoError(t, err)
	var masks []uint32
	for i := 0; i < int(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		require.NoError(t, windows.GetAce(dacl, uint32(i), &ace))
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
			continue
		}
		if (*windows.SID)(unsafe.Pointer(&ace.SidStart)).Equals(sid) {
			masks = append(masks, uint32(ace.Mask))
		}
	}
	return masks
}

func TestCrashCheckpointRegistryPermissions(t *testing.T) {
	// Never use HKLM or an installed Agent: exercise the same grant/revoke ACL
	// helpers on disposable keys under the current user's registry hive.
	root := fmt.Sprintf(`Software\DatadogAgentACLTest-%d-%d`, os.Getpid(), time.Now().UnixNano())
	paths := []string{root + `\agent_crash_reporting`, root + `\windows_crash_reporting`}
	t.Cleanup(func() {
		for _, path := range paths {
			err := registry.DeleteKey(registry.CURRENT_USER, path)
			if !errors.Is(err, registry.ErrNotExist) {
				require.NoError(t, err)
			}
		}
		err := registry.DeleteKey(registry.CURRENT_USER, root)
		if !errors.Is(err, registry.ErrNotExist) {
			require.NoError(t, err)
		}
	})

	sid, err := winutil.GetSidFromUser()
	require.NoError(t, err)
	other, err := windows.CreateWellKnownSid(windows.WinLocalServiceSid)
	require.NoError(t, err)

	// Pre-existing checkpoint values and unrelated explicit rules survive.
	key, _, err := registry.CreateKey(registry.CURRENT_USER, paths[0], registry.ALL_ACCESS)
	require.NoError(t, err)
	require.NoError(t, key.SetStringValue("lastReported", "previous-crash"))
	require.NoError(t, updateRegistryKeyPermissions(key, other, windows.SET_ACCESS))
	require.NoError(t, key.Close())

	for i := 0; i < 2; i++ { // re-grant on upgrade must not accumulate ACEs
		require.NoError(t, grantRegistryPermissions(registry.CURRENT_USER, paths, sid))
	}
	for _, path := range paths {
		masks := explicitRegistryAllowMasks(t, path, sid)
		require.Len(t, masks, 1, "expected one scoped Agent ACE on %s", path)
		require.Equal(t, uint32(windows.READ_CONTROL|registry.QUERY_VALUE|registry.SET_VALUE), masks[0])
	}
	require.Len(t, explicitRegistryAllowMasks(t, paths[0], other), 1)

	// Revocation is idempotent, does not create missing keys, and preserves values.
	require.NoError(t, removeRegistryPermissions(registry.CURRENT_USER, paths, sid))
	require.NoError(t, removeRegistryPermissions(registry.CURRENT_USER, paths, sid))
	for _, path := range paths {
		require.Empty(t, explicitRegistryAllowMasks(t, path, sid))
	}
	require.Len(t, explicitRegistryAllowMasks(t, paths[0], other), 1)
	key, err = registry.OpenKey(registry.CURRENT_USER, paths[0], registry.QUERY_VALUE)
	require.NoError(t, err)
	value, _, err := key.GetStringValue("lastReported")
	require.NoError(t, err)
	require.Equal(t, "previous-crash", value)
	require.NoError(t, key.Close())

	require.NoError(t, registry.DeleteKey(registry.CURRENT_USER, paths[1]))
	require.NoError(t, removeRegistryPermissions(registry.CURRENT_USER, paths, sid))
	_, err = registry.OpenKey(registry.CURRENT_USER, paths[1], registry.READ)
	require.ErrorIs(t, err, registry.ErrNotExist)
}
