// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
	"fmt"
	"strings"
)

// AdversarialCase is a single hand-crafted payload that exercises a specific
// Prometheus/OpenMetrics spec corner. Unlike Mutator output, each case is
// reproducible by name and triages directly to a behavior class.
type AdversarialCase struct {
	Name        string                 // category/specific (e.g. "histogram/non_monotonic")
	Description string                 // one-line rationale, surfaced in test failures
	Payload     []byte                 // exact bytes served from httptest.Server
	Instance    map[string]interface{} // optional non-default instance config (nil = use default)
}

// defaultAdversarialInstance is the instance config used when an AdversarialCase
// leaves Instance nil. Same as the corpus/fuzz default: wildcard match, fixed
// namespace.
var defaultAdversarialInstance = map[string]interface{}{
	"namespace": "diff",
	"metrics":   []interface{}{".+"},
}

// AdversarialCatalog is the full set of Layer-3 cases. Adding a case is meant
// to be cheap: one literal here, no glue.
var AdversarialCatalog = func() []AdversarialCase {
	var out []AdversarialCase
	out = append(out, histogramCases()...)
	out = append(out, summaryCases()...)
	out = append(out, labelCases()...)
	out = append(out, metricNameCases()...)
	out = append(out, formatMixingCases()...)
	out = append(out, valueRenderingCases()...)
	return out
}()

// ---- builders ---------------------------------------------------------------

// family writes a single metric family with shared HELP/TYPE.
func family(name, typ string, samples ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# HELP %s synthetic adversarial case\n", name)
	fmt.Fprintf(&b, "# TYPE %s %s\n", name, typ)
	for _, s := range samples {
		b.WriteString(s)
		b.WriteByte('\n')
	}
	return b.String()
}

func bucket(name, le string, value int) string {
	return fmt.Sprintf(`%s_bucket{le="%s"} %d`, name, le, value)
}

func quantile(name, q string, value float64) string {
	return fmt.Sprintf(`%s{quantile="%s"} %g`, name, q, value)
}

// ---- histogram corners ------------------------------------------------------

