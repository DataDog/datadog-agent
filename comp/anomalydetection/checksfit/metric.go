// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checksfit

import (
	"encoding/binary"
	"errors"
	"math"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/checksfit/fitcore"
)

// maxPayload is the largest payload a FIT ring record can carry: the ring
// capacity minus its eight-byte record header and eight-byte gap.
const maxPayload = fitcore.MaxRingCapacity - 16

// payloadLimit is maxPayload. Tests lower it so size-limit paths are reachable
// without allocating gigabyte payloads.
var payloadLimit = maxPayload

func checkPayloadSize(size int) error {
	if size > payloadLimit {
		return errPayloadTooLarge
	}
	return nil
}

var (
	errPayloadTooLarge = errors.New("payload exceeds FIT maximum")
	errStringTooLong   = errors.New("string exceeds u32 length")
	errTooManyStrings  = errors.New("string count exceeds u32")
	errTruncated       = errors.New("payload is truncated")
	errTrailingBytes   = errors.New("payload has trailing bytes")
	errInvalidUTF8     = errors.New("string is not UTF-8")
)

// Metric is the type 1 payload: one scalar check metric sample, with the field
// mapping of `metric.proto`.
type Metric struct {
	// MetricType is the numeric MetricType, including unknown values.
	MetricType int32
	// Name is the metric name.
	Name string
	// Value is the scalar sample.
	Value float64
	// Timestamp is the sample's Unix timestamp in seconds.
	Timestamp uint64
	// Tags are the metric tags, in order.
	Tags []string
	// Hostname is the explicit host, or empty to use the receiver's default.
	Hostname string
	// IntervalSecs is the rate interval in seconds, or zero for other types.
	IntervalSecs uint64
}

// EncodedLen reports the exact payload length of the encoded metric.
func (m Metric) EncodedLen() (int, error) {
	nameSize, err := stringSize(m.Name)
	if err != nil {
		return 0, err
	}
	tagsSize, err := stringsSize(m.Tags)
	if err != nil {
		return 0, err
	}
	hostSize, err := stringSize(m.Hostname)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, part := range []int{4, nameSize, 8, 8, tagsSize, hostSize, 8} {
		total += part
		if err := checkPayloadSize(total); err != nil {
			return 0, err
		}
	}
	return total, nil
}

// Encode serializes the metric in the field order of the DDCHECKS v1 contract:
// `i32 metric_type`, `string name`, `f64 value`, `u64 timestamp`,
// `string[] tags`, `string hostname`, `u64 interval_secs`, all little-endian
// with u32 string lengths and u32 tag counts.
func (m Metric) Encode() ([]byte, error) {
	size, err := m.EncodedLen()
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, size)
	out = binary.LittleEndian.AppendUint32(out, uint32(m.MetricType))
	out = appendString(out, m.Name)
	out = binary.LittleEndian.AppendUint64(out, math.Float64bits(m.Value))
	out = binary.LittleEndian.AppendUint64(out, m.Timestamp)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(m.Tags)))
	for _, tag := range m.Tags {
		out = appendString(out, tag)
	}
	out = appendString(out, m.Hostname)
	out = binary.LittleEndian.AppendUint64(out, m.IntervalSecs)
	return out, nil
}

// DecodeMetric parses a type 1 payload, rejecting truncated payloads,
// mismatched lengths, trailing bytes, and invalid UTF-8.
func DecodeMetric(b []byte) (Metric, error) {
	reader := payloadReader{remaining: b}
	out := Metric{}
	metricType, err := reader.u32()
	if err != nil {
		return Metric{}, err
	}
	out.MetricType = int32(metricType)
	if out.Name, err = reader.string(); err != nil {
		return Metric{}, err
	}
	bits, err := reader.u64()
	if err != nil {
		return Metric{}, err
	}
	out.Value = math.Float64frombits(bits)
	if out.Timestamp, err = reader.u64(); err != nil {
		return Metric{}, err
	}
	count, err := reader.u32()
	if err != nil {
		return Metric{}, err
	}
	for index := uint32(0); index < count; index++ {
		tag, err := reader.string()
		if err != nil {
			return Metric{}, err
		}
		out.Tags = append(out.Tags, tag)
	}
	if out.Hostname, err = reader.string(); err != nil {
		return Metric{}, err
	}
	if out.IntervalSecs, err = reader.u64(); err != nil {
		return Metric{}, err
	}
	if len(reader.remaining) != 0 {
		return Metric{}, errTrailingBytes
	}
	return out, nil
}

func stringSize(value string) (int, error) {
	if uint64(len(value)) > math.MaxUint32 {
		return 0, errStringTooLong
	}
	size := 4 + len(value)
	if err := checkPayloadSize(size); err != nil {
		return 0, err
	}
	return size, nil
}

func stringsSize(values []string) (int, error) {
	if uint64(len(values)) > math.MaxUint32 {
		return 0, errTooManyStrings
	}
	total := 4
	for _, value := range values {
		size, err := stringSize(value)
		if err != nil {
			return 0, err
		}
		total += size
		if err := checkPayloadSize(total); err != nil {
			return 0, err
		}
	}
	return total, nil
}

func appendString(out []byte, value string) []byte {
	out = binary.LittleEndian.AppendUint32(out, uint32(len(value)))
	return append(out, value...)
}

// payloadReader reads the FIT payload primitives from a byte slice. The
// primitives are shared by the DDCHECKS records and the anomaly event records.
type payloadReader struct {
	remaining []byte
}

func (r *payloadReader) take(count int) ([]byte, error) {
	if count < 0 || count > len(r.remaining) {
		return nil, errTruncated
	}
	out := r.remaining[:count]
	r.remaining = r.remaining[count:]
	return out, nil
}

func (r *payloadReader) u32() (uint32, error) {
	bytes, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(bytes), nil
}

func (r *payloadReader) u64() (uint64, error) {
	bytes, err := r.take(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(bytes), nil
}

func (r *payloadReader) string() (string, error) {
	length, err := r.u32()
	if err != nil {
		return "", err
	}
	bytes, err := r.take(int(length))
	if err != nil {
		return "", err
	}
	if !utf8.Valid(bytes) {
		return "", errInvalidUTF8
	}
	return string(bytes), nil
}
