// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eudm

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/inventory/systeminfo"
)

// deviceName uses the configured Computer Name instead of a network-derived hostname.
func deviceName(info *systeminfo.SystemInfo) (string, error) {
	name := info.ComputerName
	// Match Apple's Computer Name to hostname conversion by stripping apostrophes.
	return strings.ReplaceAll(strings.ReplaceAll(name, "'", ""), "’", ""), nil
}
