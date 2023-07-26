// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package request

import (
	"google.golang.org/protobuf/proto"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
)

// ContentTypeProtobuf holds the HTML content-type of a Protobuf payload
const ContentTypeProtobuf = "application/protobuf"

type protoSerializer struct{}

// Marshal returns the proto encoding of the ProcessStatRequest
func (protoSerializer) Marshal(r *pbgo.ProcessStatRequest) ([]byte, error) {
	buf, err := proto.Marshal(r)
	return buf, err
}

// Unmarshal parses the proto-encoded ProcessStatRequest
func (protoSerializer) Unmarshal(blob []byte) (*pbgo.ProcessStatRequest, error) {
	req := new(pbgo.ProcessStatRequest)
	if err := proto.Unmarshal(blob, req); err != nil {
		return nil, err
	}
	return req, nil
}

// ContentType returns ContentTypeProtobuf
func (p protoSerializer) ContentType() string {
	return ContentTypeProtobuf
}

var _ Marshaler = protoSerializer{}
var _ Unmarshaler = protoSerializer{}
