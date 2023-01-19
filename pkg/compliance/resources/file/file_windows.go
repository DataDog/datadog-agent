// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package file

import (
	"errors"
	"os"
)

func getFileUser(fi os.FileInfo) (string, error) {
	return "", errors.New("retrieving file user not supported in windows")
}

func getFileGroup(fi os.FileInfo) (string, error) {
	return "", errors.New("retrieving file group not supported in windows")
}
