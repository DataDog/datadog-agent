// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && !linux

package procfs

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/sbom"
)

func (c *Collector) getPath(_ sbom.ScanRequest) (string, error) {
	return "", errors.New("not supported")
}

// IsAgentContainer returns whether the container ID is the agent one
func IsAgentContainer(ctrID string) (bool, error) {
	return false, errors.New("not supported")
}
