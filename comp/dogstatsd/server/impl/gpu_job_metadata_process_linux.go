// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package serverimpl

import "github.com/DataDog/datadog-agent/pkg/util/kernel"

func defaultGPUJobMetadataProcessExists(processID uint32) bool {
	return kernel.ProcessExists(int(processID))
}
