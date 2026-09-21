// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

const (
	statefulStreamFullMethod = "/datadog.intake.stateful.StatefulIntake/StatefulStream"
	statefulServiceName      = "datadog.intake.stateful.StatefulIntake"
)

type statefulBatch struct {
	BatchID uint32
	Data    []byte
}

type batchStatus struct {
	BatchID uint32
	Status  int32
}

type statefulCodec struct{}

func (statefulCodec) Name() string { return "foldspace" }

func (statefulCodec) Marshal(v any) ([]byte, error) {
	switch m := v.(type) {
	case *statefulBatch:
		buf := protowire.AppendTag(nil, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(m.BatchID))
		buf = protowire.AppendTag(buf, 2, protowire.BytesType)
		buf = protowire.AppendBytes(buf, m.Data)
		return buf, nil
	case *batchStatus:
		buf := protowire.AppendTag(nil, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(m.BatchID))
		buf = protowire.AppendTag(buf, 2, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(m.Status))
		return buf, nil
	default:
		return nil, status.Errorf(codes.Internal, "unsupported type %T", v)
	}
}

func (c statefulCodec) Unmarshal(data []byte, v any) error {
	switch m := v.(type) {
	case *statefulBatch:
		*m = statefulBatch{}
		for len(data) > 0 {
			num, typ, n := protowire.ConsumeTag(data)
			if n < 0 {
				return protowire.ParseError(n)
			}
			data = data[n:]
			switch {
			case num == 1 && typ == protowire.VarintType:
				val, n := protowire.ConsumeVarint(data)
				if n < 0 {
					return protowire.ParseError(n)
				}
				m.BatchID = uint32(val)
				data = data[n:]
			case num == 2 && typ == protowire.BytesType:
				val, n := protowire.ConsumeBytes(data)
				if n < 0 {
					return protowire.ParseError(n)
				}
				m.Data = append([]byte(nil), val...)
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
	case *batchStatus:
		*m = batchStatus{}
		return nil
	default:
		return status.Errorf(codes.Internal, "unsupported type %T", v)
	}
}

var _ encoding.Codec = statefulCodec{}

type statefulIntake struct {
	fi *Server
}

func (s statefulIntake) StatefulStream(stream grpc.BidiStreamingServer[statefulBatch, batchStatus]) error {
	apiKey := ""
	if md, ok := metadata.FromIncomingContext(stream.Context()); ok {
		if vals := md.Get("dd-api-key"); len(vals) > 0 {
			apiKey = vals[0]
		}
	}
	for {
		batch, err := stream.Recv()
		if err != nil {
			return nil
		}
		logs := decodeStatefulLogs(batch.Data)
		if len(logs) == 0 {
			logs = []*aggregator.Log{{Message: string(batch.Data), Service: "foldspace"}}
		}
		payload, err := json.Marshal(logs)
		if err != nil {
			log.Printf("fakeintake stateful: marshal logs: %v", err)
			continue
		}
		if err := s.fi.store.AppendPayload("/api/v2/logs", apiKey, payload, "identity", "application/json", time.Now()); err != nil {
			log.Printf("fakeintake stateful: store: %v", err)
		}
		if err := stream.Send(&batchStatus{BatchID: batch.BatchID, Status: 1}); err != nil {
			return err
		}
	}
}

func decodeStatefulLogs(data []byte) []*aggregator.Log {
	// LogDatumSequence.data is field 1, repeated. Each LogDatum is a oneof;
	// Log is field 4, and Log.raw_log is field 7.
	var logs []*aggregator.Log
	rest := data
	for len(rest) > 0 {
		num, typ, n := protowire.ConsumeTag(rest)
		if n < 0 {
			return logs
		}
		rest = rest[n:]
		if num != 1 || typ != protowire.BytesType {
			skip := protowire.ConsumeFieldValue(num, typ, rest)
			if skip < 0 {
				return logs
			}
			rest = rest[skip:]
			continue
		}
		datum, n := protowire.ConsumeBytes(rest)
		if n < 0 {
			return logs
		}
		rest = rest[n:]
		if l := decodeLogDatum(datum); l != nil {
			logs = append(logs, l)
		}
	}
	return logs
}

func decodeLogDatum(datum []byte) *aggregator.Log {
	rest := datum
	for len(rest) > 0 {
		num, typ, n := protowire.ConsumeTag(rest)
		if n < 0 {
			return nil
		}
		rest = rest[n:]
		if num == 4 && typ == protowire.BytesType {
			logMsg, n := protowire.ConsumeBytes(rest)
			if n < 0 {
				return nil
			}
			return decodeLog(logMsg)
		}
		skip := protowire.ConsumeFieldValue(num, typ, rest)
		if skip < 0 {
			return nil
		}
		rest = rest[skip:]
	}
	return nil
}

func decodeLog(msg []byte) *aggregator.Log {
	out := &aggregator.Log{}
	rest := msg
	for len(rest) > 0 {
		num, typ, n := protowire.ConsumeTag(rest)
		if n < 0 {
			break
		}
		rest = rest[n:]
		if num == 7 && typ == protowire.BytesType {
			val, n := protowire.ConsumeBytes(rest)
			if n < 0 {
				break
			}
			out.Message = string(val)
			rest = rest[n:]
			continue
		}
		skip := protowire.ConsumeFieldValue(num, typ, rest)
		if skip < 0 {
			break
		}
		rest = rest[skip:]
	}
	if strings.TrimSpace(out.Message) == "" {
		return nil
	}
	return out
}

func newStatefulGRPC(fi *Server) *grpc.Server {
	s := grpc.NewServer(grpc.ForceServerCodec(statefulCodec{}))
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: statefulServiceName,
		HandlerType: (*statefulIntakeServer)(nil),
		Streams: []grpc.StreamDesc{{
			StreamName: "StatefulStream",
			Handler: func(srv any, stream grpc.ServerStream) error {
				return srv.(statefulIntakeServer).StatefulStream(&grpc.GenericServerStream[statefulBatch, batchStatus]{ServerStream: stream})
			},
			ServerStreams: true,
			ClientStreams: true,
		}},
	}, statefulIntake{fi: fi})
	return s
}

type statefulIntakeServer interface {
	StatefulStream(grpc.BidiStreamingServer[statefulBatch, batchStatus]) error
}
