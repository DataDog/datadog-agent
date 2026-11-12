// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eudm

import (
	"errors"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/inventory/systeminfo"
	"github.com/stretchr/testify/require"
)

func TestGetUsesLocalHostName(t *testing.T) {
	oldCollect, oldName := collectSystemInfo, localHostName
	t.Cleanup(func() { collectSystemInfo, localHostName = oldCollect, oldName })
	for _, name := range []string{"Jacks-MacBook-2", "My--Mac", "Renamed-Mac"} {
		t.Run(name, func(t *testing.T) {
			localHostName = func() (string, error) { return name, nil }
			collectSystemInfo = func() (*systeminfo.SystemInfo, error) {
				t.Fatal("system information must not be collected when serial appending is disabled")
				return nil, nil
			}
			got, err := Get(false)
			require.NoError(t, err)
			require.Equal(t, name, got)

			collectSystemInfo = func() (*systeminfo.SystemInfo, error) {
				return &systeminfo.SystemInfo{ComputerName: "Ignored Computer Name", SerialNumber: "ABC123"}, nil
			}
			got, err = Get(true)
			require.NoError(t, err)
			require.Equal(t, name+"-abc123", got)
		})
	}
}

func TestGetInvalidLocalHostName(t *testing.T) {
	oldCollect, oldName := collectSystemInfo, localHostName
	t.Cleanup(func() { collectSystemInfo, localHostName = oldCollect, oldName })
	collectSystemInfo = func() (*systeminfo.SystemInfo, error) {
		return &systeminfo.SystemInfo{ComputerName: "Valid Computer Name", SerialNumber: "ABC123"}, nil
	}
	for _, name := range []string{"", " My-Mac ", "Jack's Mac", "Jack’s-Mac", "你好", "-Mac", "Mac-", "localhost", strings.Repeat("a", 254)} {
		t.Run(name, func(t *testing.T) {
			localHostName = func() (string, error) { return name, nil }
			for _, useSerial := range []bool{false, true} {
				got, err := Get(useSerial)
				require.Error(t, err)
				require.Empty(t, got)
			}
		})
	}
	localHostName = func() (string, error) { return "", errors.New("lookup failed") }
	for _, useSerial := range []bool{false, true} {
		_, err := Get(useSerial)
		require.ErrorContains(t, err, "lookup failed")
	}
}

func TestGetLocalHostNameDoesNotTruncateForSerial(t *testing.T) {
	oldCollect, oldName := collectSystemInfo, localHostName
	t.Cleanup(func() { collectSystemInfo, localHostName = oldCollect, oldName })
	name := "My-Mac-2"
	localHostName = func() (string, error) { return name, nil }
	collectSystemInfo = func() (*systeminfo.SystemInfo, error) {
		return &systeminfo.SystemInfo{SerialNumber: strings.Repeat("a", 253-len(name))}, nil
	}
	_, err := Get(true)
	require.ErrorContains(t, err, "too long")
}
