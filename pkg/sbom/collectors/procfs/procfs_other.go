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

func getPath(_ sbom.ScanRequest) (string, error) {
	return "", errors.New("not supported")
}
