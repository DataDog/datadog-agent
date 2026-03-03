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
	"errors"
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
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// WiFi data validation constants
const (
	// RSSI (Received Signal Strength Indicator) valid range in dBm
	// In reality, the RSSI falls in the range of -10dBm to -100dBm
	// Using some margins, we check from -5dBm to -110dBm
	minRSSI = -110 // Absolute minimum (very weak signal)
	maxRSSI = -5   // Absolute maximum (unrealistic)

	// SSID (Network Name) maximum length per IEEE 802.11 standard
	maxSSIDLength = 32

	// WiFi channel valid range (covers all bands: 2.4GHz, 5GHz, 6GHz)
	minChannel = 0   // 0 indicates inactive/unknown
	maxChannel = 233 // Maximum channel number in 6GHz band (WiFi 6E/7)

	// Noise level valid range in dBm
	minNoise = -105 // minimum or best noise level
	maxNoise = -75  // level above this will be extremely congested or faulty

	// Data rate (PHY link rate) valid range in Mbps
	minDataRate = 0      // 0 indicates unknown/inactive
	maxDataRate = 100000 // 100 Gbps (supports WiFi 8 and beyond)

	// MAC address standard format length (XX:XX:XX:XX:XX:XX)
	macAddressLength = 17

	// Maximum IPC response size (4KB is sufficient for WiFi metadata)
	maxIPCResponseSize = 4096
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
	Success bool         `json:"success" required:"true"`
	Data    *guiWiFiData `json:"data"`
	Error   *string      `json:"error"`
}

// getWiFiInfo is a package-level function variable for testability
// Tests can reassign this to mock WiFi data retrieval
var getWiFiInfo func() (wifiInfo, error)

