// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build linux windows darwin

package status

import "github.com/DataDog/gohai/platform"

func getPlatformPayload() (result interface{}, err error) {
	return new(platform.Platform).Collect()
}
