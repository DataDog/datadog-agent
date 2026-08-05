// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package config

import "github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"

// grantEveryoneRead grants the Everyone group read access on a file: the Windows ACL equivalent
// of a world-readable (0644) file on Linux. os.WriteFile ignores the POSIX mode on Windows, so
// the read ACE must be set explicitly; otherwise the file inherits the restrictive
// C:\ProgramData\Datadog ACL and non-admin identities (e.g. an IIS App Pool identity) cannot
// read it.
func grantEveryoneRead(path string) error {
	return paths.SetFileReadableByEveryone(path)
}
