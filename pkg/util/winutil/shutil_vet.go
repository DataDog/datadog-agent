// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows,novet

package winutil

// GetProgramDataDir dummy function because the real one doesn't pass the vet test
func GetProgramDataDir() (path string, err error) {
	return "", nil
}
