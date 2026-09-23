// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! Detection of Prometheus/OpenMetrics exposition format in HTTP bodies.
//!
//! This is used by the `disco` CLI to empirically check whether a process
//! exposes a metrics endpoint, and in particular whether the endpoint carries
//! the `go_*` runtime metrics exposed by default by `prometheus/client_golang`.
//!
//! The parser follows the Prometheus text exposition format
//! (<https://prometheus.io/docs/instrumenting/exposition_formats/>) and the
//! OpenMetrics specification (<https://github.com/OpenObservability/OpenMetrics>).

use serde::{Deserialize, Serialize};

/// Maximum number of distinct metric names kept in the analysis output.
const MAX_METRIC_NAMES: usize = 200;

#[derive(Debug, Default, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum MetricsFormat {
    #[default]
    Unknown,
    PrometheusText,
    OpenMetrics,
}

/// Result of analyzing an HTTP response body for Prometheus/OpenMetrics
/// exposition content.
#[derive(Debug, Default, Clone, Serialize, Deserialize)]
pub struct MetricsAnalysis {
    /// True when the body parses as a Prometheus/OpenMetrics exposition
    /// document.
    pub is_metrics_endpoint: bool,
    pub format: MetricsFormat,
    /// Number of valid sample lines (`name{...} value [timestamp]`).
    pub sample_count: usize,
    /// Number of lines that failed to parse as exposition content.
    pub invalid_line_count: usize,
    /// Total number of distinct metric names seen (samples + HELP/TYPE lines).
    pub metric_name_count: usize,
    /// First `MAX_METRIC_NAMES` distinct metric names, in order of appearance.
    pub metric_names: Vec<String>,
    /// True when at least one `go_*` metric is present (the Go runtime metrics
    /// registered by default by prometheus/client_golang).
    pub has_go_runtime_metrics: bool,
    pub go_runtime_metric_count: usize,
    pub go_runtime_metric_names: Vec<String>,
}

/// Analyzes a response body. `body_truncated` indicates that the body was cut
/// before the end (e.g. because of a read cap); in that case the last line is
/// ignored since it may be incomplete.
pub fn analyze(body: &str, body_truncated: bool) -> MetricsAnalysis {
    let mut analysis = MetricsAnalysis::default();
    let mut names: Vec<String> = Vec::new();
    let mut go_names: Vec<String> = Vec::new();

    let lines: Vec<&str> = body.lines().collect();
    // When the body was truncated mid-line, the last line may be incomplete.
    let consider = if body_truncated && !body.ends_with('\n') {
        lines.len().saturating_sub(1)
    } else {
        lines.len()
    };

    for line in lines.iter().take(consider) {
        let line = line.trim_end_matches('\r');
        if line.trim().is_empty() {
            continue;
        }
        if line.starts_with('#') {
            if !handle_comment_line(line, &mut analysis, &mut names) {
                analysis.invalid_line_count += 1;
            }
        } else if let Some(name) = parse_sample_line(line) {
            analysis.sample_count += 1;
            record_name(&mut names, &mut analysis, name);
            if name.starts_with("go_") {
                analysis.has_go_runtime_metrics = true;
                if !go_names.iter().any(|n| n == name) {
                    analysis.go_runtime_metric_count += 1;
                    go_names.push(name.to_string());
                }
            }
        } else {
            analysis.invalid_line_count += 1;
        }
    }

    analysis.metric_names = names;
    analysis.go_runtime_metric_names = go_names;
    analysis.is_metrics_endpoint =
        analysis.sample_count >= 1 && analysis.invalid_line_count == 0;
    if analysis.is_metrics_endpoint && analysis.format == MetricsFormat::Unknown {
        // A well-formed exposition document terminated without the
        // OpenMetrics `# EOF` marker: classic Prometheus text format.
        analysis.format = MetricsFormat::PrometheusText;
    }

    analysis
}

fn record_name(names: &mut Vec<String>, analysis: &mut MetricsAnalysis, name: &str) {
    if !names.iter().any(|n| n == name) {
        analysis.metric_name_count += 1;
        if names.len() < MAX_METRIC_NAMES {
            names.push(name.to_string());
        }
    }
}

