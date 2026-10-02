// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package hostname

import (
	"context"
	"errors"
	"runtime"
	"strings"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/inventory/systeminfo"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const infrastructureModeEndUserDevice = "end_user_device"

// for testing purposes
var systeminfoCollect = systeminfo.Collect

// eudmSerialSanitizer strips everything outside the RFC1123 hostname
// character set (letters, digits, '-', '.') instead of mapping it to an
// illegal character like '_'.
var eudmSerialSanitizer = func(r rune) rune {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
		return r
	default:
		return -1
	}
}

// sanitizeEUDMSerialNumber normalizes a raw serial number into an
// RFC1123-safe hostname candidate.
func sanitizeEUDMSerialNumber(serial string) string {
	return strings.Map(eudmSerialSanitizer, strings.TrimSpace(serial))
}

// isEUDM returns true if the agent is configured with infrastructure_mode:
// end_user_device.
func isEUDM() bool {
	return pkgconfigsetup.Datadog().GetString("infrastructure_mode") == infrastructureModeEndUserDevice
}

// fromEUDMSerialNumber resolves the hostname from the device's serial number
// on an end-user-device host. It only fires when infrastructure_mode is
// end_user_device and the platform is one where serial number collection is
// supported. Any failure (gate closed, collection error, missing/invalid
// serial) returns an error so the provider chain falls through to the next
// provider (fqdn/os) unchanged.
func fromEUDMSerialNumber(_ context.Context, _ string) (string, error) {
	if !isEUDM() {
		return "", errors.New("'infrastructure_mode' is not 'end_user_device'")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		return "", errors.New("EUDM serial number hostname resolution is only supported on darwin and windows")
	}

	sysInfo, err := systeminfoCollect()
	if err != nil {
		log.Warnf("EUDM hostname: unable to collect system info for serial number: %s", err)
		return "", err
	}

	serial := sanitizeEUDMSerialNumber(sysInfo.SerialNumber)
	if serial == "" {
		log.Warnf("EUDM hostname: no usable serial number was collected for this device")
		return "", errors.New("no usable serial number was collected")
	}

	if err := validate.ValidHostname(serial); err != nil {
		log.Warnf("EUDM hostname: collected serial number '%s' is not a valid hostname: %s", sysInfo.SerialNumber, err)
		return "", err
	}

	return serial, nil
}
