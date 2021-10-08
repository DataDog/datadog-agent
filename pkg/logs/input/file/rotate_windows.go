// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build windows

package file

import (
	"os"
)

// DidRotate is not implemented on windows, log rotations are handled by the
// tailer for now.
func DidRotate(file *os.File, lastReadOffset int64) (bool, error) {
	return false, nil
}
