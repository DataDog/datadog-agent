// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package collector

// newDepInstallerClient returns nil on Windows for now: the restricted
// dependency endpoint of the installer daemon is unix-socket based and not
// implemented for the named pipe transport yet.
func newDepInstallerClient() DepInstaller {
	return nil
}
