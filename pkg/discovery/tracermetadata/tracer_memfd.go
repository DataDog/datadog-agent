// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package tracermetadata

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const memfdTracerFileName = "datadog-tracer-info-"
const memFdTracerMaxSize = 1 << 16

// GetTracerMetadata parses the tracer-generated metadata
// according to
// https://docs.google.com/document/d/1kcW6BLdYxXeTSUz31cBqoqfW1Jjs0IDljfKeUfIRQp4/
func GetTracerMetadata(pid int, procRoot string) (TracerMetadata, error) {
	data, err := kernel.GetProcessMemFdFile(
		pid,
		procRoot,
		memfdTracerFileName,
		memFdTracerMaxSize,
	)
	if err != nil {
		return TracerMetadata{}, err
	}
	var trMeta TracerMetadata
	if _, err := trMeta.UnmarshalMsg(data); err != nil {
		return TracerMetadata{}, fmt.Errorf("error parsing tracer metadata: %s", err)
	}
	return trMeta, nil
}
