// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !docker

package flare

func zipDockerSelfInspect(tempDir, hostname string) error {
	return nil
}

func zipDockerPs(tempDir, hostname string) error {
	return nil
}
