// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ottl

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	goccyjson "github.com/goccy/go-json"
	upstreamottl "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// The pinned OTTL fork reuses encoding/json in all build paths. Compare its
// observable result with the encoder used by unmodified OTTL. Small negative
// exponents use the stdlib spelling (1e-9 instead of goccy's 1e-09).
func TestOTTLStringJSONCompatibility(t *testing.T) {
	object := map[string]any{
		"z":       nil,
		"id":      int64(9007199254740993),
		"nested":  []any{true, 1.5, map[string]any{"text": "<>&\u2028\u2029"}},
		"escaped": "quote\" backslash\\ newline\n tab\t",
		"unicode": "café 日本語 😀",
	}
	pdataMap := pcommon.NewMap()
	require.NoError(t, pdataMap.FromRaw(object))
	pdataSlice := pcommon.NewSlice()
	require.NoError(t, pdataSlice.FromRaw([]any{object, false, nil}))
	nanMap := pcommon.NewMap()
	nanMap.PutDouble("value", math.NaN())

	for _, test := range []struct {
		name       string
		value, raw any
	}{
		{"object ordering and escaping", object, object},
		{"pdata map", pdataMap, pdataMap.AsRaw()},
		{"pdata slice", pdataSlice, pdataSlice.AsRaw()},
		{"pdata invalid number", nanMap, nanMap.AsRaw()},
		{"array", []any{object, nil, false, "text"}, []any{object, nil, false, "text"}},
		{"empty object", map[string]any{}, map[string]any{}},
		{"empty array", []any{}, []any{}},
		{"nil object", (map[string]any)(nil), (map[string]any)(nil)},
		{"nil array", ([]any)(nil), ([]any)(nil)},
		{"integer precision", int64(9007199254740993), int64(9007199254740993)},
		{"int64 maximum", int64(math.MaxInt64), int64(math.MaxInt64)},
		{"int64 minimum", int64(math.MinInt64), int64(math.MinInt64)},
		{"uint64 maximum", uint64(math.MaxUint64), uint64(math.MaxUint64)},
		{"boolean", true, true},
		{"negative zero", math.Copysign(0, -1), math.Copysign(0, -1)},
		{"decimal boundary", 1e-6, 1e-6},
		{"small exponent", 1e-9, 1e-9},
		{"large decimal", 1e20, 1e20},
		{"large exponent", 1e21, 1e21},
		{"largest float", math.MaxFloat64, math.MaxFloat64},
		{"smallest float", math.SmallestNonzeroFloat64, math.SmallestNonzeroFloat64},
		{"float32", float32(1.2345), float32(1.2345)},
		{"invalid UTF8", map[string]any{"value": string([]byte{0xff, 0xfe})}, map[string]any{"value": string([]byte{0xff, 0xfe})}},
	} {
		t.Run(test.name, func(t *testing.T) {
			expected, expectedErr := goccyjson.Marshal(test.raw)
			if test.name == "small exponent" {
				require.Equal(t, "1e-09", string(expected))
				expected = []byte("1e-9")
			}
			getter := upstreamottl.StandardStringLikeGetter[*OTTLContext]{
				Getter: func(context.Context, *OTTLContext) (any, error) { return test.value, nil },
			}
			actual, ok, err := getter.Get(context.Background(), nil)
			if expectedErr != nil {
				require.EqualError(t, err, expectedErr.Error())
				require.False(t, ok)
				return
			}
			require.NoError(t, err)
			require.True(t, ok)
			require.Equal(t, string(expected), actual)
		})
	}
}

// Include rule compilation, JSON field materialization, String conversion, and
// the body edit so compatibility isn't checked only against typed getters.
func TestOTTLStringJSONMask(t *testing.T) {
	input := `{"value":{"z":true,"id":9007199254740993,"message":"<>&\u2028\u2029","items":[1,null,"café"]}}`
	statement, err := compileOTTLMaskStatement(`set(body, String(attributes["value"]))`)
	require.NoError(t, err)
	c := AcquireOTTLContext([]byte(input))
	defer ReleaseOTTLContext(c)
	_, err = c.ApplyMask(context.Background(), statement)
	require.NoError(t, err)
	require.Equal(t, `{"id":9007199254740993,"items":[1,null,"café"],"message":"\u003c\u003e\u0026\u2028\u2029","z":true}`, string(c.Content()))
}

func BenchmarkOTTLStringJSONEncoding(b *testing.B) {
	for name, value := range map[string]any{
		"integer":      int64(9007199254740993),
		"small object": map[string]any{"service": "web", "status": int64(200), "ok": true},
		"2KiB object":  map[string]any{"message": strings.Repeat("x", 2048), "nested": map[string]any{"status": int64(200)}},
	} {
		for encoder, encode := range map[string]func(any) ([]byte, error){"stdlib": json.Marshal, "goccy": goccyjson.Marshal} {
			b.Run(name+"/"+encoder, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := encode(value); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
