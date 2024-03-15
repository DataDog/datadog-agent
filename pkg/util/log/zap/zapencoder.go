// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package log

import (
	"time"

	"go.uber.org/zap/zapcore"
)

var _ zapcore.ObjectEncoder = (*encoder)(nil)

// encoder is an ObjectEncoder backed by an array.
type encoder struct {
	prefix string
	ctx    []interface{}
}

func (e *encoder) fullKey(k string) string {
	return e.prefix + k
}

func (e *encoder) Clone() *encoder {
	var ctx []interface{}
	if e.ctx != nil {
		ctx = make([]interface{}, len(e.ctx))
		copy(ctx, e.ctx)
	}

	return &encoder{
		prefix: e.prefix,
		ctx:    ctx,
	}
}

func (e *encoder) AddArray(k string, v zapcore.ArrayMarshaler) error {
	enc := &sliceArrayEncoder{elems: make([]interface{}, 0)}
	err := v.MarshalLogArray(enc)
	e.ctx = append(e.ctx, e.fullKey(k), enc.elems)
	return err
}

func (e *encoder) AddObject(_ string, v zapcore.ObjectMarshaler) error { return v.MarshalLogObject(e) }
func (e *encoder) AddBinary(k string, v []byte)                        { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddByteString(k string, v []byte)                    { e.ctx = append(e.ctx, e.fullKey(k), string(v)) }
func (e *encoder) AddBool(k string, v bool)                            { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddDuration(k string, v time.Duration)               { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddComplex128(k string, v complex128)                { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddComplex64(k string, v complex64)                  { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddFloat64(k string, v float64)                      { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddFloat32(k string, v float32)                      { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddInt(k string, v int)                              { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddInt64(k string, v int64)                          { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddInt32(k string, v int32)                          { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddInt16(k string, v int16)                          { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddInt8(k string, v int8)                            { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddString(k string, v string)                        { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddTime(k string, v time.Time)                       { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddUint(k string, v uint)                            { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddUint64(k string, v uint64)                        { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddUint32(k string, v uint32)                        { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddUint16(k string, v uint16)                        { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddUint8(k string, v uint8)                          { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddUintptr(k string, v uintptr)                      { e.ctx = append(e.ctx, e.fullKey(k), v) }
func (e *encoder) AddReflected(k string, v interface{}) error {
	e.ctx = append(e.ctx, e.fullKey(k), v)
	return nil
}

func (e *encoder) OpenNamespace(ns string) {
	e.prefix = e.prefix + ns + "/"
}

var _ zapcore.ArrayEncoder = (*sliceArrayEncoder)(nil)

// sliceArrayEncoder is taken from
// https://github.com/uber-go/zap/blob/v1.19.0/zapcore/memory_encoder.go#L36
type sliceArrayEncoder struct {
	elems []interface{}
}

func (s *sliceArrayEncoder) AppendArray(v zapcore.ArrayMarshaler) error {
	enc := &sliceArrayEncoder{}
	err := v.MarshalLogArray(enc)
	s.elems = append(s.elems, enc.elems)
	return err
}
func (s *sliceArrayEncoder) AppendObject(v zapcore.ObjectMarshaler) error {
	m := zapcore.NewMapObjectEncoder()
	err := v.MarshalLogObject(m)
	s.elems = append(s.elems, m.Fields)
	return err
}
func (s *sliceArrayEncoder) AppendReflected(v interface{}) error {
	s.elems = append(s.elems, v)
	return nil
}
func (s *sliceArrayEncoder) AppendBool(v bool)              { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendByteString(v []byte)      { s.elems = append(s.elems, string(v)) }
func (s *sliceArrayEncoder) AppendComplex128(v complex128)  { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendComplex64(v complex64)    { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendDuration(v time.Duration) { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendFloat64(v float64)        { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendFloat32(v float32)        { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendInt(v int)                { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendInt64(v int64)            { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendInt32(v int32)            { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendInt16(v int16)            { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendInt8(v int8)              { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendString(v string)          { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendTime(v time.Time)         { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendUint(v uint)              { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendUint64(v uint64)          { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendUint32(v uint32)          { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendUint16(v uint16)          { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendUint8(v uint8)            { s.elems = append(s.elems, v) }
func (s *sliceArrayEncoder) AppendUintptr(v uintptr)        { s.elems = append(s.elems, v) }