func histogramCases() []AdversarialCase {
	var cases []AdversarialCase

	// Non-monotonic buckets: 0.5 cumulative <= 1.0 cumulative is the contract;
	// here bucket(le=1.0)=3 < bucket(le=0.5)=5 violates it.
	cases = append(cases, AdversarialCase{
		Name:        "histogram/non_monotonic_buckets",
		Description: "buckets out of order: le=1.0 has fewer counts than le=0.5",
		Payload: []byte(family("req_dur", "histogram",
			bucket("req_dur", "0.5", 5),
			bucket("req_dur", "1.0", 3),
			bucket("req_dur", "+Inf", 10),
			"req_dur_count 10",
			"req_dur_sum 4.2",
		)),
	})

	// Missing +Inf bucket: the spec requires it; many parsers tolerate but
	// some transformers reject.
	cases = append(cases, AdversarialCase{
		Name:        "histogram/missing_plus_inf",
		Description: "histogram with no +Inf bucket; _count present",
		Payload: []byte(family("req_dur", "histogram",
			bucket("req_dur", "0.5", 5),
			bucket("req_dur", "1.0", 8),
			"req_dur_count 10",
			"req_dur_sum 4.2",
		)),
	})

	// Duplicate le value: which bucket wins is implementation-defined.
	cases = append(cases, AdversarialCase{
		Name:        "histogram/duplicate_le",
		Description: "two buckets with identical le=1.0; different counts",
		Payload: []byte(family("req_dur", "histogram",
			bucket("req_dur", "0.5", 5),
			bucket("req_dur", "1.0", 8),
			bucket("req_dur", "1.0", 7),
			bucket("req_dur", "+Inf", 10),
			"req_dur_count 10",
			"req_dur_sum 4.2",
		)),
	})

	// Negative le: counter-style values can technically be negative for some
	// gauges, but bucket boundaries are usually >= 0. Spec allows it.
	cases = append(cases, AdversarialCase{
		Name:        "histogram/negative_le_bound",
		Description: "bucket with negative le boundary",
		Payload: []byte(family("req_dur", "histogram",
			bucket("req_dur", "-1.0", 0),
			bucket("req_dur", "0", 2),
			bucket("req_dur", "+Inf", 10),
			"req_dur_count 10",
			"req_dur_sum 4.2",
		)),
	})

	// _count != last (+Inf) bucket count: spec requires equality.
	cases = append(cases, AdversarialCase{
		Name:        "histogram/count_neq_inf_bucket",
		Description: "_count = 99 but +Inf bucket = 10",
		Payload: []byte(family("req_dur", "histogram",
			bucket("req_dur", "0.5", 5),
			bucket("req_dur", "+Inf", 10),
			"req_dur_count 99",
			"req_dur_sum 4.2",
		)),
	})

	// _sum is NaN: spec allows NaN for sums when negative observations present;
	// transformers must propagate without crashing.
	cases = append(cases, AdversarialCase{
		Name:        "histogram/sum_is_nan",
		Description: "histogram with _sum=NaN (valid when observations include negatives)",
		Payload: []byte(family("req_dur", "histogram",
			bucket("req_dur", "0.5", 5),
			bucket("req_dur", "+Inf", 10),
			"req_dur_count 10",
			"req_dur_sum NaN",
		)),
	})

	// _sum is +Inf: implementations may or may not coerce.
	cases = append(cases, AdversarialCase{
		Name:        "histogram/sum_is_inf",
		Description: "histogram with _sum=+Inf",
		Payload: []byte(family("req_dur", "histogram",
			bucket("req_dur", "0.5", 5),
			bucket("req_dur", "+Inf", 10),
			"req_dur_count 10",
			"req_dur_sum +Inf",
		)),
	})

	// Missing _count and _sum: the buckets alone might be submitted as a
	// degraded histogram, or the whole family rejected.
	cases = append(cases, AdversarialCase{
		Name:        "histogram/missing_count_and_sum",
		Description: "buckets only, no _count or _sum trailers",
		Payload: []byte(family("req_dur", "histogram",
			bucket("req_dur", "0.5", 5),
			bucket("req_dur", "+Inf", 10),
		)),
	})

	// Bucket without TYPE declaration: heuristic name-based bucket detection?
	cases = append(cases, AdversarialCase{
		Name:        "histogram/no_type_declaration",
		Description: "foo_bucket{} samples present but no `# TYPE foo histogram`",
		Payload: []byte("# HELP req_dur synthetic adversarial case\n" +
			bucket("req_dur", "0.5", 5) + "\n" +
			bucket("req_dur", "+Inf", 10) + "\n" +
			"req_dur_count 10\n" +
			"req_dur_sum 4.2\n"),
	})

	return cases
}

// ---- summary corners --------------------------------------------------------

func summaryCases() []AdversarialCase {
	var cases []AdversarialCase

	// Quantile > 1: invalid per spec.
	cases = append(cases, AdversarialCase{
		Name:        "summary/quantile_above_one",
		Description: "quantile=1.5 (invalid per spec; range is [0,1])",
		Payload: []byte(family("req_lat", "summary",
			quantile("req_lat", "0.5", 0.1),
			quantile("req_lat", "1.5", 999.0),
			"req_lat_count 100",
			"req_lat_sum 50",
		)),
	})

	// Negative quantile.
	cases = append(cases, AdversarialCase{
		Name:        "summary/quantile_negative",
		Description: "quantile=-0.5",
		Payload: []byte(family("req_lat", "summary",
			quantile("req_lat", "-0.5", -1.0),
			quantile("req_lat", "0.5", 0.1),
			"req_lat_count 100",
			"req_lat_sum 50",
		)),
	})

	// Non-numeric quantile.
	cases = append(cases, AdversarialCase{
		Name:        "summary/quantile_non_numeric",
		Description: `quantile="median" (text instead of number)`,
		Payload: []byte(family("req_lat", "summary",
			`req_lat{quantile="median"} 0.1`,
			"req_lat_count 100",
			"req_lat_sum 50",
		)),
	})

	// Duplicate quantile.
	cases = append(cases, AdversarialCase{
		Name:        "summary/duplicate_quantile",
		Description: "two samples with quantile=0.5, different values",
		Payload: []byte(family("req_lat", "summary",
			quantile("req_lat", "0.5", 0.1),
			quantile("req_lat", "0.5", 0.2),
			"req_lat_count 100",
			"req_lat_sum 50",
		)),
	})

	// Summary without count/sum.
	cases = append(cases, AdversarialCase{
		Name:        "summary/missing_count_and_sum",
		Description: "quantiles only, no _count or _sum trailers",
		Payload: []byte(family("req_lat", "summary",
			quantile("req_lat", "0.5", 0.1),
			quantile("req_lat", "0.99", 0.5),
		)),
	})

	return cases
}

