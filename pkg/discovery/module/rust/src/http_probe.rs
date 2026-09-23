// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! Minimal HTTP/1.1 GET prober used by the `disco` CLI to check whether a
//! TCP port serves Prometheus/OpenMetrics content on a given path (typically
//! `/metrics`).
//!
//! This is intentionally a small, dependency-free implementation over
//! `std::net`: the probe targets are literal IP addresses, the requests are
//! plain HTTP/1.1, and all operations are bounded by explicit timeouts so a
//! scan of hundreds of ports stays fast.

use std::io::{ErrorKind, Read, Write};
use std::net::{SocketAddr, TcpStream};
use std::time::{Duration, Instant};

use serde::{Deserialize, Serialize};

use crate::openmetrics;

/// Maximum size of the HTTP header section we accept (headers beyond this
/// are truncated and the response is treated as not-http).
const MAX_HEADER_BYTES: usize = 16 * 1024;
/// Granularity of read timeouts while waiting for the response body.
const READ_SLICE: Duration = Duration::from_millis(500);

/// Coarse classification of a probe outcome.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Classification {
    /// HTTP 200 with a body that parses as Prometheus/OpenMetrics exposition.
    PrometheusMetrics,
    HttpOkNotMetrics,
    HttpRedirect,
    HttpAuthRequired,
    HttpNotFound,
    HttpOtherStatus,
    ConnectRefused,
    ConnectTimeout,
    ConnectError,
    ReadTimeout,
    /// The port answered with something that is not HTTP/1.x (possibly TLS).
    NotHttp,
    ProtocolError,
    /// Could not enter the network namespace owning the port (used by the
    /// scan orchestration, never by `probe_endpoint` itself).
    NetnsEnterFailed,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EndpointProbeResult {
    /// Full URL that was probed, e.g. `http://127.0.0.1:9404/metrics`.
    pub url: String,
    pub classification: Classification,
    pub status: Option<u16>,
    pub content_type: Option<String>,
    /// `Location` header value for redirects.
    pub location: Option<String>,
    /// Number of body bytes received (after chunked decoding, before the cap).
    pub body_bytes: usize,
    pub body_truncated: bool,
    pub analysis: Option<openmetrics::MetricsAnalysis>,
    pub error: Option<String>,
    pub elapsed_ms: u64,
}

