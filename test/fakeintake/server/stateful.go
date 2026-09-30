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
	// Dictionary entries and the service/tags carry-forward state are
	// stream-scoped: the client only re-sends a dict_entry_define, or a
	// non-zero Log.service/Log.tags, when the value changes, so a later batch
	// on the same stream can reference ids a prior batch defined.
	decodeState := newStatefulDecodeState()
	for {
		batch, err := stream.Recv()
		if err != nil {
			return nil
		}
		logs := decodeState.decodeLogs(batch.Data)
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

// statefulDecodeState carries stream-scoped decode state across batches on
// one StatefulStream: dictionary entries, and the service/tags "last value"
// a Log's delta fields carry forward from. Mirrors the reference decoder
// (resolve_delta_value / optional_dictionary_value in the foldspace server
// crate's lib/server/src/lib.rs), scaled down to the fields this fakeintake
// stub needs: dictionary strings and Log.service/Log.tags.
type statefulDecodeState struct {
	dict        map[uint64]string
	lastService uint64
	lastTags    uint64
}

func newStatefulDecodeState() *statefulDecodeState {
	return &statefulDecodeState{dict: make(map[uint64]string)}
}

// resolveDelta mirrors resolve_delta_value: 0 means "reuse the last resolved
// id"; any other value both selects and becomes the new last.
func resolveDelta(last *uint64, value uint64) uint64 {
	if value == 0 {
		return *last
	}
	*last = value
	return value
}

// dictValue mirrors optional_dictionary_value: ids 0 and 1 both mean "absent"
// (0 is reserved for delta encoding, 1 for explicit absence).
func (s *statefulDecodeState) dictValue(id uint64) string {
	if id == 0 || id == 1 {
		return ""
	}
	return s.dict[id]
}

// decodeLogs decodes one batch's LogDatumSequence.data (field 1, repeated).
// Each LogDatum is a oneof; this stub only acts on dict_entry_define (field
// 1) and log (field 4) — the fields fakeintake's log aggregator surfaces.
// Pattern/JSON-schema datums are left undecoded, matching Log.raw_log being
// the only payload shape the current e2e coverage sends.
func (s *statefulDecodeState) decodeLogs(data []byte) []*aggregator.Log {
	var logs []*aggregator.Log
	rest := data
	for len(rest) > 0 {
		num, typ, n := protowire.ConsumeTag(rest)
		if n < 0 {
			return logs
		}
		rest = rest[n:]
		if typ != protowire.BytesType {
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
		switch num {
		case 1: // dict_entry_define
			s.decodeDictEntryDefine(datum)
		case 4: // log
			if l := s.decodeLog(datum); l != nil {
				logs = append(logs, l)
			}
		}
	}
	return logs
}

// decodeDictEntryDefine decodes a DictEntryDefine{id uint64 = 1, value string
// = 2} and stores it, so later Logs that reference id resolve to value.
func (s *statefulDecodeState) decodeDictEntryDefine(datum []byte) {
	var id uint64
	var value string
	rest := datum
	for len(rest) > 0 {
		num, typ, n := protowire.ConsumeTag(rest)
		if n < 0 {
			return
		}
		rest = rest[n:]
		switch {
		case num == 1 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(rest)
			if n < 0 {
				return
			}
			id = v
			rest = rest[n:]
		case num == 2 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(rest)
			if n < 0 {
				return
			}
			value = string(v)
			rest = rest[n:]
		default:
			skip := protowire.ConsumeFieldValue(num, typ, rest)
			if skip < 0 {
				return
			}
			rest = rest[skip:]
		}
	}
	// id 0 is reserved for delta encoding; a definition under it is malformed
	// and would otherwise poison dictValue's "absent" sentinel.
	if id != 0 {
		s.dict[id] = value
	}
}

// decodeLog decodes a Log message: service (field 3) and tags (field 4) are
// delta-encoded dictionary ids, resolved against this stream's carried-
// forward state; raw_log (field 7) is the message body. Pattern-encoded logs
// (pattern_id + dynamic_values instead of raw_log) are not decoded, matching
// the datum-level scope note above.
func (s *statefulDecodeState) decodeLog(msg []byte) *aggregator.Log {
	var serviceID, tagsID uint64
	var rawLog string
	rest := msg
	for len(rest) > 0 {
		num, typ, n := protowire.ConsumeTag(rest)
		if n < 0 {
			break
		}
		rest = rest[n:]
		switch {
		case num == 3 && typ == protowire.VarintType: // service
			v, n := protowire.ConsumeVarint(rest)
			if n < 0 {
				return nil
			}
			serviceID = v
			rest = rest[n:]
		case num == 4 && typ == protowire.VarintType: // tags
			v, n := protowire.ConsumeVarint(rest)
			if n < 0 {
				return nil
			}
			tagsID = v
			rest = rest[n:]
		case num == 7 && typ == protowire.BytesType: // raw_log
			v, n := protowire.ConsumeBytes(rest)
			if n < 0 {
				return nil
			}
			rawLog = string(v)
			rest = rest[n:]
		default:
			skip := protowire.ConsumeFieldValue(num, typ, rest)
			if skip < 0 {
				return nil
			}
			rest = rest[skip:]
		}
	}
	if strings.TrimSpace(rawLog) == "" {
		return nil
	}
	out := &aggregator.Log{
		Message: rawLog,
		Service: s.dictValue(resolveDelta(&s.lastService, serviceID)),
	}
	if tagsStr := s.dictValue(resolveDelta(&s.lastTags, tagsID)); tagsStr != "" {
		out.Tags = strings.Split(tagsStr, ",")
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