/// Handles a line starting with `#`. Returns true when the line is a valid
/// HELP/TYPE/UNIT directive, an `# EOF` marker (OpenMetrics), or a plain
/// comment.
fn handle_comment_line(
    line: &str,
    analysis: &mut MetricsAnalysis,
    names: &mut Vec<String>,
) -> bool {
    // The '#' is one byte, so this is always a valid boundary.
    let content = line.get(1..).unwrap_or("").trim_start();
    let (keyword, rest) = split_first_word(content);
    match keyword {
        "HELP" => {
            let (name, _doc) = split_first_word(rest);
            if !valid_metric_name(name) {
                return false;
            }
            record_name(names, analysis, name);
            true
        }
        "TYPE" => {
            let (name, rest) = split_first_word(rest);
            if !valid_metric_name(name) {
                return false;
            }
            let (type_str, _) = split_first_word(rest);
            if !valid_metric_type(type_str) {
                return false;
            }
            record_name(names, analysis, name);
            true
        }
        "UNIT" => {
            let (name, rest) = split_first_word(rest);
            if !valid_metric_name(name) {
                return false;
            }
            let (unit, _) = split_first_word(rest);
            if unit.is_empty() || unit.contains(char::is_whitespace) {
                return false;
            }
            record_name(names, analysis, name);
            true
        }
        "EOF" => {
            if !rest.is_empty() {
                return false;
            }
            analysis.format = MetricsFormat::OpenMetrics;
            true
        }
        // Plain comment line, ignored per the exposition format.
        _ => true,
    }
}

/// Splits `s` at the first whitespace into `(word, rest-without-leading-space)`.
fn split_first_word(s: &str) -> (&str, &str) {
    let Some(pos) = s.find(char::is_whitespace) else {
        return (s, "");
    };
    let word = s.get(0..pos).unwrap_or("");
    let rest = s.get(pos..).unwrap_or("").trim_start();
    (word, rest)
}

fn valid_metric_name(name: &str) -> bool {
    let mut chars = name.chars();
    match chars.next() {
        Some(c) if c.is_ascii_alphabetic() || c == '_' || c == ':' => {}
        _ => return false,
    }
    chars.all(|c| c.is_ascii_alphanumeric() || c == '_' || c == ':')
}

fn valid_metric_type(t: &str) -> bool {
    matches!(
        t,
        "counter"
            | "gauge"
            | "histogram"
            | "gaugehistogram"
            | "summary"
            | "info"
            | "stateset"
            | "unknown"
            | "untyped"
    )
}

fn is_metric_name_char(c: char) -> bool {
    c.is_ascii_alphanumeric() || c == '_' || c == ':'
}

/// Parses a sample line of the exposition format:
/// `metric_name [ "{" labels "}" ] whitespace value [ whitespace timestamp ]`.
/// Returns the metric name on success.
fn parse_sample_line(line: &str) -> Option<&str> {
    let split = line.find(|c: char| !is_metric_name_char(c))?;
    if split == 0 {
        return None;
    }
    let name = line.get(0..split)?;
    if !valid_metric_name(name) {
        return None;
    }
    let after_name = line.get(split..)?;

    // Optional label set.
    let after_labels = if let Some(labels) = after_name.strip_prefix('{') {
        let close = find_labels_end(labels)?;
        let labels_content = labels.get(0..close)?;
        if !valid_labels_content(labels_content) {
            return None;
        }
        let after = labels.get(close + 1..)?;
        if !after.starts_with(char::is_whitespace) {
            return None;
        }
        after
    } else {
        if !after_name.starts_with(char::is_whitespace) {
            return None;
        }
        after_name
    };

    // Value.
    let value_part = after_labels.trim_start();
    if value_part.is_empty() {
        return None;
    }
    let value_end = value_part
        .find(char::is_whitespace)
        .unwrap_or(value_part.len());
    let value = value_part.get(0..value_end)?;
    parse_prometheus_float(value)?;

    // Optional timestamp.
    let ts_part = value_part.get(value_end..)?.trim_start();
    if !ts_part.is_empty() {
        let ts_end = ts_part.find(char::is_whitespace).unwrap_or(ts_part.len());
        parse_prometheus_float(ts_part.get(0..ts_end)?)?;
        if !ts_part.get(ts_end..)?.trim_start().is_empty() {
            return None;
        }
    }

    Some(name)
}

/// Accepts float values as defined by the exposition format: standard float
/// literals plus `NaN`, `Inf`, `+Inf`, `-Inf`. (Rust's `f64::from_str`
/// accepts those case-insensitively, plus a few extra spellings like
/// `infinity` which we tolerate.)
fn parse_prometheus_float(s: &str) -> Option<f64> {
    if s.is_empty() {
        return None;
    }
    s.parse::<f64>().ok()
}