/// Probes `http://<addr><path>` with the given timeouts.
///
/// The request is sent with `Connection: close`, so well-behaved servers
/// (including everything built on Go's net/http) close the connection after
/// the response; a global deadline bounds the worst case regardless.
pub fn probe_endpoint(
    addr: SocketAddr,
    path: &str,
    connect_timeout: Duration,
    request_timeout: Duration,
    max_body_bytes: usize,
) -> EndpointProbeResult {
    let start = Instant::now();
    let url = format!("http://{addr}{path}");

    let error_result = |classification: Classification, error: String, body_bytes: usize| {
        EndpointProbeResult {
            elapsed_ms: elapsed_ms(start),
            url: url.clone(),
            classification,
            status: None,
            content_type: None,
            location: None,
            body_bytes,
            body_truncated: false,
            analysis: None,
            error: Some(error),
        }
    };

    let stream = match TcpStream::connect_timeout(&addr, connect_timeout) {
        Ok(s) => s,
        Err(e) => {
            let classification = match e.kind() {
                ErrorKind::ConnectionRefused => Classification::ConnectRefused,
                ErrorKind::TimedOut => Classification::ConnectTimeout,
                _ => Classification::ConnectError,
            };
            return error_result(classification, e.to_string(), 0);
        }
    };

    let deadline = start + request_timeout;
    let mut stream = stream;
    let _ = stream.set_nodelay(true);
    let _ = stream.set_write_timeout(Some(request_timeout));

    let request = format!(
        "GET {path} HTTP/1.1\r\nHost: {addr}\r\nUser-Agent: dd-disco/0.1\r\nAccept: */*\r\nConnection: close\r\n\r\n"
    );
    if let Err(e) = stream.write_all(request.as_bytes()) {
        return error_result(
            Classification::ProtocolError,
            format!("write failed: {e}"),
            0,
        );
    }

    // Read the response. Total cap leaves room for the header section; the
    // body itself is capped at max_body_bytes (larger bodies are marked
    // truncated, which the analysis accounts for).
    let total_cap = max_body_bytes.saturating_add(MAX_HEADER_BYTES);
    let mut buf: Vec<u8> = Vec::with_capacity(8 * 1024);
    let mut chunk = [0u8; 16 * 1024];
    let mut body_truncated = false;

    loop {
        if Instant::now() >= deadline {
            return error_result(
                Classification::ReadTimeout,
                format!("no complete response within {}ms", request_timeout.as_millis()),
                buf.len(),
            );
        }
        let remaining = deadline - Instant::now();
        let _ = stream.set_read_timeout(Some(remaining.min(READ_SLICE)));
        match stream.read(&mut chunk) {
            Ok(0) => break, // EOF: server closed the connection
            Ok(n) => {
                if let Some(part) = chunk.get(..n) {
                    buf.extend_from_slice(part);
                }
                if buf.len() >= total_cap {
                    body_truncated = true;
                    break;
                }
                if response_complete(&buf, max_body_bytes) {
                    break;
                }
            }
            Err(e) if e.kind() == ErrorKind::WouldBlock || e.kind() == ErrorKind::TimedOut => {
                // Interrupted read slice; the deadline check above bounds the
                // total wait.
                continue;
            }
            Err(e) => {
                return error_result(
                    Classification::ProtocolError,
                    format!("read failed: {e}"),
                    buf.len(),
                );
            }
        }
    }

    parse_response(&buf, body_truncated, max_body_bytes, start, url)
}

fn elapsed_ms(start: Instant) -> u64 {
    u64::try_from(start.elapsed().as_millis()).unwrap_or(u64::MAX)
}

/// Heuristic to stop reading early: the header section is complete and the
/// body satisfies the advertised Content-Length (or reached the cap). This
/// avoids waiting for EOF on servers that keep the connection open despite
/// `Connection: close`.
fn response_complete(buf: &[u8], max_body_bytes: usize) -> bool {
    let Some(term_pos) = find_subslice(buf, b"\r\n\r\n") else {
        return false;
    };
    let headers = String::from_utf8_lossy(buf.get(0..term_pos).unwrap_or(&[]));
    let mut content_length: Option<usize> = None;
    for line in headers.split('\n') {
        let line = line.trim_end_matches('\r');
        if let Some((name, value)) = line.split_once(':')
            && name.trim().eq_ignore_ascii_case("content-length")
        {
            content_length = value.trim().parse::<usize>().ok();
        }
    }
    let Some(expected) = content_length else {
        return false;
    };
    let body_start = term_pos + 4;
    let body_len = buf.len().saturating_sub(body_start);
    body_len >= expected.min(max_body_bytes)
}

