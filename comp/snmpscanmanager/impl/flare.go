// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmpscanmanagerimpl

import (
	"path/filepath"

	flare "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
)

const (
	flareDirName  = "snmp"
	flareFileName = "snmp_scanned_devices"
)

func (m *snmpScanManagerImpl) fillFlare(fb flare.FlareBuilder) error {
	if !persistentcache.Exists(cacheKey) {
		return nil
	}

	filePath, err := persistentcache.GetFileForKey(cacheKey)
	if err != nil {
		return err
	}

	return fb.CopyFileTo(filePath, filepath.Join(flareDirName, flareFileName))
}
