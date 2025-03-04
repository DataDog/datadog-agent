// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package installer

import (
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
)

// platformPrepareExperiment runs extra steps needed for the experiment on a specific platform
func (i *installerImpl) platformPrepareExperiment(_ *oci.DownloadedPackage, _ *repository.Repository) error {
	return nil
}
