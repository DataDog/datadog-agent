// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package request

import (
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
)

var (
	pSerializer = protoSerializer{}
	jSerializer = jsonSerializer{
		marshaler: protojson.MarshalOptions{
			EmitUnpopulated: true,
		},
	}
)

// Marshaler is an interface implemented by all process request serializers
type Marshaler interface {
	Marshal(r *pbgo.ProcessStatRequest) ([]byte, error)
	ContentType() string
}

// Unmarshaler is an interface implemented by all process request deserializers
type Unmarshaler interface {
	Unmarshal([]byte) (*pbgo.ProcessStatRequest, error)
}

// GetMarshaler returns the appropriate Marshaler based on the given accept header
func GetMarshaler(accept string) Marshaler {
	if strings.Contains(accept, ContentTypeProtobuf) {
		return pSerializer
	}

	return jSerializer
}

// GetUnmarshaler returns the appropriate Unmarshaler based on the given content type
func GetUnmarshaler(ctype string) Unmarshaler {
	if strings.Contains(ctype, ContentTypeProtobuf) {
		return pSerializer
	}

	return jSerializer
}