// ---- label corners ----------------------------------------------------------

func labelCases() []AdversarialCase {
	var cases []AdversarialCase

	// Duplicate label name within one sample: invalid per spec.
	cases = append(cases, AdversarialCase{
		Name:        "labels/duplicate_name",
		Description: `{foo="a",foo="b"} — same label name twice`,
		Payload:     []byte(family("m", "gauge", `m{foo="a",foo="b"} 1`)),
	})

	// Empty label value (legal per spec, treated as label absent in Prometheus
	// semantics).
	cases = append(cases, AdversarialCase{
		Name:        "labels/empty_value",
		Description: `{foo=""} — empty value should be equivalent to label absent`,
		Payload: []byte(family("m", "gauge",
			`m{foo=""} 1`,
			`m{foo="x"} 2`,
			`m 3`,
		)),
	})

	// Very long label value.
	longVal := strings.Repeat("x", 2048)
	cases = append(cases, AdversarialCase{
		Name:        "labels/very_long_value",
		Description: "2048-char label value",
		Payload:     []byte(family("m", "gauge", fmt.Sprintf(`m{long=%q} 1`, longVal))),
	})

	// Many labels on one sample (cardinality).
	cases = append(cases, AdversarialCase{
		Name:        "labels/wide_set_64",
		Description: "64 distinct labels on one sample",
		Payload: func() []byte {
			var lb strings.Builder
			lb.WriteByte('{')
			for i := 0; i < 64; i++ {
				if i > 0 {
					lb.WriteByte(',')
				}
				fmt.Fprintf(&lb, `lbl_%02d="v%02d"`, i, i)
			}
			lb.WriteByte('}')
			return []byte(family("m", "gauge", "m"+lb.String()+" 1"))
		}(),
	})

	// Embedded quote and backslash in label value.
	cases = append(cases, AdversarialCase{
		Name:        "labels/escape_sequences",
		Description: `escapes: \\, \", \n in label value`,
		Payload:     []byte(family("m", "gauge", `m{a="x\"y\\z\nq"} 1`)),
	})

	// Label value with raw newline (illegal per spec, escape required).
	cases = append(cases, AdversarialCase{
		Name:        "labels/raw_newline_in_value",
		Description: "unescaped newline inside label value",
		Payload:     []byte("# HELP m synthetic\n# TYPE m gauge\nm{a=\"hello\nworld\"} 1\n"),
	})

	return cases
}

// ---- metric name corners ----------------------------------------------------