fn parse_response(
    buf: &[u8],
    truncated: bool,
    max_body_bytes: usize,
    start: Instant,
    url: String,
) -> EndpointProbeResult {
    let base = |classification: Classification,
                status: Option<u16>,
                content_type: Option<String>,
                location: Option<String>,
                body_bytes: usize,
                body_truncated: bool,
                analysis: Option<openmetrics::MetricsAnalysis>,
                error: Option<String>| EndpointProbeResult {
        elapsed_ms: elapsed_ms(start),
        url: url.clone(),
        classification,
        status,
        content_type,
        location,
        body_bytes,
        body_truncated,
        analysis,
        error,
    };
    let not_http = |detail: String| {
        base(
            Classification::NotHttp,
            None,
            None,
            None,
            buf.len(),
            false,
            None,
            Some(detail),
        )
    };

    let (term_len, term_pos) = if let Some(p) = find_subslice(buf, b"\r\n\r\n") {
        (4, p)
    } else if let Some(p) = find_subslice(buf, b"\n\n") {
        (2, p)
    } else if buf.is_empty() {
        return not_http("empty response: connection closed without data".to_string());
    } else {
        return not_http(format!(
            "no HTTP header terminator; first bytes: {}",
            first_bytes_hex(buf)
        ));
    };

    let headers_raw = buf.get(0..term_pos).unwrap_or(&[]);
    let headers_str = String::from_utf8_lossy(headers_raw);
    let mut lines = headers_str.split('\n').map(|l| l.trim_end_matches('\r'));

    let status_line = lines.next().unwrap_or("");
    let mut parts = status_line.split_whitespace();
    let Some(http_version) = parts.next() else {
        return not_http("empty status line".to_string());
    };
    if !http_version.starts_with("HTTP/1.") {
        return not_http(format!(
            "status line is not HTTP/1.x ({http_version:?}); first bytes: {}",
            first_bytes_hex(buf)
        ));
    }
    let status = match parts.next().and_then(|c| c.parse::<u16>().ok()) {
        Some(s) if (100..600).contains(&s) => s,
        _ => {
            return not_http(format!("unparsable status line: {status_line:?}"));
        }
    };

    let mut content_type: Option<String> = None;
    let mut content_length: Option<usize> = None;
    let mut location: Option<String> = None;
    let mut chunked = false;
    for line in lines {
        let Some((name, value)) = line.split_once(':') else {
            continue;
        };
        let name = name.trim().to_ascii_lowercase();
        let value = value.trim();
        match name.as_str() {
            "content-type" => content_type = Some(value.to_string()),
            "content-length" => content_length = value.parse::<usize>().ok(),
            "location" => location = Some(value.to_string()),
            "transfer-encoding" if value.to_ascii_lowercase().contains("chunked") => {
                chunked = true;
            }
            _ => {}
        }
    }

    let body_start = term_pos + term_len;
    let body_raw = buf.get(body_start..).unwrap_or(&[]);
    let (body, chunk_truncated) = if chunked {
        let (decoded, t) = decode_chunked(body_raw, max_body_bytes);
        (decoded, t)
    } else if body_raw.len() > max_body_bytes {
        (
            body_raw.get(0..max_body_bytes).unwrap_or(&[]).to_vec(),
            true,
        )
    } else {
        (body_raw.to_vec(), false)
    };
    let mut body_truncated = truncated || chunk_truncated;
    if let Some(expected) = content_length
        && expected > body.len()
    {
        body_truncated = true;
    }

    let (classification, analysis) = if status == 200 {
        let analysis = openmetrics::analyze(&String::from_utf8_lossy(&body), body_truncated);
        let classification = if analysis.is_metrics_endpoint {
            Classification::PrometheusMetrics
        } else {
            Classification::HttpOkNotMetrics
        };
        (classification, Some(analysis))
    } else {
        let classification = match status {
            301 | 302 | 303 | 307 | 308 => Classification::HttpRedirect,
            401 | 403 => Classification::HttpAuthRequired,
            404 => Classification::HttpNotFound,
            _ => Classification::HttpOtherStatus,
        };
        (classification, None)
    };

    base(
        classification,
        Some(status),
        content_type,
        location,
        body.len(),
        body_truncated,
        analysis,
        None,
    )
}

fn find_subslice(haystack: &[u8], needle: &[u8]) -> Option<usize> {
    memchr::memmem::find(haystack, needle)
}

fn first_bytes_hex(buf: &[u8]) -> String {
    let prefix = buf.get(0..usize::min(16, buf.len())).unwrap_or(&[]);
    prefix
        .iter()
        .map(|b| format!("{b:02x}"))
        .collect::<Vec<_>>()
        .join(" ")
}

