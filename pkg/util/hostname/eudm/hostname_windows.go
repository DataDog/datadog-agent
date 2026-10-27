// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eudm

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/inventory/systeminfo"
)

var osHostname = os.Hostname

// deviceName uses the OS hostname, which on Windows is the configured computer name.
func deviceName(_ *systeminfo.SystemInfo) (string, error) {
	return osHostname()
}
