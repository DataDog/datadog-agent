// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package common

func (s *Setup) postInstallPackages() (err error) {
	// nothing to do on windows
	return nil
}

func copyInstallerSSI() error {
	// nothing to do on windows
	return nil
}