func metricNameCases() []AdversarialCase {
	var cases []AdversarialCase

	// Same metric name, two TYPE lines with different types.
	cases = append(cases, AdversarialCase{
		Name:        "name/conflicting_type",
		Description: "same metric declared as gauge then counter",
		Payload: []byte("# TYPE m gauge\nm 1\n" +
			"# TYPE m counter\nm 2\n"),
	})

	// Counter named `foo_total` AND a separate metric `foo`: name munging may
	// collide on the Datadog-side metric name (the `.count` suffix).
	cases = append(cases, AdversarialCase{
		Name:        "name/total_suffix_collision",
		Description: "counter foo_total alongside separate metric foo (both could map to diff.foo.count)",
		Payload: []byte("# TYPE foo gauge\nfoo 1\n" +
			"# TYPE foo_total counter\nfoo_total 5\n"),
	})

	// HELP/TYPE in the wrong order (TYPE before HELP).
	cases = append(cases, AdversarialCase{
		Name:        "name/type_before_help",
		Description: "# TYPE precedes # HELP for the same metric",
		Payload:     []byte("# TYPE m gauge\n# HELP m synthetic\nm 1\n"),
	})

	// HELP without TYPE.
	cases = append(cases, AdversarialCase{
		Name:        "name/help_without_type",
		Description: "HELP present, TYPE absent: defaults to untyped",
		Payload:     []byte("# HELP m synthetic\nm 1\n"),
	})

	// TYPE for a metric that has no samples.
	cases = append(cases, AdversarialCase{
		Name:        "name/type_no_samples",
		Description: "# TYPE m gauge declared but zero samples for m",
		Payload:     []byte("# TYPE m gauge\n# TYPE n gauge\nn 1\n"),
	})

	return cases
}

// ---- format mixing ----------------------------------------------------------

// These cases use OpenMetrics 1.0.0 features in payloads that the harness will
// nonetheless serve as `text/plain; version=0.0.4` (Prometheus content-type).
// That mismatch is itself an interesting test: which parser switches modes on
// content-type vs which switches on content sniffing?
func formatMixingCases() []AdversarialCase {
	var cases []AdversarialCase

	// OpenMetrics # UNIT directive in a Prometheus payload.
	cases = append(cases, AdversarialCase{
		Name:        "format/openmetrics_unit_directive",
		Description: "# UNIT line (OpenMetrics 1.0.0 only) inside Prometheus-text payload",
		Payload: []byte("# HELP req_dur duration\n" +
			"# TYPE req_dur gauge\n" +
			"# UNIT req_dur seconds\n" +
			"req_dur 0.42\n"),
	})

	// OpenMetrics exemplar trailer.
	cases = append(cases, AdversarialCase{
		Name:        "format/openmetrics_exemplar",
		Description: "OpenMetrics exemplar trailer on a sample line",
		Payload: []byte("# TYPE req_dur histogram\n" +
			`req_dur_bucket{le="0.5"} 5 # {trace_id="abc"} 0.3 1620000000.0` + "\n" +
			`req_dur_bucket{le="+Inf"} 10` + "\n" +
			"req_dur_count 10\n" +
			"req_dur_sum 4.2\n"),
	})

	// OpenMetrics # EOF marker.
	cases = append(cases, AdversarialCase{
		Name:        "format/openmetrics_eof",
		Description: "# EOF (OpenMetrics terminator) at end of payload",
		Payload: []byte("# TYPE m gauge\n" +
			"m 1\n" +
			"# EOF\n"),
	})

	// Samples *after* # EOF.
	cases = append(cases, AdversarialCase{
		Name:        "format/samples_after_eof",
		Description: "sample lines appearing after a # EOF marker",
		Payload: []byte("# TYPE m gauge\n" +
			"m 1\n" +
			"# EOF\n" +
			"m 2\n"),
	})

	// Sample line with millisecond timestamp.
	cases = append(cases, AdversarialCase{
		Name:        "format/timestamp_per_sample",
		Description: "sample includes explicit timestamp (Prometheus extension)",
		Payload:     []byte("# TYPE m gauge\nm 42 1620000000000\n"),
	})

	// CRLF line endings instead of LF.
	cases = append(cases, AdversarialCase{
		Name:        "format/crlf_line_endings",
		Description: "\\r\\n line endings",
		Payload:     []byte("# TYPE m gauge\r\nm 1\r\n"),
	})

	// Mixed LF and CRLF.
	cases = append(cases, AdversarialCase{
		Name:        "format/mixed_line_endings",
		Description: "some lines LF, some CRLF",
		Payload:     []byte("# TYPE m gauge\nm 1\r\nm 2\n"),
	})

	// --- Fuzz-discovered tiny cases (each minimized to <40 bytes by the
	// engine). Promoted from /tmp/fuzz-findings to permanent regression seeds
	// because they're minimal repros for the architectural-divergence bug
	// class and will quickly verify any fix.

	// Bare "# TYPE" with no metric/type fields. Go's parser likely errors
	// on the truncated line; Python likely skips it.
	cases = append(cases, AdversarialCase{
		Name:        "format/bare_type_keyword",
		Description: "`# TYPE` line with no metric name or type field (fuzz-minimized)",
		Payload:     []byte("# TYPE"),
	})

	// Bare "# HELP" with trailing space.
	cases = append(cases, AdversarialCase{
		Name:        "format/bare_help_keyword",
		Description: "`# HELP ` line with no metric name (fuzz-minimized)",
		Payload:     []byte("# HELP "),
	})

	// Form-feed (0x0c) at top level: control char outside string context.
	cases = append(cases, AdversarialCase{
		Name:        "format/leading_form_feed",
		Description: "payload starts with space + form-feed (\\x0c) (fuzz-minimized)",
		Payload:     []byte(" \f"),
	})

	// Numeric metric name with empty braces. Per spec, metric names must
	// start with letter or underscore; `0` should be rejected.
	cases = append(cases, AdversarialCase{
		Name:        "name/numeric_metric_name",
		Description: "`0{}0` — metric name is a digit, illegal per spec (fuzz-minimized)",
		Payload:     []byte("0{}0"),
	})

	return cases
}

