// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows && npm

package marshal

import (
	"github.com/DataDog/datadog-agent/pkg/network"
)

func initializeUSMEncoders(conns *network.Connections) []usmEncoder {
	encoders := make([]usmEncoder, 0, 1)

	if encoder := newHTTPEncoder(conns.HTTP); encoder != nil {
		encoders = append(encoders, encoder)
	}

	return encoders
}