/// Decodes a chunked body up to `cap` bytes. Returns the decoded body and
/// whether it was truncated (missing chunk data or cut before the final
/// chunk).
fn decode_chunked(data: &[u8], cap: usize) -> (Vec<u8>, bool) {
    let mut out: Vec<u8> = Vec::new();
    let mut rest = data;
    loop {
        let Some(line_end) = find_subslice(rest, b"\r\n") else {
            return (out, true);
        };
        let size_line = String::from_utf8_lossy(rest.get(0..line_end).unwrap_or(&[]));
        let size_str = size_line.split(';').next().unwrap_or("").trim();
        let Ok(size) = usize::from_str_radix(size_str, 16) else {
            return (out, true);
        };
        if size == 0 {
            // Final chunk; trailers (if any) are ignored.
            return (out, false);
        }
        let chunk_start = line_end + 2;
        if rest.len() < chunk_start {
            return (out, true);
        }
        let take = usize::min(size, cap.saturating_sub(out.len()));
        if let Some(chunk) = rest.get(chunk_start..chunk_start + take) {
            out.extend_from_slice(chunk);
        }
        if out.len() >= cap {
            return (out, true);
        }
        let after_chunk = chunk_start + size;
        if rest.len() < after_chunk + 2 {
            // The trailing CRLF of the chunk is missing: truncated body.
            return (out, true);
        }
        rest = rest.get(after_chunk + 2..).unwrap_or(&[]);
    }
}

#[cfg(test)]
#[allow(clippy::unwrap_used, clippy::expect_used)]
mod tests {
    use super::*;
    use std::net::TcpListener;

    /// Starts a one-shot TCP server that reads the request, writes
    /// `response` and closes. Returns the address to connect to.
    fn serve_once(response: Vec<u8>) -> SocketAddr {
        let listener = TcpListener::bind("127.0.0.1:0").expect("bind");
        let addr = listener.local_addr().expect("local addr");
        std::thread::spawn(move || {
            if let Ok((mut conn, _)) = listener.accept() {
                let mut req = [0u8; 4096];
                // Read the request (single small request).
                let _ = conn.read(&mut req);
                let _ = conn.write_all(&response);
                let _ = conn.flush();
                // Drop closes the connection.
            }
        });
        addr
    }

    fn probe(addr: SocketAddr, path: &str) -> EndpointProbeResult {
        probe_endpoint(
            addr,
            path,
            Duration::from_millis(500),
            Duration::from_millis(2000),
            64 * 1024,
        )
    }

    const METRICS_BODY: &str = "# HELP go_goroutines Number of goroutines that currently exist.\n# TYPE go_goroutines gauge\ngo_goroutines 42\nhttp_requests_total 1027\n";

