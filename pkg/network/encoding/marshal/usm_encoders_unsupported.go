// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !((linux && linux_bpf) || (windows && npm))

package marshal

import (
	"github.com/DataDog/datadog-agent/pkg/network"
)

// InitializeUSMEncoders creates a slice of encoders that apply to the data in conns
func InitializeUSMEncoders(*network.Connections) []USMEncoder {
	return nil
}
