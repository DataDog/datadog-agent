// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filesystem

import "os"

// FileExists returns true if a file exists and is accessible, false otherwise
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
