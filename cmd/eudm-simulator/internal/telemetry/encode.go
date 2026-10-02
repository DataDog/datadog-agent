// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"bytes"
	"encoding/json"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
)

// Encode preserves protobuf oneof values as well as the ordinary typed sample
// fields. encoding/json cannot decode the generated oneof interface members.
func Encode(value any) ([]byte, error) {
	switch value := value.(type) {
	case *model.CollectorProc:
		return encodeProto(value)
	case *model.CollectorConnections:
		return encodeProto(value)
	default:
		return json.Marshal(value)
	}
}

func encodeProto(value proto.Message) ([]byte, error) {
	var data bytes.Buffer
	encoder := jsonpb.Marshaler{OrigName: true, EnumsAsInts: true}
	if err := encoder.Marshal(&data, value); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

func decodeProto(data []byte, value proto.Message) error {
	// Apply the common null/trailing-document checks before strict protobuf
	// decoding. jsonpb rejects unknown fields unless explicitly configured to
	// ignore them; capture bundles never enable that option.
	var document json.RawMessage
	if err := decode(data, &document); err != nil {
		return err
	}
	return jsonpb.Unmarshal(bytes.NewReader(document), value)
}
