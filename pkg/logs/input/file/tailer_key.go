// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package file

import "fmt"

// tailerKey can be implemented by File and Tailer.
type tailerKey interface {
	getPath() string
	getSourceIdentifier() string
}

// buildTailerKey returns a tailer identifier taking into account the filepath and the container ID
// when an container ID is available.
// When no container ID is available, it's only returning the filepath.
func buildTailerKey(obj tailerKey) string {
	if obj.getSourceIdentifier() != "" {
		return fmt.Sprintf("%s/%s", obj.getPath(), obj.getSourceIdentifier())
	}
	return obj.getPath()
}
