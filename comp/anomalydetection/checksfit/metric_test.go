// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checksfit

import (
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"testing"
)

// The golden payloads were produced by the Rust `datadog-checks-protocol` crate
// (Message::Metric(..).encode_payload()) on the saluki branch
// `celian/innov-isolated-ipc`. They pin Go/Rust wire parity for the DDCHECKS v1
// metric payload; regenerate them there if the contract changes.
const (
	goldenFullHex = "01000000150000006c6f672e7061747465726e2e6162632e636f756e7400" +
		"0000000040454000f153650000000002000000140000006f627365727665725f" +
		"736f757263653a6c6f677310000000736572766963653a636865636b6f757405" +
		"0000007765622d310000000000000000"
	goldenMinimalHex = "03000000150000006c6f672e6669656c642e6a736f6e2e6e756d626572" +
		"000000000000f8bf070000000000000000000000000000000f00000000000000"
)

func goldenFull() Metric {
	return Metric{
		MetricType: MetricTypeCounter,
		Name:       "log.pattern.abc.count",
		Value:      42.5,
		Timestamp:  1_700_000_000,
		Tags:       []string{"observer_source:logs", "service:checkout"},
		Hostname:   "web-1",
	}
}

func goldenMinimal() Metric {
	return Metric{
		MetricType:   MetricTypeGauge,
		Name:         "log.field.json.number",
		Value:        -1.5,
		Timestamp:    7,
		IntervalSecs: 15,
	}
}

func TestMetricEncodingMatchesRustGoldenBytes(t *testing.T) {
	tests := []struct {
		name   string
		metric Metric
		want   string
	}{
		{name: "full", metric: goldenFull(), want: goldenFullHex},
		{name: "minimal", metric: goldenMinimal(), want: goldenMinimalHex},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.metric.Encode()
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if hex.EncodeToString(got) != test.want {
				t.Fatalf("encoded bytes differ from the Rust golden payload\n got %s\nwant %s",
					hex.EncodeToString(got), test.want)
			}
			if size, err := test.metric.EncodedLen(); err != nil || size != len(got) {
				t.Fatalf("EncodedLen = %d, %v; want %d, nil", size, err, len(got))
			}
		})
	}
}

func TestMetricDecodeRoundTripsGoldenBytes(t *testing.T) {
	tests := []struct {
		name   string
		metric Metric
		want   string
	}{
		{name: "full", metric: goldenFull(), want: goldenFullHex},
		{name: "minimal", metric: goldenMinimal(), want: goldenMinimalHex},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, err := hex.DecodeString(test.want)
			if err != nil {
				t.Fatalf("decode golden hex: %v", err)
			}
			got, err := DecodeMetric(payload)
			if err != nil {
				t.Fatalf("DecodeMetric: %v", err)
			}
			if !metricsEqual(got, test.metric) {
				t.Fatalf("decoded metric = %+v, want %+v", got, test.metric)
			}
		})
	}
}

func metricsEqual(a, b Metric) bool {
	if a.MetricType != b.MetricType || a.Name != b.Name || a.Value != b.Value ||
		a.Timestamp != b.Timestamp || a.Hostname != b.Hostname || a.IntervalSecs != b.IntervalSecs {
		return false
	}
	if len(a.Tags) != len(b.Tags) {
		return false
	}
	for index := range a.Tags {
		if a.Tags[index] != b.Tags[index] {
			return false
		}
	}
	return true
}

func TestDecodeMetricRejectsMalformedPayloads(t *testing.T) {
	full, err := hex.DecodeString(goldenFullHex)
	if err != nil {
		t.Fatalf("decode golden hex: %v", err)
	}

	tests := []struct {
		name    string
		payload []byte
		wantErr error
	}{
		{name: "empty", payload: nil, wantErr: errTruncated},
		{name: "truncated name", payload: full[:8], wantErr: errTruncated},
		{name: "trailing bytes", payload: append(append([]byte{}, full...), 0), wantErr: errTrailingBytes},
		{name: "name length beyond payload", payload: mutate(full, func(b []byte) {
			b[4] = 0xff
		}), wantErr: errTruncated},
		{name: "tag count beyond payload", payload: mutate(full, func(b []byte) {
			// The tag count sits right after metric_type, name, value, and timestamp.
			offset := 4 + 4 + len("log.pattern.abc.count") + 8 + 8
			b[offset] = 0x7f
		}), wantErr: errTruncated},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeMetric(test.payload)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("DecodeMetric error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func mutate(payload []byte, change func([]byte)) []byte {
	out := append([]byte{}, payload...)
	change(out)
	return out
}

func TestDecodeMetricRejectsInvalidUTF8(t *testing.T) {
	metric := goldenMinimal()
	payload, err := metric.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	payload[8] = 0xff // first byte of the metric name
	if _, err := DecodeMetric(payload); !errors.Is(err, errInvalidUTF8) {
		t.Fatalf("DecodeMetric error = %v, want %v", err, errInvalidUTF8)
	}
}

func TestEncodedLenRejectsOversizedPaylods(t *testing.T) {
	restore := lowerPayloadLimit(t, 64)
	defer restore()

	metric := Metric{Name: strings.Repeat("a", 128)}
	if _, err := metric.EncodedLen(); !errors.Is(err, errPayloadTooLarge) {
		t.Fatalf("EncodedLen error = %v, want %v", err, errPayloadTooLarge)
	}
	if _, err := metric.Encode(); !errors.Is(err, errPayloadTooLarge) {
		t.Fatalf("Encode error = %v, want %v", err, errPayloadTooLarge)
	}
	// A metric that fits under the lowered limit still encodes.
	if _, err := (Metric{Name: "ok.count", Value: 1}).Encode(); err != nil {
		t.Fatalf("Encode small metric: %v", err)
	}
}

// lowerPayloadLimit shrinks the payload limit for the duration of a test.
func lowerPayloadLimit(t *testing.T, limit int) (restore func()) {
	t.Helper()
	previous := payloadLimit
	payloadLimit = limit
	return func() { payloadLimit = previous }
}

func TestMetricEncodesZeroNameAndEmptyTags(t *testing.T) {
	payload, err := Metric{MetricType: MetricTypeCounter, Value: math.Inf(1)}.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := DecodeMetric(payload)
	if err != nil {
		t.Fatalf("DecodeMetric: %v", err)
	}
	if !math.IsInf(got.Value, 1) || got.Name != "" || len(got.Tags) != 0 || got.Hostname != "" {
		t.Fatalf("round trip = %+v, want infinite value with empty name, tags, and hostname", got)
	}
}

func TestDescriptorMatchesRustContract(t *testing.T) {
	if got := string(Descriptor.ID[:]); got != "DDCHECKS" {
		t.Fatalf("descriptor identity = %q, want %q", got, "DDCHECKS")
	}
	if Descriptor.Version != 1 {
		t.Fatalf("descriptor version = %d, want 1", Descriptor.Version)
	}
	want := []uint32{TypeMetric, TypeLog, TypeServiceCheck, TypeEvent}
	if len(Descriptor.MessageTypes) != len(want) {
		t.Fatalf("descriptor message types = %v, want %v", Descriptor.MessageTypes, want)
	}
	for index, id := range want {
		if Descriptor.MessageTypes[index] != id {
			t.Fatalf("descriptor message types = %v, want %v", Descriptor.MessageTypes, want)
		}
	}
}
