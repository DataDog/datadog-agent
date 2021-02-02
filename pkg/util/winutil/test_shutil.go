// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// +build windows,dovet

package winutil

// GetProgramDataDir dummy function because the real one doesn't pass the vet test
func GetProgramDataDir() (path string, err error) {
	return "", nil
}

// GetProgramDataDirForProduct dummy function because the real one doesn't vet
func GetProgramDataDirForProduct(product string) (path string, err error) {
	return "", nil
}
