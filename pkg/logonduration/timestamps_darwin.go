// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package logonduration

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework OSLog

#include <stdlib.h>

// Returns Unix timestamp (seconds since epoch) or 0 on error
// Query type: 0 = login window time, 1 = login time (sessionDidLogin)
double queryLoginTimestamp(double bootTimestamp, int queryType);

// Returns 1 if FileVault is enabled, 0 if disabled, -1 on error
int checkFileVaultEnabled(void);
*/
import "C"

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxIPCResponseSize = 4096
	ipcTimeout         = 2 * time.Second
)

// GetBootTime returns the system boot time using sysctl kern.boottime
func GetBootTime() (time.Time, error) {
	tv, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return time.Time{}, fmt.Errorf("sysctl kern.boottime failed: %w", err)
	}
	return time.Unix(tv.Sec, int64(tv.Usec)*1000), nil
}

// GetLoginWindowTime queries OSLogStore for when the login window appeared.
// This requires root privileges to access the local log store.
func GetLoginWindowTime(bootTime time.Time) (time.Time, error) {
	bootTimestamp := C.double(float64(bootTime.Unix()))
	result := C.queryLoginTimestamp(bootTimestamp, 0) // 0 = login window time

	if result == 0 {
		return time.Time{}, fmt.Errorf("failed to query login window time from unified logs")
	}

	resultFloat := float64(result)
	return time.Unix(int64(resultFloat), int64((resultFloat-float64(int64(resultFloat)))*1e9)), nil
}

// GetLoginTime queries OSLogStore for when the user entered credentials.
// This requires root privileges to access the local log store.
func GetLoginTime(bootTime time.Time) (time.Time, error) {
	bootTimestamp := C.double(float64(bootTime.Unix()))
	result := C.queryLoginTimestamp(bootTimestamp, 1) // 1 = login time (sessionDidLogin)

	if result == 0 {
		return time.Time{}, fmt.Errorf("failed to query login time from unified logs")
	}

	resultFloat := float64(result)
	return time.Unix(int64(resultFloat), int64((resultFloat-float64(int64(resultFloat)))*1e9)), nil
}

// IsFileVaultEnabled checks if FileVault is enabled.
// This requires root privileges to run fdesetup.
func IsFileVaultEnabled() (bool, error) {
	result := C.checkFileVaultEnabled()
	if result < 0 {
		return false, fmt.Errorf("failed to check FileVault status")
	}
	return result == 1, nil
}

// GetLoginTimestamps collects all login-related timestamps from the system.
// This is the main entry point for the system-probe module.
func GetLoginTimestamps() *LoginTimestamps {
	result := &LoginTimestamps{}

	// Get boot time first (needed as reference for log queries)
	bootTime, err := GetBootTime()
	if err != nil {
		result.Error = fmt.Sprintf("failed to get boot time: %v", err)
		return result
	}

	// Get login window time via CGO to OSLogStore
	start := time.Now()
	if lwt, err := GetLoginWindowTime(bootTime); err == nil {
		result.LoginWindowTime = &lwt
		log.Infof("logonduration: login window time: %v (query took %.3fs)", lwt, time.Since(start).Seconds())
	} else {
		log.Warnf("logonduration: failed to get login window time: %v (query took %.3fs)", err, time.Since(start).Seconds())
	}

	// Get login time via CGO to OSLogStore
	start = time.Now()
	if lt, err := GetLoginTime(bootTime); err == nil {
		result.LoginTime = &lt
		log.Infof("logonduration: login time: %v (query took %.3fs)", lt, time.Since(start).Seconds())
	} else {
		log.Warnf("logonduration: failed to get login time: %v (query took %.3fs)", err, time.Since(start).Seconds())
	}

	// Check FileVault status
	start = time.Now()
	if fv, err := IsFileVaultEnabled(); err == nil {
		result.FileVaultEnabled = &fv
		log.Infof("logonduration: FileVault enabled: %v (query took %.3fs)", fv, time.Since(start).Seconds())
	} else {
		log.Warnf("logonduration: failed to check FileVault status: %v (query took %.3fs)", err, time.Since(start).Seconds())
	}

	return result
}

// GetDesktopReadyData retrieves desktop ready status from the GUI via IPC.
// This doesn't require root privileges.
func GetDesktopReadyData() (*DesktopReadyData, error) {
	uid, err := getConsoleUserUID()
	if err != nil {
		return nil, fmt.Errorf("no console user: %w", err)
	}

	socketPath := filepath.Join(pkgconfigsetup.InstallPath, "run", "ipc", fmt.Sprintf("gui-%s.sock", uid))

	if err := validateSocketOwnership(socketPath, uid); err != nil {
		return nil, fmt.Errorf("socket validation failed: %w", err)
	}

	return fetchDesktopReadyFromGUI(socketPath, ipcTimeout)
}

func getConsoleUserUID() (string, error) {
	cmd := exec.Command("/usr/bin/stat", "-f", "%u", "/dev/console")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to stat /dev/console: %w", err)
	}

	uid := strings.TrimSpace(string(output))
	if uid == "" || uid == "0" {
		return "", fmt.Errorf("no console user logged in (UID: %s)", uid)
	}

	log.Debugf("logonduration: console user UID: %s", uid)
	return uid, nil
}

func validateSocketOwnership(socketPath string, expectedUID string) error {
	fileInfo, err := os.Stat(socketPath)
	if err != nil {
		return fmt.Errorf("cannot stat socket %s: %w", socketPath, err)
	}

	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot get socket file stat")
	}

	actualUID := strconv.FormatUint(uint64(stat.Uid), 10)
	if actualUID != expectedUID {
		return fmt.Errorf("socket owner mismatch: expected UID %s, got UID %s", expectedUID, actualUID)
	}

	return nil
}

func fetchDesktopReadyFromGUI(socketPath string, timeout time.Duration) (*DesktopReadyData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to GUI socket: %w", err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	// Send request for desktop ready status only
	request := map[string]string{"command": "get_desktop_ready"}
	requestData, _ := json.Marshal(request)
	conn.Write(append(requestData, '\n'))

	reader := bufio.NewReaderSize(conn, maxIPCResponseSize)
	responseLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var response DesktopReadyIPCResponse
	if err := json.Unmarshal([]byte(responseLine), &response); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}

	if !response.Success || response.Data == nil {
		errMsg := "unknown error"
		if response.Error != nil {
			errMsg = *response.Error
		}
		return nil, fmt.Errorf("GUI returned error: %s", errMsg)
	}

	return response.Data, nil
}
