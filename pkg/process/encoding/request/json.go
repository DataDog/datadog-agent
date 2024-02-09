// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package request

import (
	"google.golang.org/protobuf/encoding/protojson"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
)

// ContentTypeJSON holds the HTML content-type of a JSON payload
const ContentTypeJSON = "application/json"

type jsonSerializer struct {
	marshaler protojson.MarshalOptions
}

// Marshal returns the json encoding of the ProcessStatRequest
func (j jsonSerializer) Marshal(r *pbgo.ProcessStatRequest) ([]byte, error) {
	return j.marshaler.Marshal(r)
}

// Unmarshal parses the JSON-encoded ProcessStatRequest
func (jsonSerializer) Unmarshal(blob []byte) (*pbgo.ProcessStatRequest, error) {
	req := new(pbgo.ProcessStatRequest)
	if err := protojson.Unmarshal(blob, req); err != nil {
		return nil, err
	}
	return req, nil
}

func (j jsonSerializer) ContentType() string {
	return ContentTypeJSON
}

var _ Marshaler = jsonSerializer{}
var _ Unmarshaler = jsonSerializer{}
