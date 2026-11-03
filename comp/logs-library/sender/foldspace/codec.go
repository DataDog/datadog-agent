// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package foldspace

import (
	"fmt"

	"google.golang.org/grpc/encoding"
	"google.golang.org/protobuf/encoding/protowire"
)

// StatefulBatch is the transport message written on StatefulStream.
type StatefulBatch struct {
	BatchID uint32
	Data    []byte
}

// BatchStatus is the transport acknowledgement.
type BatchStatus struct {
	BatchID uint32
	Status  int32
}

const (
	statefulStreamFullMethod = "/datadog.intake.stateful.StatefulIntake/StatefulStream"
	statefulServiceName      = "datadog.intake.stateful.StatefulIntake"
	statefulStreamName       = "StatefulStream"
)

type statefulCodec struct{}

func (statefulCodec) Name() string { return "foldspace" }

func init() {
	encoding.RegisterCodec(statefulCodec{})
}

func (statefulCodec) Marshal(v any) ([]byte, error) {
	switch m := v.(type) {
	case *StatefulBatch:
		return marshalBatch(m), nil
	case *BatchStatus:
		return marshalStatus(m), nil
	default:
		return nil, fmt.Errorf("foldspace codec: unsupported type %T", v)
	}
}

func (statefulCodec) Unmarshal(data []byte, v any) error {
	switch m := v.(type) {
	case *StatefulBatch:
		return unmarshalBatch(data, m)
	case *BatchStatus:
		return unmarshalStatus(data, m)
	default:
		return fmt.Errorf("foldspace codec: unsupported type %T", v)
	}
}

func marshalBatch(b *StatefulBatch) []byte {
	buf := protowire.AppendTag(nil, 1, protowire.VarintType)
	buf = protowire.AppendVarint(buf, uint64(b.BatchID))
	buf = protowire.AppendTag(buf, 2, protowire.BytesType)
	buf = protowire.AppendBytes(buf, b.Data)
	return buf
}

func marshalStatus(s *BatchStatus) []byte {
	buf := protowire.AppendTag(nil, 1, protowire.VarintType)
	buf = protowire.AppendVarint(buf, uint64(s.BatchID))
	buf = protowire.AppendTag(buf, 2, protowire.VarintType)
	buf = protowire.AppendVarint(buf, uint64(s.Status))
	return buf
}

func unmarshalBatch(data []byte, out *StatefulBatch) error {
	*out = StatefulBatch{}
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return protowire.ParseError(n)
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return protowire.ParseError(n)
			}
			out.BatchID = uint32(v)
			data = data[n:]
		case num == 2 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return protowire.ParseError(n)
			}
			out.Data = append([]byte(nil), v...)
			data = data[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return protowire.ParseError(n)
			}
			data = data[n:]
		}
	}
	return nil
}

func unmarshalStatus(data []byte, out *BatchStatus) error {
	*out = BatchStatus{}
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return protowire.ParseError(n)
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return protowire.ParseError(n)
			}
			out.BatchID = uint32(v)
			data = data[n:]
		case num == 2 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return protowire.ParseError(n)
			}
			out.Status = int32(v)
			data = data[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return protowire.ParseError(n)
			}
			data = data[n:]
		}
	}
	return nil
}
