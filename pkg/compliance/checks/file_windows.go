// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build windows

package checks

import (
	"errors"
	"os"
)

func getFileOwner(fi os.FileInfo) (string, error) {
	return "", errors.New("retrieving file owner not supported in windows")
}
