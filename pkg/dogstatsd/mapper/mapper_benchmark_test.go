// Copyright 2013 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mapper

import (
	"fmt"
	"testing"
)

var (
	ruleTemplateSingleMatchGlob = `
- match: metric%d.*
  name: "metric_single"
  labels:
    name: "$1"
`
	ruleTemplateSingleMatchRegex = `
- match: metric%d\.([^.]*)
  name: "metric_single"
  labels:
    name: "$1"
`

	ruleTemplateMultipleMatchGlob = `
- match: metric%d.*.*.*.*.*.*.*.*.*.*.*.*
  name: "metric_multi"
  labels:
    name: "$1-$2-$3.$4-$5-$6.$7-$8-$9.$10-$11-$12"
`

	ruleTemplateMultipleMatchRegex = `
- match: metric%d\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)
  name: "metric_multi"
  labels:
    name: "$1-$2-$3.$4-$5-$6.$7-$8-$9.$10-$11-$12"
`
)

func duplicateRules(count int, template string) string {
	rules := ""
	for i := 0; i < count; i++ {
		rules += fmt.Sprintf(template, i)
	}
	return rules
}

func BenchmarkGlob(b *testing.B) {
	config := `---
mappings:
- match: test.dispatcher.*.*.succeeded
  name: "dispatch_events"
  labels:
    processor: "$1"
    action: "$2"
    result: "succeeded"
    job: "test_dispatcher"
- match: test.my-dispatch-host01.name.dispatcher.*.*.*
  name: "host_dispatch_events"
  labels:
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
- match: request_time.*.*.*.*.*.*.*.*.*.*.*.*
  name: "tyk_http_request"
  labels:
    method_and_path: "${1}"
    response_code: "${2}"
    apikey: "${3}"
    apiversion: "${4}"
    apiname: "${5}"
    apiid: "${6}"
    ipv4_t1: "${7}"
    ipv4_t2: "${8}"
    ipv4_t3: "${9}"
    ipv4_t4: "${10}"
    orgid: "${11}"
    oauthid: "${12}"
- match: "*.*"
  name: "catchall"
  labels:
    first: "$1"
    second: "$2"
    third: "$3"
    job: "-"
  `
	mappings := []string{
		"test.dispatcher.FooProcessor.send.succeeded",
		"test.my-dispatch-host01.name.dispatcher.FooProcessor.send.succeeded",
		"request_time.get/threads/1/posts.200.00000000.nonversioned.discussions.a11bbcdf0ac64ec243658dc64b7100fb.172.20.0.1.12ba97b7eaa1a50001000001.",
		"foo.bar",
		"foo.bar.baz",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkGlobNoOrdering(b *testing.B) {
	config := `---
defaults:
  glob_disable_ordering: true
mappings:
- match: test.dispatcher.*.*.succeeded
  name: "dispatch_events"
  labels:
    processor: "$1"
    action: "$2"
    result: "succeeded"
    job: "test_dispatcher"
- match: test.my-dispatch-host01.name.dispatcher.*.*.*
  name: "host_dispatch_events"
  labels:
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
- match: request_time.*.*.*.*.*.*.*.*.*.*.*.*
  name: "tyk_http_request"
  labels:
    method_and_path: "${1}"
    response_code: "${2}"
    apikey: "${3}"
    apiversion: "${4}"
    apiname: "${5}"
    apiid: "${6}"
    ipv4_t1: "${7}"
    ipv4_t2: "${8}"
    ipv4_t3: "${9}"
    ipv4_t4: "${10}"
    orgid: "${11}"
    oauthid: "${12}"
- match: "*.*"
  name: "catchall"
  labels:
    first: "$1"
    second: "$2"
    third: "$3"
    job: "-"
  `
	mappings := []string{
		"test.dispatcher.FooProcessor.send.succeeded",
		"test.my-dispatch-host01.name.dispatcher.FooProcessor.send.succeeded",
		"request_time.get/threads/1/posts.200.00000000.nonversioned.discussions.a11bbcdf0ac64ec243658dc64b7100fb.172.20.0.1.12ba97b7eaa1a50001000001.",
		"foo.bar",
		"foo.bar.baz",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkGlobNoOrderingWithBacktracking(b *testing.B) {
	config := `---
defaults:
  glob_disable_ordering: true
mappings:
- match: test.dispatcher.*.*.succeeded
  name: "dispatch_events"
  labels:
    processor: "$1"
    action: "$2"
    result: "succeeded"
    job: "test_dispatcher"
- match: test.dispatcher.*.received.*
  name: "dispatch_events_wont_match"
  labels:
    processor: "$1"
    action: "received"
    result: "$2"
    job: "test_dispatcher"
- match: test.my-dispatch-host01.name.dispatcher.*.*.*
  name: "host_dispatch_events"
  labels:
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
- match: request_time.*.*.*.*.*.*.*.*.*.*.*.*
  name: "tyk_http_request"
  labels:
    method_and_path: "${1}"
    response_code: "${2}"
    apikey: "${3}"
    apiversion: "${4}"
    apiname: "${5}"
    apiid: "${6}"
    ipv4_t1: "${7}"
    ipv4_t2: "${8}"
    ipv4_t3: "${9}"
    ipv4_t4: "${10}"
    orgid: "${11}"
    oauthid: "${12}"
- match: "*.*"
  name: "catchall"
  labels:
    first: "$1"
    second: "$2"
    third: "$3"
    job: "-"
  `
	mappings := []string{
		"test.dispatcher.FooProcessor.send.succeeded",
		"test.my-dispatch-host01.name.dispatcher.FooProcessor.send.succeeded",
		"request_time.get/threads/1/posts.200.00000000.nonversioned.discussions.a11bbcdf0ac64ec243658dc64b7100fb.172.20.0.1.12ba97b7eaa1a50001000001.",
		"foo.bar",
		"foo.bar.baz",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkRegex(b *testing.B) {
	config := `---
defaults:
  match_type: regex
mappings:
- match: test\.dispatcher\.([^.]*)\.([^.]*)\.([^.]*)
  name: "dispatch_events"
  labels:
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
- match: test.my-dispatch-host01.name.dispatcher\.([^.]*)\.([^.]*)\.([^.]*)
  name: "host_dispatch_events"
  labels:
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
- match: request_time\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)
  name: "tyk_http_request"
  labels:
    method_and_path: "${1}"
    response_code: "${2}"
    apikey: "${3}"
    apiversion: "${4}"
    apiname: "${5}"
    apiid: "${6}"
    ipv4_t1: "${7}"
    ipv4_t2: "${8}"
    ipv4_t3: "${9}"
    ipv4_t4: "${10}"
    orgid: "${11}"
    oauthid: "${12}"
- match: \.([^.]*)\.([^.]*)
  name: "catchall"
  labels:
    first: "$1"
    second: "$2"
    third: "$3"
    job: "-"
  `
	mappings := []string{
		"test.dispatcher.FooProcessor.send.succeeded",
		"test.my-dispatch-host01.name.dispatcher.FooProcessor.send.succeeded",
		"request_time.get/threads/1/posts.200.00000000.nonversioned.discussions.a11bbcdf0ac64ec243658dc64b7100fb.172.20.0.1.12ba97b7eaa1a50001000001.",
		"foo.bar",
		"foo.bar.baz",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkGlobSingleMatch(b *testing.B) {
	config := `---
mappings:
- match: metric.*
  name: "metric_one"
  labels:
    name: "$1"
  `
	mappings := []string{
		"metric.aaa",
		"metric.bbb",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkRegexSingleMatch(b *testing.B) {
	config := `---
mappings:
- match: metric\.([^.]*)
  name: "metric_one"
  match_type: regex
  labels:
    name: "$1"
  `
	mappings := []string{
		"metric.aaa",
		"metric.bbb",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkGlobMultipleCaptures(b *testing.B) {
	config := `---
mappings:
- match: metric.*.*.*.*.*.*.*.*.*.*.*.*
  name: "metric_multi"
  labels:
    name: "$1-$2-$3.$4-$5-$6.$7-$8-$9.$10-$11-$12"
  `
	mappings := []string{
		"metric.a.b.c.d.e.f.g.h.i.j.k.l",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkRegexMultipleCaptures(b *testing.B) {
	config := `---
mappings:
- match: metric\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)
  name: "metric_multi"
  match_type: regex
  labels:
    name: "$1-$2-$3.$4-$5-$6.$7-$8-$9.$10-$11-$12"
  `
	mappings := []string{
		"metric.a.b.c.d.e.f.g.h.i.j.k.l",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkGlobMultipleCapturesNoFormat(b *testing.B) {
	config := `---
mappings:
- match: metric.*.*.*.*.*.*.*.*.*.*.*.*
  name: "metric_multi"
  labels:
    name: "not_relevant"
  `
	mappings := []string{
		"metric.a.b.c.d.e.f.g.h.i.j.k.l",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkRegexMultipleCapturesNoFormat(b *testing.B) {
	config := `---
mappings:
- match: metric\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)
  name: "metric_multi"
  match_type: regex
  labels:
    name: "not_relevant"
  `
	mappings := []string{
		"metric.a.b.c.d.e.f.g.h.i.j.k.l",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkGlobMultipleCapturesDifferentLabels(b *testing.B) {
	config := `---
mappings:
- match: metric.*.*.*.*.*.*.*.*.*.*.*.*
  name: "metric_multi"
  labels:
    label1: "$1"
    label2: "$2"
    label3: "$3"
    label4: "$4"
    label5: "$5"
    label6: "$6"
    label7: "$7"
    label8: "$8"
    label9: "$9"
    label10: "$10"
    label11: "$11"
    label12: "$12"
  `
	mappings := []string{
		"metric.a.b.c.d.e.f.g.h.i.j.k.l",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkRegexMultipleCapturesDifferentLabels(b *testing.B) {
	config := `---
mappings:
- match: metric\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)\.([^.]*)
  name: "metric_multi"
  match_type: regex
  labels:
    label1: "$1"
    label2: "$2"
    label3: "$3"
    label4: "$4"
    label5: "$5"
    label6: "$6"
    label7: "$7"
    label8: "$8"
    label9: "$9"
    label10: "$10"
    label11: "$11"
    label12: "$12"
  `
	mappings := []string{
		"metric.a.b.c.d.e.f.g.h.i.j.k.l",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkGlob10Rules(b *testing.B) {
	config := `---
mappings:` + duplicateRules(100, ruleTemplateSingleMatchGlob)
	mappings := []string{
		"metric100.a",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkGlob10RulesCached(b *testing.B) {
	config := `---
mappings:` + duplicateRules(100, ruleTemplateSingleMatchGlob)
	mappings := []string{
		"metric100.a",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 1000)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkRegex10RulesAverage(b *testing.B) {
	config := `---
defaults:
  match_type: regex
mappings:` + duplicateRules(10, ruleTemplateSingleMatchRegex)
	mappings := []string{
		"metric5.a",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkRegex10RulesAverageCached(b *testing.B) {
	config := `---
defaults:
  match_type: regex
mappings:` + duplicateRules(10, ruleTemplateSingleMatchRegex)
	mappings := []string{
		"metric5.a",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 1000)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkGlob100Rules(b *testing.B) {
	config := `---
mappings:` + duplicateRules(100, ruleTemplateSingleMatchGlob)
	mappings := []string{
		"metric100.a",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkGlob100RulesCached(b *testing.B) {
	config := `---
mappings:` + duplicateRules(100, ruleTemplateSingleMatchGlob)
	mappings := []string{
		"metric100.a",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 1000)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkGlob100RulesNoMatch(b *testing.B) {
	config := `---
mappings:` + duplicateRules(100, ruleTemplateSingleMatchGlob)
	mappings := []string{
		"metricnomatchy.a",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkGlob100RulesNoOrderingNoMatch(b *testing.B) {
	config := `---
defaults:
  glob_disable_ordering: true
mappings:` + duplicateRules(100, ruleTemplateSingleMatchGlob)
	mappings := []string{
		"metricnomatchy.a",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkRegex100RulesAverage(b *testing.B) {
	config := `---
defaults:
  match_type: regex
mappings:` + duplicateRules(100, ruleTemplateSingleMatchRegex)
	mappings := []string{
		"metric50.a",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkRegex100RulesWorst(b *testing.B) {
	config := `---
defaults:
  match_type: regex
mappings:` + duplicateRules(100, ruleTemplateSingleMatchRegex)
	mappings := []string{
		"metric100.a",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkGlob100RulesMultipleCaptures(b *testing.B) {
	config := `---
mappings:` + duplicateRules(100, ruleTemplateMultipleMatchGlob)
	mappings := []string{
		"metric50.a.b.c.d.e.f.g.h.i.j.k.l",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkGlob100RulesMultipleCapturesCached(b *testing.B) {
	config := `---
mappings:` + duplicateRules(100, ruleTemplateMultipleMatchGlob)
	mappings := []string{
		"metric50.a.b.c.d.e.f.g.h.i.j.k.l",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 1000)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkRegex100RulesMultipleCapturesAverage(b *testing.B) {
	config := `---
defaults:
  match_type: regex
mappings:` + duplicateRules(100, ruleTemplateMultipleMatchRegex)
	mappings := []string{
		"metric50.a.b.c.d.e.f.g.h.i.j.k.l",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkRegex100RulesMultipleCapturesWorst(b *testing.B) {
	config := `---
defaults:
  match_type: regex
mappings:` + duplicateRules(100, ruleTemplateMultipleMatchRegex)
	mappings := []string{
		"metric100.a.b.c.d.e.f.g.h.i.j.k.l",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}

func BenchmarkRegex100RulesMultipleCapturesWorstCached(b *testing.B) {
	config := `---
defaults:
  match_type: regex
mappings:` + duplicateRules(100, ruleTemplateMultipleMatchRegex)
	mappings := []string{
		"metric100.a.b.c.d.e.f.g.h.i.j.k.l",
	}

	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 1000)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		for _, metric := range mappings {
			mapper.GetMapping(metric, MetricTypeCounter)
		}
	}
}
