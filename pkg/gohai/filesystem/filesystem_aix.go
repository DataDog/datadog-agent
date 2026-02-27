// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build aix

package filesystem

import "errors"

func getFileSystemInfo() ([]MountInfo, error) {
	return nil, errors.New("filesystem info not supported on AIX")
}
