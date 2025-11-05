// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

//nolint:revive // TODO(PLINT) Fix revive linter
package wlan

/*
#cgo CFLAGS: -I .
#cgo LDFLAGS: -framework CoreWLAN -framework CoreLocation -framework Foundation -framework Cocoa
#include "wlan_darwin.h"
#include <stdlib.h>
*/
import "C"
import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"unsafe"
)

// phyMode represents the PHY mode of the WiFi connection.
type phyMode int

// See: https://developer.apple.com/documentation/corewlan/cwphymode?language=objc
const (
	phyModeNone phyMode = iota // No PHY mode.
	phyMode11a                 // IEEE 802.11a PHY.
	phyMode11b                 // IEEE 802.11b PHY.
	phyMode11g                 // IEEE 802.11g PHY.
	phyMode11n                 // IEEE 802.11n PHY.
	phyMode11ac                // IEEE 802.11ac PHY.
)

func (phy phyMode) String() string {
	switch phy {
	case phyModeNone:
		return "None"
	case phyMode11a:
		return "802.11a"
	case phyMode11b:
		return "802.11b"
	case phyMode11g:
		return "802.11g"
	case phyMode11n:
		return "802.11n"
	case phyMode11ac:
		return "802.11ac"
	default:
		return ""
	}
}

func GetWiFiInfo() (wifiInfo, error) {
	info := C.GetWiFiInformation()

	ssid := C.GoString(info.ssid)
	bssid := C.GoString(info.bssid)
	hardwareAddress := C.GoString(info.hardwareAddress)
	errorMessage := C.GoString(info.errorMessage)

	// important: free the C strings fields
	if info.ssid != nil {
		C.free(unsafe.Pointer(info.ssid))
	}
	if info.bssid != nil {
		C.free(unsafe.Pointer(info.bssid))
	}
	if info.hardwareAddress != nil {
		C.free(unsafe.Pointer(info.hardwareAddress))
	}
	if info.errorMessage != nil {
		C.free(unsafe.Pointer(info.errorMessage))
	}

	wifiInfo := wifiInfo{
		rssi:         int(info.rssi),
		ssid:         ssid,
		bssid:        bssid,
		channel:      int(info.channel),
		noise:        int(info.noise),
		noiseValid:   true,
		transmitRate: float64(info.transmitRate), // in Mbps
		macAddress:   hardwareAddress,
		phyMode:      phyMode(info.activePHYMode).String(),
	}

	var err error
	if errorMessage != "" {
		err = errors.New(errorMessage)
	}

	return wifiInfo, err
}

// HasLocationPermission checks if the agent has location permission
func HasLocationPermission() bool {
	return bool(C.HasLocationPermission())
}

// RequestLocationPermissionGUI launches a GUI session to request location permission
// This should be called from the agent's request-location-permission subcommand
func RequestLocationPermissionGUI() {
	C.RequestLocationPermissionGUI()
}

// RequestLocationPermission spawns the agent subcommand to request location permission
func (c *WLANCheck) RequestLocationPermission() error {
	// Get the current user's UID (agent runs as root, but we need user session)
	uid := os.Getenv("SUDO_UID")
	if uid == "" {
		// Try to find the console user
		cmd := exec.Command("/usr/bin/stat", "-f", "%u", "/dev/console")
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("could not determine user UID: %w", err)
		}
		uid = string(output[:len(output)-1]) // trim newline
	}

	// Get agent binary path
	agentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine agent path: %w", err)
	}

	// Spawn the GUI as the user using launchctl asuser
	cmd := exec.Command(
		"/bin/launchctl",
		"asuser",
		uid,
		agentPath,
		"wlan",
		"request-location-permission",
	)

	// Start in background - don't wait for completion
	return cmd.Start()
}