    #[test]
    fn test_prometheus_metrics_endpoint() {
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: text/plain; version=0.0.4; charset=utf-8\r\nContent-Length: {}\r\n\r\n{}",
            METRICS_BODY.len(),
            METRICS_BODY
        );
        let addr = serve_once(response.into_bytes());
        let r = probe(addr, "/metrics");
        assert_eq!(r.classification, Classification::PrometheusMetrics);
        assert_eq!(r.status, Some(200));
        assert_eq!(
            r.content_type.as_deref(),
            Some("text/plain; version=0.0.4; charset=utf-8")
        );
        let a = r.analysis.as_ref().unwrap();
        assert!(a.is_metrics_endpoint);
        assert!(a.has_go_runtime_metrics);
    }

    #[test]
    fn test_http_404() {
        let response = "HTTP/1.1 404 Not Found\r\nContent-Type: text/plain\r\nContent-Length: 9\r\n\r\nnot found";
        let addr = serve_once(response.as_bytes().to_vec());
        let r = probe(addr, "/metrics");
        assert_eq!(r.classification, Classification::HttpNotFound);
        assert_eq!(r.status, Some(404));
    }

    #[test]
    fn test_redirect() {
        let response = "HTTP/1.1 302 Found\r\nLocation: /custom-metrics\r\nContent-Length: 0\r\n\r\n";
        let addr = serve_once(response.as_bytes().to_vec());
        let r = probe(addr, "/metrics");
        assert_eq!(r.classification, Classification::HttpRedirect);
        assert_eq!(r.location.as_deref(), Some("/custom-metrics"));
    }

    #[test]
    fn test_auth_required() {
        let response = "HTTP/1.1 401 Unauthorized\r\nContent-Length: 0\r\n\r\n";
        let addr = serve_once(response.as_bytes().to_vec());
        let r = probe(addr, "/metrics");
        assert_eq!(r.classification, Classification::HttpAuthRequired);
    }

    #[test]
    fn test_html_response() {
        let body = "<html><body>hello</body></html>";
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: {}\r\n\r\n{}",
            body.len(),
            body
        );
        let addr = serve_once(response.into_bytes());
        let r = probe(addr, "/");
        assert_eq!(r.classification, Classification::HttpOkNotMetrics);
        let a = r.analysis.as_ref().unwrap();
        assert!(!a.is_metrics_endpoint);
    }

    #[test]
    fn test_chunked_metrics() {
        let body = "4\r\ngo_g\r\n10\r\noroutines 42\nu 1\n\r\n0\r\n\r\n";
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nTransfer-Encoding: chunked\r\n\r\n{}",
            body
        );
        let addr = serve_once(response.into_bytes());
        let r = probe(addr, "/metrics");
        assert_eq!(r.classification, Classification::PrometheusMetrics);
        let a = r.analysis.as_ref().unwrap();
        assert_eq!(a.sample_count, 2);
    }

    #[test]
    fn test_not_http_tls_like() {
        // TLS ServerHello starts with 0x16 0x03 ...
        let response = [0x16, 0x03, 0x01, 0x00, 0x50, 0x01, 0x00, 0x00];
        let addr = serve_once(response.to_vec());
        let r = probe(addr, "/metrics");
        assert_eq!(r.classification, Classification::NotHttp);
        assert!(r.error.as_deref().unwrap().contains("16 03 01"));
    }

    #[test]
    fn test_connect_refused() {
        // Bind and immediately close to get a refused port.
        let listener = TcpListener::bind("127.0.0.1:0").expect("bind");
        let addr = listener.local_addr().expect("addr");
        drop(listener);
        let r = probe(addr, "/metrics");
        assert_eq!(r.classification, Classification::ConnectRefused);
    }

    #[test]
    fn test_read_timeout() {
        let listener = TcpListener::bind("127.0.0.1:0").expect("bind");
        let addr = listener.local_addr().expect("addr");
        std::thread::spawn(move || {
            if let Ok((conn, _)) = listener.accept() {
                // Hold the connection open without answering.
                std::thread::sleep(Duration::from_secs(10));
                drop(conn);
            }
        });
        let r = probe_endpoint(
            addr,
            "/metrics",
            Duration::from_millis(500),
            Duration::from_millis(300),
            64 * 1024,
        );
        assert_eq!(r.classification, Classification::ReadTimeout);
    }

    #[test]
    fn test_body_cap_marks_truncated() {
        // Build a valid metrics body larger than the cap.
        let mut body = String::new();
        for i in 0..1000 {
            body.push_str(&format!("metric_{i} 1\n"));
        }
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Length: {}\r\n\r\n{}",
            body.len(),
            body
        );
        let addr = serve_once(response.into_bytes());
        let r = probe_endpoint(
            addr,
            "/metrics",
            Duration::from_millis(500),
            Duration::from_millis(2000),
            8 * 1024, // much smaller than the body
        );
        assert_eq!(r.classification, Classification::PrometheusMetrics);
        assert!(r.body_truncated);
        // The partial last line must not break detection.
        let a = r.analysis.as_ref().unwrap();
        assert!(a.is_metrics_endpoint);
        assert_eq!(a.invalid_line_count, 0);
    }
}
