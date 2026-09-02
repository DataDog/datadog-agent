// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

package env

// readPersistedProcessManagerEnabled has no persisted state to read on platforms other than
// Linux/Windows, where the installer daemon doesn't run as a supervised system service the same
// way.
func readPersistedProcessManagerEnabled() (bool, bool) {
	return false, false
}
