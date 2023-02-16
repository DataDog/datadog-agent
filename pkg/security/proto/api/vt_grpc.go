// Copyright 2021 The Vitess Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// This implementation is based on
// https://github.com/vitessio/vitess/blob/main/go/vt/servenv/grpc_codec.go

package api

import (
	fmt "fmt"

	"google.golang.org/grpc/encoding"
	"google.golang.org/protobuf/proto"
)

const codecName = "proto"

// MaybeVTCodec represents a codec able to encode and decode vt enabled proto messages
type MaybeVTCodec struct{}

type vtprotoMessage interface {
	MarshalVT() ([]byte, error)
	UnmarshalVT([]byte) error
}

// Marshal encodes the protobuf message to a byte array
func (MaybeVTCodec) Marshal(v interface{}) ([]byte, error) {
	vt, ok := v.(vtprotoMessage)
	if ok {
		return vt.MarshalVT()
	}

	msg, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("failed to marshal, message is %T, want proto.Message", v)
	}
	return proto.Marshal(msg)
}

// Unmarshal decodes the byte array to the provided value
func (MaybeVTCodec) Unmarshal(data []byte, v interface{}) error {
	vt, ok := v.(vtprotoMessage)
	if ok {
		return vt.UnmarshalVT(data)
	}

	msg, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("failed to unmarshal, message is %T, want proto.Message", v)
	}
	return proto.Unmarshal(data, msg)
}

// Name returns the name of the codec
func (MaybeVTCodec) Name() string {
	return codecName
}

func RegisterVTCodec() {
	encoding.RegisterCodec(MaybeVTCodec{})
}
