// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packages

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// getExtensionStoragePath returns the path where extension lists should be stored.
// For OCI packages use RootTmpDir (temporary storage under installer data),
// otherwise use the package path itself.
//
//nolint:unused // Used in shared extension functions
func getExtensionStoragePath(packagePath string) string {
	if strings.HasPrefix(packagePath, paths.PackagesPath) {
		return paths.RootTmpDir
	}
	return packagePath
}
