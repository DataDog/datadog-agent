// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

//+build !zstd

package jsonstream

import (
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

const (
	Available = false
)

func Payloads(m marshaler.StreamMarshaler) (forwarder.Payloads, error) {
	return nil, nil
}
