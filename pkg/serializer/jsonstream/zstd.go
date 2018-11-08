// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

//+build zstd

package jsonstream

import (
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

const (
	// Available is true if the code is compiled in
	Available = true
)

// the backend accepts payloads up to 3MB/50MB, but being conservative is okay
var maxPayloadSize = 2 * 1024 * 1024
var maxUncompressedSize = 40 * 1024 * 1024

// Payloads serializes a metadata payload and sends it to the forwarder
func Payloads(m marshaler.StreamMarshaler) (forwarder.Payloads, error) {
	return nil, nil
}
