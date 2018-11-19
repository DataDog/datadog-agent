// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build containerd

package containerd

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/metadata/host/container"
)

func init() {
	container.RegisterMetadataProvider("containerd", getMetadata)
}

func getMetadata() (map[string]string, error) {
	metadata := make(map[string]string)
	ctx := context.Background()

	cu := InstanciateContainerdUtil()
	defer cu.Close()
	err := cu.EnsureServing(ctx)
	if err != nil {
		return metadata, err
	}
	ver, err := cu.Metadata(ctx)
	if err != nil {
		return metadata, err
	}

	metadata["runtime"] = "containerd"
	metadata["version"] = ver.Version
	metadata["revision"] = ver.Revision
	return metadata, nil
}
