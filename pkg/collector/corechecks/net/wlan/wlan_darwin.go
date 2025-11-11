// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

//nolint:revive // TODO(PLINT) Fix revive linter
package wlan

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

	checkpkg "github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// guiWiFiData represents the WiFi data structure from GUI IPC
type guiWiFiData struct {
	RSSI               int     `json:"rssi"`
	SSID               string  `json:"ssid"`
	BSSID              string  `json:"bssid"`
	Channel            int     `json:"channel"`
	Noise              int     `json:"noise"`
	NoiseValid         bool    `json:"noise_valid"`
	TransmitRate       float64 `json:"transmit_rate"`
	ReceiveRate        float64 `json:"receive_rate"`
	ReceiveRateValid   bool    `json:"receive_rate_valid"`
	MACAddress         string  `json:"mac_address"`
	PHYMode            string  `json:"phy_mode"`
	LocationAuthorized bool    `json:"location_authorized"`
	Error              *string `json:"error"`
}

// guiIPCResponse represents the IPC response from GUI
type guiIPCResponse struct {
	Success bool         `json:"success"`
	Data    *guiWiFiData `json:"data"`
	Error   *string      `json:"error"`
}

// GetWiFiInfo retrieves WiFi information via IPC from the GUI
func GetWiFiInfo() (wifiInfo, error) {
	// Get console user UID
	uid, err := getConsoleUserUID()
	if err != nil {
		return wifiInfo{}, fmt.Errorf("cannot determine console user: %w", err)
	}

	// Try to fetch from GUI
	socketPath := fmt.Sprintf("/var/run/datadog-agent/wifi-%s.sock", uid)
	info, err := fetchWiFiFromGUI(socketPath, 2*time.Second)
	if err != nil {
		// GUI might not be running - try to launch it
		log.Infof("GUI not responding, attempting to launch it: %v", err)
		if launchErr := launchGUIApp(uid); launchErr != nil {
			log.Warnf("Failed to launch GUI app: %v", launchErr)
			return wifiInfo{}, fmt.Errorf("GUI IPC unavailable and failed to launch: %w", err)
		}

		log.Info("Waiting for GUI to start, retrying connection...")

		// Retry connection with exponential backoff (20 retries over 10 seconds)
		retryErr := checkpkg.Retry(500*time.Millisecond, 20, func() error {
			info, err = fetchWiFiFromGUI(socketPath, 2*time.Second)
			if err != nil {
				return checkpkg.RetryableError{Err: err}
			}
			return nil
		}, "GUI WiFi socket connection")

		if retryErr != nil {
			return wifiInfo{}, fmt.Errorf("GUI IPC unavailable after launch attempt: %w", retryErr)
		}
	}

	return info, nil
}

// getConsoleUserUID returns the UID of the console user
func getConsoleUserUID() (string, error) {
	cmd := exec.Command("/usr/bin/stat", "-f", "%u", "/dev/console")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to stat /dev/console: %w", err)
	}

	uid := strings.TrimSpace(string(output))
	if uid == "" || uid == "0" {
		// No user logged in or at login screen
		return "", fmt.Errorf("no console user logged in (UID: %s)", uid)
	}

	log.Debugf("Console user UID: %s", uid)
	return uid, nil
}

// validateSocketOwnership verifies the socket file is owned by the expected user
func validateSocketOwnership(socketPath string) error {
	// Extract expected UID from socket path (wifi-{UID}.sock)
	base := filepath.Base(socketPath)
	expectedUID := strings.TrimPrefix(base, "wifi-")
	expectedUID = strings.TrimSuffix(expectedUID, ".sock")

	// Get socket file info
	fileInfo, err := os.Stat(socketPath)
	if err != nil {
		return fmt.Errorf("cannot stat socket: %w", err)
	}

	// Get socket file owner UID
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot get socket file stat")
	}

	actualUID := strconv.FormatUint(uint64(stat.Uid), 10)

	// Verify ownership matches expected user
	if actualUID != expectedUID {
		return fmt.Errorf("socket owner mismatch: expected UID %s, got UID %s (potential hijacking attempt)", expectedUID, actualUID)
	}

	log.Debugf("Socket ownership validated: UID %s", actualUID)
	return nil
}