// ---- value rendering --------------------------------------------------------

func valueRenderingCases() []AdversarialCase {
	var cases []AdversarialCase

	// Integer-only rendering vs decimal: "42" vs "42.0".
	cases = append(cases, AdversarialCase{
		Name:        "values/integer_vs_decimal",
		Description: "counter rendered as `1` (int) vs `1.0` (decimal)",
		Payload: []byte("# TYPE m counter\n" +
			`m{form="int"} 1` + "\n" +
			`m{form="dec"} 1.0` + "\n"),
	})

	// Hexadecimal float (illegal in Prometheus, legal in some C-style parsers).
	cases = append(cases, AdversarialCase{
		Name:        "values/hex_float",
		Description: "value rendered as 0x1.fffp+0 (not legal in Prometheus text)",
		Payload:     []byte("# TYPE m gauge\nm 0x1.fffp+0\n"),
	})

	// Leading/trailing whitespace.
	cases = append(cases, AdversarialCase{
		Name:        "values/leading_whitespace",
		Description: "sample line with leading spaces",
		Payload:     []byte("# TYPE m gauge\n   m 1\n"),
	})

	// Very large positive value beyond float64 range.
	cases = append(cases, AdversarialCase{
		Name:        "values/over_max_float64",
		Description: "value = 1e400 (exceeds float64 range; should become +Inf)",
		Payload:     []byte("# TYPE m gauge\nm 1e400\n"),
	})

	// Subnormal: smallest representable positive double.
	cases = append(cases, AdversarialCase{
		Name:        "values/smallest_subnormal",
		Description: "value = 5e-324 (smallest positive subnormal double)",
		Payload:     []byte("# TYPE m gauge\nm 5e-324\n"),
	})

	// Value with explicit positive sign.
	cases = append(cases, AdversarialCase{
		Name:        "values/explicit_plus_sign",
		Description: "value `+42` (explicit plus; not all parsers accept)",
		Payload:     []byte("# TYPE m gauge\nm +42\n"),
	})

	// Trailing characters after value: ignored or error?
	cases = append(cases, AdversarialCase{
		Name:        "values/trailing_garbage",
		Description: "`m 1 abc` (extra token after value, before optional timestamp)",
		Payload:     []byte("# TYPE m gauge\nm 1 abc\n"),
	})

	return cases
}
