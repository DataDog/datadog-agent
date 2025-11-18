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
		// No console user detected - try any available socket as fallback
		log.Debugf("No console user detected: %v, trying any available socket", err)
		return tryAnyAvailableGUISocket()
	}

	// Try to fetch from console user's GUI
	socketPath := fmt.Sprintf("/var/run/datadog-agent/gui-%s.sock", uid)
	info, err := fetchWiFiFromGUI(socketPath, 2*time.Second)
	if err != nil {
		// GUI might not be running - try to launch it
		log.Infof("Console user's GUI not responding, attempting to launch it: %v", err)
		if launchErr := launchGUIApp(uid); launchErr != nil {
			log.Warnf("Failed to launch console user's GUI app: %v", launchErr)
		} else {
			log.Info("Waiting for console user's GUI to start, retrying connection...")

			// Retry connection with exponential backoff (20 retries over 10 seconds)
			retryErr := checkpkg.Retry(500*time.Millisecond, 20, func() error {
				info, err = fetchWiFiFromGUI(socketPath, 2*time.Second)
				if err != nil {
					return checkpkg.RetryableError{Err: err}
				}
				return nil
			}, "GUI WiFi socket connection")

			if retryErr == nil {
				// Successfully connected after launch
				return info, nil
			}
			log.Infof("Console user's GUI still unavailable after launch attempt: %v", retryErr)
		}

		// Fallback: Try any available socket
		log.Info("Falling back to any available GUI socket...")
		return tryAnyAvailableGUISocket()
	}

	return info, nil
}

// tryAnyAvailableGUISocket attempts to connect to any available GUI socket
// This is used as a fallback when the console user's GUI is unavailable
// WiFi data is system-wide, so any user's GUI will return identical data
func tryAnyAvailableGUISocket() (wifiInfo, error) {
	// Find all GUI sockets
	sockets, err := filepath.Glob("/var/run/datadog-agent/gui-*.sock")
	if err != nil {
		return wifiInfo{}, fmt.Errorf("failed to search for GUI sockets: %w", err)
	}

	if len(sockets) == 0 {
		return wifiInfo{}, fmt.Errorf("no GUI sockets found in /var/run/datadog-agent/")
	}

	log.Debugf("Found %d GUI socket(s), trying each in order", len(sockets))

	// Try each socket (validateSocketOwnership ensures legitimacy)
	var lastErr error
	for _, socketPath := range sockets {
		log.Debugf("Trying fallback socket: %s", socketPath)
		info, err := fetchWiFiFromGUI(socketPath, 2*time.Second)
		if err == nil {
			log.Infof("Successfully retrieved WiFi data from fallback socket: %s", socketPath)
			return info, nil
		}
		log.Debugf("Fallback socket %s failed: %v", socketPath, err)
		lastErr = err
	}

	return wifiInfo{}, fmt.Errorf("all WiFi sockets unavailable, last error: %w", lastErr)
}

// getConsoleUserUID returns the UID of the active console user
//
// Implementation Notes:
// - Uses /dev/console ownership to determine the active user
// - Handles Fast User Switching (FUS) - tracks the foreground user
// - Returns error when at login window (UID 0) or no user logged in
//
// Fast User Switching Behavior:
//   - Identifies the active (foreground) console user
//   - Prefers the active user's GUI socket for data collection
//   - If active user's GUI is unavailable, the caller (GetWiFiInfo) will
//     automatically fall back to any available user's GUI socket
//   - WiFi data is system-wide, so any user's GUI returns identical data
//   - Fallback uses validateSocketOwnership() to prevent socket hijacking
//
// Alternatively, API SCDynamicStoreCopyConsoleUser() from the SystemConfiguration
// framework could be used via CGO. Such approach is more complicated than the
// current /dev/console approach which is pure Go.
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
	// Extract expected UID from socket path (gui-{UID}.sock)
	base := filepath.Base(socketPath)
	expectedUID := strings.TrimPrefix(base, "gui-")
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

// checkGUIServiceStatus checks if the GUI LaunchAgent service is loaded and running
// Returns: "running", "loaded", "not_found", or error
func checkGUIServiceStatus(uid string) (string, error) {
	cmd := exec.Command("/bin/launchctl", "asuser", uid, "launchctl", "list", "com.datadoghq.gui")
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Non-zero exit code typically means service doesn't exist
		if strings.Contains(string(output), "Could not find service") {
			return "not_found", nil
		}
		return "", fmt.Errorf("launchctl list failed: %w, output: %s", err, string(output))
	}

	// Parse output to determine if service is running
	// Running service will have "PID" field in output
	// Example output:
	// {
	//     "PID" = 12345;
	//     "Label" = "com.datadoghq.gui";
	//     ...
	// }
	outputStr := string(output)
	if strings.Contains(outputStr, "\"PID\"") || strings.Contains(outputStr, "PID =") {
		return "running", nil
	}

	// Service is loaded but not running
	return "loaded", nil
}

// startGUIService attempts to start the GUI LaunchAgent service
func startGUIService(uid string) error {
	cmd := exec.Command("/bin/launchctl", "asuser", uid, "launchctl", "start", "com.datadoghq.gui")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start: %w, output: %s", err, string(output))
	}
	return nil
}

// launchGUIApp attempts to launch the GUI application for the specified user
func launchGUIApp(uid string) error {
	// First, check if the LaunchAgent service exists and its current state
	serviceStatus, err := checkGUIServiceStatus(uid)
	if err != nil {
		log.Warnf("Failed to check GUI service status: %v", err)
		// Continue to fallback method
	} else {
		switch serviceStatus {
		case "running":
			log.Info("GUI LaunchAgent is already running")
			return nil
		case "loaded":
			log.Info("GUI LaunchAgent is loaded, attempting to start...")
			if err := startGUIService(uid); err != nil {
				log.Warnf("Failed to start GUI LaunchAgent: %v", err)
				// Continue to fallback method
			} else {
				log.Info("Successfully started GUI via LaunchAgent")
				return nil
			}
		case "not_found":
			log.Warn("GUI LaunchAgent service not found - may have been removed by user")
			log.Info("Attempting to launch GUI app directly as fallback...")
		}
	}

	// Fallback: Try to open the app directly
	// This works even if the LaunchAgent isn't loaded
	log.Debug("Using fallback method: launching GUI app directly")
	cmd := exec.Command("/bin/launchctl", "asuser", uid, "/usr/bin/open", "-a", "Datadog Agent")
	output, err := cmd.CombinedOutput()
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
