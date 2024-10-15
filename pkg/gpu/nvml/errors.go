// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nvml

import (
	"errors"

	nvidiaNvml "github.com/NVIDIA/go-nvml/pkg/nvml"
)

func wrapError(ret nvidiaNvml.Return) error {
	if ret == nvidiaNvml.SUCCESS {
		return nil
	}

	return errors.New(nvidiaNvml.ErrorString(ret))
}