// launchGUIApp attempts to launch the GUI application for the specified user
func launchGUIApp(uid string) error {
	// First try using launchctl to start the LaunchAgent service
	// This is the preferred method as it uses the proper LaunchAgent infrastructure
	cmd := exec.Command("/bin/launchctl", "asuser", uid, "launchctl", "start", "com.datadoghq.gui")
	output, err := cmd.CombinedOutput()
	if err == nil {
		log.Info("Successfully started GUI via LaunchAgent")
		return nil
	}

	log.Debugf("LaunchAgent start failed (may already be loaded): %v, output: %s", err, string(output))

	// Fallback: Try to open the app directly
	// This works even if the LaunchAgent isn't loaded
	cmd = exec.Command("/bin/launchctl", "asuser", uid, "/usr/bin/open", "-a", "Datadog Agent")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to open GUI app: %w, output: %s", err, string(output))
	}

	log.Info("Successfully launched GUI app directly")
	return nil
}

// fetchWiFiFromGUI connects to the GUI Unix socket and fetches WiFi data
func fetchWiFiFromGUI(socketPath string, timeout time.Duration) (wifiInfo, error) {
	// Validate socket ownership before connecting (security: prevent socket hijacking)
	if err := validateSocketOwnership(socketPath); err != nil {
		return wifiInfo{}, fmt.Errorf("socket validation failed: %w", err)
	}

	// Create context with overall timeout for the entire operation
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Use Dialer with context for connection
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return wifiInfo{}, fmt.Errorf("failed to connect to GUI socket %s: %w", socketPath, err)
	}
	defer conn.Close()

	// Set deadline based on context deadline
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return wifiInfo{}, fmt.Errorf("failed to set deadline: %w", err)
		}
	}

	// Send request
	request := map[string]string{"command": "get_wifi_info"}
	requestData, err := json.Marshal(request)
	if err != nil {
		return wifiInfo{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	if _, err := conn.Write(append(requestData, '\n')); err != nil {
		return wifiInfo{}, fmt.Errorf("failed to write request: %w", err)
	}

	log.Debugf("Sent request to GUI: %s", string(requestData))

	// Read response
	reader := bufio.NewReader(conn)
	responseLine, err := reader.ReadString('\n')
	if err != nil {
		return wifiInfo{}, fmt.Errorf("failed to read response: %w", err)
	}

	log.Debugf("Received response from GUI: %s", strings.TrimSpace(responseLine))

	// Parse response
	var response guiIPCResponse
	if err := json.Unmarshal([]byte(responseLine), &response); err != nil {
		return wifiInfo{}, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for errors in response
	if !response.Success || response.Data == nil {
		errMsg := "unknown error"
		if response.Error != nil {
			errMsg = *response.Error
		} else if response.Data != nil && response.Data.Error != nil {
			errMsg = *response.Data.Error
		}
		return wifiInfo{}, fmt.Errorf("GUI returned error: %s", errMsg)
	}

	data := response.Data

	// Log if location is not authorized
	if !data.LocationAuthorized {
		log.Warn("Location permission not granted - SSID/BSSID may be unavailable")
	}

	// Convert to wifiInfo structure
	info := wifiInfo{
		rssi:             data.RSSI,
		ssid:             data.SSID,
		bssid:            data.BSSID,
		channel:          data.Channel,
		noise:            data.Noise,
		noiseValid:       data.NoiseValid,
		transmitRate:     data.TransmitRate,
		receiveRate:      data.ReceiveRate,
		receiveRateValid: data.ReceiveRateValid,
		macAddress:       data.MACAddress,
		phyMode:          data.PHYMode,
	}

	log.Debugf("WiFi info retrieved: SSID=%s, RSSI=%d, PHYMode=%s, LocationAuth=%v",
		info.ssid, info.rssi, info.phyMode, data.LocationAuthorized)

	return info, nil
}