/// Returns the index of the `}` that closes a label set, ignoring braces
/// inside quoted label values.
fn find_labels_end(s: &str) -> Option<usize> {
    let mut in_quote = false;
    let mut escaped = false;
    for (i, b) in s.bytes().enumerate() {
        if escaped {
            escaped = false;
            continue;
        }
        if in_quote {
            match b {
                b'\\' => escaped = true,
                b'"' => in_quote = false,
                _ => {}
            }
        } else if b == b'"' {
            in_quote = true;
        } else if b == b'}' {
            return Some(i);
        }
    }
    None
}

/// Returns the index of the closing (unescaped) `"` starting the scan at
/// index 0 (the opening quote is not part of `s`).
fn find_quote_end(s: &str) -> Option<usize> {
    let mut escaped = false;
    for (i, b) in s.bytes().enumerate() {
        if escaped {
            escaped = false;
            continue;
        }
        match b {
            b'\\' => escaped = true,
            b'"' => return Some(i),
            _ => {}
        }
    }
    None
}

/// Validates the content of a label set (between the braces): a comma
/// separated list of `name="value"` pairs, or empty.
fn valid_labels_content(s: &str) -> bool {
    let trimmed = s.trim();
    if trimmed.is_empty() {
        return true;
    }
    let mut rest = trimmed;
    loop {
        let Some(eq) = rest.find('=') else {
            return false;
        };
        let name = rest.get(0..eq).unwrap_or("");
        if !valid_label_name(name) {
            return false;
        }
        let after_eq = rest.get(eq + 1..).unwrap_or("");
        if !after_eq.starts_with('"') {
            return false;
        }
        // Scan the label value (between the quotes).
        let value = after_eq.get(1..).unwrap_or("");
        let Some(close) = find_quote_end(value) else {
            return false;
        };
        let value_content = value.get(0..close).unwrap_or("");
        if !valid_escapes(value_content) {
            return false;
        }
        let after = value.get(close + 1..).unwrap_or("");
        if after.is_empty() {
            return true;
        }
        let Some(next) = after.strip_prefix(',') else {
            return false;
        };
        rest = next.trim_start();
        if rest.is_empty() {
            // Trailing comma is not allowed.
            return false;
        }
    }
}

fn valid_label_name(name: &str) -> bool {
    let mut chars = name.chars();
    match chars.next() {
        Some(c) if c.is_ascii_alphabetic() || c == '_' => {}
        _ => return false,
    }
    chars.all(|c| c.is_ascii_alphanumeric() || c == '_')
}

/// Only `\\`, `\"` and `\n` escapes are allowed in label values.
fn valid_escapes(v: &str) -> bool {
    let mut chars = v.chars();
    while let Some(c) = chars.next() {
        if c == '\\' {
            match chars.next() {
                Some('\\') | Some('"') | Some('n') => {}
                _ => return false,
            }
        }
    }
    true
}

#[cfg(test)]
#[allow(clippy::unwrap_used, clippy::expect_used)]
mod tests {
    use super::*;

    const CLIENT_GOLANG_BODY: &str = r#"# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 42
# HELP go_gc_duration_seconds A summary of the pause duration of garbage collection cycles.
# TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 1.5e-05
go_gc_duration_seconds{quantile="0.75"} 0.000123
go_gc_duration_seconds_sum 0.0045
go_gc_duration_seconds_count 12
# HELP http_requests_total Total number of HTTP requests.
# TYPE http_requests_total counter
http_requests_total{code="200",method="get",handler="/metrics"} 1027
"#;

    #[test]
    fn test_client_golang_body() {
        let a = analyze(CLIENT_GOLANG_BODY, false);
        assert!(a.is_metrics_endpoint);
        assert_eq!(a.format, MetricsFormat::PrometheusText);
        assert_eq!(a.sample_count, 6);
        assert_eq!(a.invalid_line_count, 0);
        assert!(a.has_go_runtime_metrics);
        // go_goroutines, go_gc_duration_seconds, go_gc_duration_seconds_sum
        // and _count are four distinct metric names.
        assert_eq!(a.go_runtime_metric_count, 4);
        assert!(a.metric_names.contains(&"go_goroutines".to_string()));
        assert!(a.metric_names.contains(&"http_requests_total".to_string()));
    }

    #[test]
    fn test_openmetrics_format() {
        let body = "# TYPE go_threads gauge\ngo_threads 9\n# EOF\n";
        let a = analyze(body, false);
        assert!(a.is_metrics_endpoint);
        assert_eq!(a.format, MetricsFormat::OpenMetrics);
        assert!(a.has_go_runtime_metrics);
    }

