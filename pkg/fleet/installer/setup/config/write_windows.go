// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package config

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// ensureReadablePermissions grants the Everyone group read access on Windows for files written
// with the world-read bit (e.g. 0644, such as application_monitoring.yaml), mirroring the POSIX
// mode that os.WriteFile applies on other platforms. The perms argument is effectively ignored
// for ACLs on Windows, so the read ACE must be set explicitly; otherwise the file inherits the
// restrictive C:\ProgramData\Datadog ACL and non-admin identities (e.g. an IIS App Pool
// identity) cannot read it.
func ensureReadablePermissions(path string, perms os.FileMode) error {
	if perms&0o004 == 0 {
		return nil
	}
	return paths.SetFileReadableByEveryone(path)
}