// GetWiFiInfo retrieves WiFi information via IPC from the GUI/user app
func (c *WLANCheck) GetWiFiInfo() (wifiInfo, error) {
	// If tests have overridden getWiFiInfo, use that instead
	if getWiFiInfo != nil {
		return getWiFiInfo()
	}

	// Production implementation
	// Get console user UID
	uid, err := getConsoleUserUID()
	if err != nil {
		// No console user detected - try any available socket as fallback
		log.Debugf("No console user detected: %v, trying any available socket", err)
		return c.tryAnyAvailableGUISocket()
	}

	// Try to fetch from console user's GUI
	socketPath := filepath.Join(pkgconfigsetup.InstallPath, "run", "ipc", fmt.Sprintf("gui-%s.sock", uid))
	info, err := c.fetchWiFiFromGUI(socketPath, 1*time.Second)
	if err != nil {
		// GUI might not be running - try to launch it
		log.Infof("Console user's GUI not responding, attempting to launch it: %v", err)
		if launchErr := launchGUIApp(uid); launchErr != nil {
			log.Warnf("Failed to launch console user's GUI app: %v", launchErr)
		} else {
			log.Info("Waiting for console user's GUI to start, retrying connection...")

			// Retry connection (give GUI ~1.6s to start, then fallback)
			// This prevents blocking the check scheduler for too long (WiFi metrics are periodic, not critical)
			retryErr := checkpkg.Retry(400*time.Millisecond, 4, func() error {
				info, err = c.fetchWiFiFromGUI(socketPath, 1*time.Second)
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
		return c.tryAnyAvailableGUISocket()
	}

	return info, nil
}

// tryAnyAvailableGUISocket attempts to connect to any available GUI socket
// This is used as a fallback when the console user's GUI is unavailable
// WiFi data is system-wide, so any user's GUI will return identical data
func (c *WLANCheck) tryAnyAvailableGUISocket() (wifiInfo, error) {
	// Find all GUI sockets
	runPath := filepath.Join(pkgconfigsetup.InstallPath, "run", "ipc")
	socketsPattern := filepath.Join(runPath, "gui-*.sock")
	sockets, err := filepath.Glob(socketsPattern)
	if err != nil {
		return wifiInfo{}, fmt.Errorf("failed to search for GUI sockets: %w", err)
	}

	if len(sockets) == 0 {
		return wifiInfo{}, fmt.Errorf("no GUI sockets found in %s", runPath)
	}

	log.Debugf("Found %d GUI socket(s), trying each in order", len(sockets))

	// Try each socket (validateSocketOwnership ensures legitimacy)
	var lastErr error
	for _, socketPath := range sockets {
		log.Debugf("Trying fallback socket: %s", socketPath)
		info, err := c.fetchWiFiFromGUI(socketPath, 1*time.Second)
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
		return errors.New("cannot get socket file stat")
	}

	actualUID := strconv.FormatUint(uint64(stat.Uid), 10)

	// Verify ownership matches expected user
	if actualUID != expectedUID {
		// Special handling for root-owned sockets (likely from installation issue)
		if actualUID == "0" {
			return fmt.Errorf("socket owner mismatch: expected UID %s, got UID 0 (root). "+
				"This may indicate a security issue or installation problem. "+
				"Socket preserved for investigation. "+
				"To fix: sudo rm %s (after investigation)", expectedUID, socketPath)
		}
		return fmt.Errorf("socket owner mismatch: expected UID %s, got UID %s (potential hijacking attempt)", expectedUID, actualUID)
	}

	log.Debugf("Socket ownership validated: UID %s", actualUID)
	return nil
}

// isValidMACAddress validates MAC address format (XX:XX:XX:XX:XX:XX)
func isValidMACAddress(mac string) bool {
	// MAC address should be 17 characters: XX:XX:XX:XX:XX:XX
	if len(mac) != macAddressLength {
		return false
	}
	// Check format: hex pairs separated by colons
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return false
	}
	for _, part := range parts {
		if len(part) != 2 {
			return false
		}
		for _, c := range part {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// validateWiFiData performs defensive validation on WiFi data received from GUI
func validateWiFiData(data *guiWiFiData) error {
	var validationErrors []string

	// RSSI should be in valid range
	if data.RSSI < minRSSI || data.RSSI > maxRSSI {
		msg := fmt.Sprintf("RSSI out of range: %d (expected %d to %d)", data.RSSI, minRSSI, maxRSSI)
		log.Warn(msg)
		validationErrors = append(validationErrors, "invalid_rssi")
	}

	// SSID max length per IEEE 802.11 standard
	if len(data.SSID) > maxSSIDLength {
		msg := fmt.Sprintf("SSID too long: %d bytes (max %d)", len(data.SSID), maxSSIDLength)
		log.Warn(msg)
		validationErrors = append(validationErrors, "invalid_ssid_length")
	}

	// BSSID should be empty or valid MAC address format
	if data.BSSID != "" && !isValidMACAddress(data.BSSID) {
		msg := fmt.Sprintf("Invalid BSSID format: %s (expected XX:XX:XX:XX:XX:XX)", data.BSSID)
		log.Warn(msg)
		validationErrors = append(validationErrors, "invalid_bssid_format")
	}

	// Channel should be in valid WiFi channel range
	if data.Channel < minChannel || data.Channel > maxChannel {
		msg := fmt.Sprintf("Channel out of range: %d (expected %d-%d)", data.Channel, minChannel, maxChannel)
		log.Warn(msg)
		validationErrors = append(validationErrors, "invalid_channel")
	}

	// Noise should be in reasonable range
	if data.Noise < minNoise || data.Noise > maxNoise {
		msg := fmt.Sprintf("Noise out of range: %d (expected %d to %d)", data.Noise, minNoise, maxNoise)
		log.Warn(msg)
		validationErrors = append(validationErrors, "invalid_noise")
	}

	// Data rates should be non-negative and reasonable
	if data.TransmitRate < minDataRate || data.TransmitRate > maxDataRate {
		msg := fmt.Sprintf("Transmit rate out of range: %f (expected %d-%d Mbps)", data.TransmitRate, minDataRate, maxDataRate)
		log.Warn(msg)
		validationErrors = append(validationErrors, "invalid_tx_rate")
	}
	if data.ReceiveRate < minDataRate || data.ReceiveRate > maxDataRate {
		msg := fmt.Sprintf("Receive rate out of range: %f (expected %d-%d Mbps)", data.ReceiveRate, minDataRate, maxDataRate)
		log.Warn(msg)
		validationErrors = append(validationErrors, "invalid_rx_rate")
	}

	// MAC address should be empty or valid format
	if data.MACAddress != "" && !isValidMACAddress(data.MACAddress) {
		msg := fmt.Sprintf("Invalid MAC address format: %s (expected XX:XX:XX:XX:XX:XX)", data.MACAddress)
		log.Warn(msg)
		validationErrors = append(validationErrors, "invalid_mac_format")
	}

	// PHY mode should be from known set
	validPHYModes := map[string]bool{
		"802.11a":  true,
		"802.11b":  true,
		"802.11g":  true,
		"802.11n":  true,
		"802.11ac": true,
		"802.11ax": true,
		"802.11ah": true,
		"802.11ad": true,
		"802.11ay": true,
		"802.11be": true,
		"None":     true,
		"Unknown":  true,
	}
	if !validPHYModes[data.PHYMode] {
		msg := fmt.Sprintf("Unknown PHY mode: %s (expected 802.11a/b/g/n/ac/ax/ah/ad/ay/be/None/Unknown)", data.PHYMode)
		log.Warn(msg)
		validationErrors = append(validationErrors, "invalid_phy_mode")
	}

	// Return aggregated error if any validation failed
	if len(validationErrors) > 0 {
		return fmt.Errorf("WiFi data validation failed: %s", strings.Join(validationErrors, ", "))
	}

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
func (c *WLANCheck) fetchWiFiFromGUI(socketPath string, timeout time.Duration) (wifiInfo, error) {
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

	// Read response with size limit
	reader := bufio.NewReaderSize(conn, maxIPCResponseSize)
	responseLine, err := reader.ReadString('\n')
	if err != nil {
		return wifiInfo{}, fmt.Errorf("failed to read response: %w", err)
	}

	// Validate response size
	if len(responseLine) > maxIPCResponseSize {
		return wifiInfo{}, fmt.Errorf("response too large: %d bytes (max %d)", len(responseLine), maxIPCResponseSize)
	}

	log.Debugf("Received response from GUI: %s", strings.TrimSpace(responseLine))

	// Parse and validate response structure with strict field checking
	// DisallowUnknownFields rejects any JSON with extra/unknown fields (attack detection)
	var response guiIPCResponse
	decoder := json.NewDecoder(strings.NewReader(responseLine))
	decoder.DisallowUnknownFields() // Strict validation: reject extra fields

	if err := decoder.Decode(&response); err != nil {
		// This catches:
		// - Malformed JSON
		// - Type mismatches
		// - Unknown/extra fields (potential attacks)
		return wifiInfo{}, fmt.Errorf("invalid IPC response structure: %w", err)
	}
	log.Debug("IPC response passed structural validation")

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

	// Validate received data (defense against malicious/compromised GUI)
	if err := validateWiFiData(data); err != nil {
		return wifiInfo{}, fmt.Errorf("invalid WiFi data from GUI: %w", err)
	}

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