    #[test]
    fn test_values() {
        let body = "a 1\nb -3.14\nc NaN\nd +Inf\ne -Inf\nf 1e-9\ng 0\n";
        let a = analyze(body, false);
        assert!(a.is_metrics_endpoint);
        assert_eq!(a.sample_count, 7);
    }

    #[test]
    fn test_timestamps() {
        let body = "up 1 1680000000000\nbuild_info{a=\"b\"} 1 1.68e9\n";
        let a = analyze(body, false);
        assert!(a.is_metrics_endpoint);
        assert_eq!(a.sample_count, 2);
    }

    #[test]
    fn test_html_is_not_metrics() {
        let body = "<html><head><title>metrics</title></head><body>go_goroutines 1</body></html>";
        let a = analyze(body, false);
        assert!(!a.is_metrics_endpoint);
        assert_eq!(a.sample_count, 0);
    }

    #[test]
    fn test_json_is_not_metrics() {
        let body = r#"{"go_goroutines": 1, "status": "ok"}"#;
        let a = analyze(body, false);
        assert!(!a.is_metrics_endpoint);
    }

    #[test]
    fn test_empty_body() {
        let a = analyze("", false);
        assert!(!a.is_metrics_endpoint);
        assert_eq!(a.sample_count, 0);
    }

    #[test]
    fn test_only_comments() {
        let body = "# HELP a help\n# TYPE a gauge\n";
        let a = analyze(body, false);
        assert!(!a.is_metrics_endpoint);
        assert_eq!(a.sample_count, 0);
        assert_eq!(a.metric_name_count, 1);
    }

    #[test]
    fn test_truncated_body_ignores_partial_last_line() {
        let full = "go_goroutines 1\nhttp_requests_total 5\n";
        let truncated: String = full.get(..full.len() - 5).unwrap_or("").to_string();
        let a = analyze(&truncated, true);
        assert!(a.is_metrics_endpoint);
        assert_eq!(a.sample_count, 1);
        // The partial line must not be counted as invalid.
        assert_eq!(a.invalid_line_count, 0);
    }

    #[test]
    fn test_truncated_body_complete_last_line() {
        let full = "go_goroutines 1\nhttp_requests_total 5\n";
        let cut: String = full.get(..full.len() - 1).unwrap_or("").to_string(); // drop only '\n'
        let a = analyze(&cut, true);
        // cut does not end with \n and last line is complete; parse either way.
        assert!(a.is_metrics_endpoint);
    }

    #[test]
    fn test_invalid_lines_counted() {
        let body = "go_goroutines 1\nnot a metric line\nhttp 200 OK\n";
        let a = analyze(body, false);
        assert!(!a.is_metrics_endpoint);
        assert_eq!(a.sample_count, 1);
        assert_eq!(a.invalid_line_count, 2);
    }

    #[test]
    fn test_label_escaping() {
        let ok = "metric{label=\"a\\\"b\\\\c\\nd\"} 1\n";
        assert!(analyze(ok, false).is_metrics_endpoint);

        let bad = "metric{label=\"a\\xb\"} 1\n";
        assert!(!analyze(bad, false).is_metrics_endpoint);

        let braces_in_value = "metric{path=\"/}\"} 1\n";
        assert!(analyze(braces_in_value, false).is_metrics_endpoint);

        let trailing_comma = "metric{a=\"1\",} 1\n";
        assert!(!analyze(trailing_comma, false).is_metrics_endpoint);
    }

    #[test]
    fn test_bad_type_directive() {
        let body = "# TYPE go_goroutines wrongtype\ngo_goroutines 1\n";
        let a = analyze(body, false);
        assert!(!a.is_metrics_endpoint);
    }

    #[test]
    fn test_metric_name_validation() {
        // ':' is allowed, leading digits are not.
        assert!(analyze("a:b 1\n", false).is_metrics_endpoint);
        assert!(!analyze("1abc 1\n", false).is_metrics_endpoint);
        // Value missing.
        assert!(!analyze("go_goroutines\n", false).is_metrics_endpoint);
        // '=' instead of whitespace.
        assert!(!analyze("foo=1\n", false).is_metrics_endpoint);
    }

    #[test]
    fn test_crlf_line_endings() {
        let body = "go_goroutines 1\r\nhttp_requests_total 5\r\n";
        let a = analyze(body, false);
        assert!(a.is_metrics_endpoint);
    }
}
