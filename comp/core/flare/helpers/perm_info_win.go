// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package helpers

// filePermsInfo represents file rights on windows.
type filePermsInfo struct{}

func (p permissionsInfos) add(filePath string) error {
	return nil
}

func (p permissionsInfos) commit() ([]byte, error) {
	return nil, nil
}
