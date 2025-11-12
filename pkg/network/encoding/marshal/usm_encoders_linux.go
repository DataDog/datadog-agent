// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf

package marshal

import (
	"github.com/DataDog/datadog-agent/pkg/network"
)

// InitializeUSMEncoders creates a slice of encoders that apply to the data in conns
func InitializeUSMEncoders(conns *network.Connections) []USMEncoder {
	encoders := make([]USMEncoder, 0)

	if encoder := newHTTPEncoder(conns.USMData.HTTP); encoder != nil {
		encoders = append(encoders, encoder)
	}
	if encoder := newHTTP2Encoder(conns.USMData.HTTP2); encoder != nil {
		encoders = append(encoders, encoder)
	}
	if encoder := newRedisEncoder(conns.USMData.Redis); encoder != nil {
		encoders = append(encoders, encoder)
	}
	if encoder := newKafkaEncoder(conns.USMData.Kafka); encoder != nil {
		encoders = append(encoders, encoder)
	}
	if encoder := newPostgresEncoder(conns.USMData.Postgres); encoder != nil {
		encoders = append(encoders, encoder)
	}

	return encoders
}
