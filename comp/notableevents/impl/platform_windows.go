// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package notableeventsimpl

import sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"

// newPlatformCollector preserves the existing in-process Windows collector.
func newPlatformCollector(outChan chan<- eventPayload, _ sysprobeconfig.Component) (*collector, error) {
	return newCollector(outChan)
}
