// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/DataDog/fastjq"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottllog"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"
	"github.com/valyala/fastjson"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

// BenchmarkOTTLFiltering compares the complete parse-and-evaluate path with the
// same input and nonmatching predicate. A message survives every rule, so larger
// rule counts measure shared parsing rather than early-exit behavior. Setup and
// rule compilation are excluded. fastjq is Ralph's pinned benchmark version.
func BenchmarkOTTLFiltering(b *testing.B) {
	ctx := context.Background()
	condition, err := compileOTTLCondition(`attributes["level"] == "debug"`)
	if err != nil {
		b.Fatal(err)
	}
	parser, err := ottllog.NewParser(ottlfuncs.StandardFuncs[*ottllog.TransformContext](), component.TelemetrySettings{Logger: zap.NewNop()})
	if err != nil {
		b.Fatal(err)
	}
	reference, err := parser.ParseCondition(`attributes["level"] == "debug"`)
	if err != nil {
		b.Fatal(err)
	}
	jq, err := fastjq.Compile(`select(.level == "debug")`)
	if err != nil {
		b.Fatal(err)
	}
	regex := regexp.MustCompile(`"level"\s*:\s*"debug"`)

	for _, size := range []int{256, 2048, 16384} {
		// Keep the selected field after padding so byte scanning and full parsing
		// both encounter the payload, rather than benchmarking an early field only.
		input := []byte(fmt.Sprintf(`{"message":"%s","level":"info","service":"web","status":200}`, strings.Repeat("x", size-64)))
		for _, rules := range []int{1, 5, 20} {
			name := fmt.Sprintf("bytes=%d/rules=%d", len(input), rules)
			b.Run(name+"/regex", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(input)))
				for b.Loop() {
					for range rules {
						if regex.Match(input) {
							b.Fatal("unexpected match")
						}
					}
				}
			})
			b.Run(name+"/fastjq", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(input)))
				for b.Loop() {
					for range rules {
						if err := jq.RunFunc(input, func([]byte) error { b.Fatal("unexpected match"); return nil }); err != nil {
							b.Fatal(err)
						}
					}
				}
			})
			b.Run(name+"/fastjq_reused_buffer", func(b *testing.B) {
				buffer := make([]byte, 0, len(input))
				b.ReportAllocs()
				b.SetBytes(int64(len(input)))
				for b.Loop() {
					for range rules {
						result, err := jq.RunWithBuffer(input, buffer)
						if err != nil || len(result) != 0 {
							b.Fatalf("unexpected result=%s err=%v", result, err)
						}
					}
				}
			})
			b.Run(name+"/ottl_original", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(input)))
				for b.Loop() {
					for range rules {
						c, err := benchmarkOriginalOTTLContext(input)
						if err != nil {
							b.Fatal(err)
						}
						match, err := reference.Eval(ctx, c)
						c.Close()
						if err != nil || match {
							b.Fatalf("match=%v err=%v", match, err)
						}
					}
				}
			})
			b.Run(name+"/ottl_shared_pdata", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(input)))
				for b.Loop() {
					c, err := benchmarkOriginalOTTLContext(input)
					if err != nil {
						b.Fatal(err)
					}
					for range rules {
						match, err := reference.Eval(ctx, c)
						if err != nil || match {
							b.Fatalf("match=%v err=%v", match, err)
						}
					}
					c.Close()
				}
			})
			b.Run(name+"/ottl_agent", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(input)))
				for b.Loop() {
					c := AcquireOTTLContext(input)
					for range rules {
						match, err := condition.Eval(ctx, c)
						if err != nil || match {
							b.Fatalf("match=%v err=%v", match, err)
						}
					}
					ReleaseOTTLContext(c)
				}
			})
		}
	}
}

// Mirror the original prototype's JSON decode and top-level scalar projection.
// Logging is omitted so this measures a lower bound for its successful path.
func benchmarkOriginalOTTLContext(content []byte) (*ottllog.TransformContext, error) {
	var fields map[string]any
	if err := json.Unmarshal(content, &fields); err != nil {
		return nil, err
	}
	record := plog.NewLogRecord()
	for key, value := range fields {
		switch v := value.(type) {
		case string:
			record.Attributes().PutStr(key, v)
		case bool:
			record.Attributes().PutBool(key, v)
		case float64:
			record.Attributes().PutDouble(key, v)
		}
	}
	resource := plog.NewResourceLogs()
	scope := resource.ScopeLogs().AppendEmpty()
	return ottllog.NewTransformContext(resource, scope, record), nil
}

// BenchmarkOTTLValidation separates strict validation from arena parsing so
// large-log regressions can be attributed to input preparation, not the DSL.
func BenchmarkOTTLValidation(b *testing.B) {
	for _, size := range []int{256, 2048, 16384} {
		input := []byte(fmt.Sprintf(`{"message":"%s","level":"info"}`, strings.Repeat("x", size)))
		for name, validate := range map[string]func([]byte) bool{
			"encoding_json": json.Valid,
			"fastjson":      func(b []byte) bool { return fastjson.ValidateBytes(b) == nil },
			"jsontext":      func(b []byte) bool { return jsontext.Value(b).IsValid(jsontext.AllowDuplicateNames(true)) },
		} {
			b.Run(fmt.Sprintf("bytes=%d/%s", len(input), name), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(input)))
				for b.Loop() {
					if !validate(input) {
						b.Fatal("invalid input")
					}
				}
			})
		}
		b.Run(fmt.Sprintf("bytes=%d/parse_only", len(input)), func(b *testing.B) {
			var parser fastjson.Parser
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			for b.Loop() {
				if _, err := parser.ParseBytes(input); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkOTTLMasking includes parsing, all field writes, and final encoding.
// Each rule edits a different field, so the whole document need only be encoded
// once. Compilation and constructing the input are outside the timed loop.
func BenchmarkOTTLMasking(b *testing.B) {
	for _, count := range []int{1, 5, 20} {
		b.Run(fmt.Sprintf("rules=%d", count), func(b *testing.B) {
			var input strings.Builder
			input.WriteString(`{"padding":"` + strings.Repeat("x", 2048) + `"`)
			statements := make([]*OTTLStatement, count)
			for i := range statements {
				fmt.Fprintf(&input, `,"field%d":"secret"`, i)
				var err error
				statements[i], err = compileOTTLMaskStatement(fmt.Sprintf(`set(attributes["field%d"], "***")`, i))
				if err != nil {
					b.Fatal(err)
				}
			}
			input.WriteByte('}')
			content := []byte(input.String())
			b.ReportAllocs()
			b.SetBytes(int64(len(content)))
			b.ResetTimer()
			for b.Loop() {
				c := AcquireOTTLContext(content)
				for _, statement := range statements {
					changed, err := c.ApplyMask(context.Background(), statement)
					if err != nil || !changed {
						b.Fatalf("changed=%v err=%v", changed, err)
					}
				}
				_ = c.Content()
				ReleaseOTTLContext(c)
			}
		})
	}
}
