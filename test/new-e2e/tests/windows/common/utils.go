// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"

	"github.com/cenkalti/backoff/v4"
	"golang.org/x/text/encoding/unicode"
)

// ConvertUTF16ToUTF8 converts a byte slice from UTF-16 to UTF-8
//
// UTF-16 little-endian (UTF-16LE) is the encoding standard in the Windows operating system.
// https://learn.microsoft.com/en-us/globalization/encoding/transformations-of-unicode-code-points
func ConvertUTF16ToUTF8(content []byte) ([]byte, error) {
	utf16 := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	utf8, err := utf16.NewDecoder().Bytes(content)
	if err != nil {
		return nil, fmt.Errorf("failed to convert UTF-16 to UTF-8: %v", err)
	}
	return utf8, nil
}

// TrimTrailingSlashesAndLower trims trailing slashes and lowercases the path for use in simple comparisons.
//
// Some cases may require a more comprehensive comparison, which could be made by normalizing the path on the host
// via PowerShell, to support removing dot paths, resolving links, etc
func TrimTrailingSlashesAndLower(path string) string {
	// Normalize paths
	// trim trailing slashes
	path = strings.TrimSuffix(path, `\`)
	// windows paths are case-insensitive
	path = strings.ToLower(path)
	return path
}

// MeasureCommand uses Measure-Command and returns time taken (in milliseconds), out, err
func MeasureCommand(host *components.RemoteHost, command string) (time.Duration, string, error) {
	// *>&1 redirects all streams
	// https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_redirection
	powershellCommand := fmt.Sprintf(`
		$taken = Measure-Command { $cmdout = $( %s ) *>&1 | Out-String }
		@{
			TotalMilliseconds=$taken.TotalMilliseconds
			Output=$cmdout
		} | ConvertTo-JSON`, command)
	out, err := host.Execute(powershellCommand)
	if err != nil {
		return 0, out, err
	}
	type measureCommandOutput struct {
		TotalMilliseconds float64 `json:"TotalMilliseconds"`
		Output            string  `json:"Output"`
	}
	var m measureCommandOutput
	err = json.Unmarshal([]byte(out), &m)
	if err != nil {
		return 0, "", fmt.Errorf("failed to unmarshal Measure-Command output: %w\n%s", err, out)
	}
	return time.Duration(m.TotalMilliseconds) * time.Millisecond, m.Output, nil
}

// FileNameFromPath returns the last part of a path, which is the file name. Trailing slashes are removed before extracting the last element.
// Supports both backslashes and forward slashes, by returning the last part after the last backslash or forward slash.
func FileNameFromPath(path string) string {
	// remove trailing slash, if any
	path = strings.TrimSuffix(path, `\`)
	path = strings.TrimSuffix(path, `/`)
	// get index of last backslash and last forward slash
	lastBackslash := strings.LastIndex(path, `\`)
	lastForwardSlash := strings.LastIndex(path, `/`)
	// if no backslash or forward slash is found, return the path as is
	if lastBackslash == -1 && lastForwardSlash == -1 {
		return path
	}
	// get the last part of the path after the last backslash or forward slash
	if lastBackslash > lastForwardSlash {
		return path[lastBackslash+1:]
	}
	return path[lastForwardSlash+1:]
}

// CleanDirectory removes all children of a directory, but leaves the directory itself.
//
// returns nil if the directory does not exist
func CleanDirectory(host *components.RemoteHost, dir string) error {
	// check if the directory exists
	_, err := host.Lstat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	// get children
	entries, err := host.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("error reading dir %s: %w", dir, err)
	}

	// delete children
	for _, entry := range entries {
		err = host.RemoveAll(filepath.Join(dir, entry.Name()))
		if err != nil {
			return fmt.Errorf("error removing path %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// DefaultRebootBackoff returns a default backoff that is intended to cover normal reboot cases.
// Cases with extended reboot times such as sysprep may require a longer backoff.
//
// Current backoffs: 5s, 7s, 11s, 17s, 25s, 38s, 60s, 60s...for up to 5 minutes
func DefaultRebootBackoff() backoff.BackOff {
	return backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(5*time.Second),
		backoff.WithMaxInterval(60*time.Second),
		backoff.WithMaxElapsedTime(5*time.Minute),
	)
}

// RebootAndWaitWithBackoff reboots the host and waits for the reboot to complete.
func RebootAndWaitWithBackoff(host *components.RemoteHost, b backoff.BackOff) error {
	return WaitForRebootFunc(host, b, func() error {
		// https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.management/restart-computer
		_, _ = host.Execute("Restart-Computer -Force")
		// Is ignoring the error the right thing to do? Does it ever fail, or could it transient failure
		// if reboot happens before the command returns?
		return nil
	})
}

// WaitForRebootFunc waits for the host to reboot by checking the last boot time, rebootFunc is called
// only once and must trigger the reboot when called.
func WaitForRebootFunc(host *components.RemoteHost, b backoff.BackOff, rebootFunc func() error) error {
	// get last boot time
	out, err := host.Execute("(Get-CimInstance Win32_OperatingSystem).LastBootUpTime")
	if err != nil {
		return fmt.Errorf("failed to get last boot time: %w", err)
	}
	lastBootTime := strings.TrimSpace(out)
	fmt.Println("last boot time:", lastBootTime)

	// trigger reboot
	err = rebootFunc()
	if err != nil {
		return fmt.Errorf("failed to trigger reboot: %w", err)
	}

	// wait for last boot time to change, this indicates the reboot has completed
	return backoff.Retry(func() error {
		// need to re-establish connection after reboot
		err := host.Reconnect()
		if err != nil {
			return err
		}
		out, err = host.Execute("(Get-CimInstance Win32_OperatingSystem).LastBootUpTime")
		if err != nil {
			return err
		}
		bootTime := strings.TrimSpace(out)
		fmt.Println("current boot time:", bootTime)
		if bootTime == lastBootTime {
			return fmt.Errorf("boot time has not changed")
		}
		return nil
	}, b)
}
