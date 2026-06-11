// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

const (
	diskSpdDir    = "C:/diskspd"
	diskSpdZipDst = "C:/DiskSpd.zip"
	// diskSpdArtifactKey is the object key under the E2E artifact bucket (artifact host; s3://agent-e2e-s3-bucket).
	diskSpdArtifactKey = "processes/DiskSpd.zip"
	// DiskSpdExe is the path to the amd64 diskspd executable after extraction
	// (Microsoft DiskSpd release layout: amd64/diskspd.exe inside the zip).
	DiskSpdExe = "C:/diskspd/amd64/diskspd.exe"
)

// setupDiskSpd downloads DiskSpd from the E2E artifact host and extracts it if not already present.
func setupDiskSpd(host *components.RemoteHost) error {
	err := host.HostArtifactClient.Get(diskSpdArtifactKey, diskSpdZipDst)
	if err != nil {
		return fmt.Errorf("failed to download DiskSpd from artifact bucket: %w", err)
	}

	_, err = host.Execute(fmt.Sprintf(
		`if (-Not (Test-Path -Path '%s')) { Expand-Archive -Path '%s' -DestinationPath '%s' -Force }`,
		diskSpdDir, diskSpdZipDst, diskSpdDir))
	if err != nil {
		return fmt.Errorf("failed to extract DiskSpd: %w", err)
	}

	return nil
}
