// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cri

package cri

import (
	"github.com/DataDog/datadog-agent/pkg/metadata/host/container"
)

func init() {
	container.RegisterMetadataProvider("cri", getMetadata)
}

func getMetadata() (map[string]string, error) {
	metadata := make(map[string]string)
	cu, err := GetUtil()
	if err != nil {
		return metadata, err
	}
	metadata["cri_name"] = cu.GetRuntime()
	metadata["cri_version"] = cu.GetRuntimeVersion()

	return metadata, nil
}
