// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package unmarshal implements the unmarshalling side of network encoding
package unmarshal

import (
	"strings"

	model "github.com/DataDog/agent-payload/v5/process"
)

var (
	pSerializer Unmarshaler = protoSerializer{}
	jSerializer Unmarshaler = jsonSerializer{}
)

// Unmarshaler is an interface implemented by all Connections deserializers
type Unmarshaler interface {
	Unmarshal([]byte) (*model.Connections, error)
	ContentType() string
}

// GetUnmarshaler returns the appropriate Unmarshaler based on the given content type
func GetUnmarshaler(ctype string) Unmarshaler {
	if strings.Contains(ctype, ContentTypeProtobuf) {
		return pSerializer
	}
	return jSerializer
}
