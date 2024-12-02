// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package procutil

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func NewGpuProbe(
	config pkgconfigmodel.Reader,
) {
	nvml := nvml.New(nvml.WithLibraryPath(config.GetString("gpu_monitoring.nvml_lib_path")))
}
